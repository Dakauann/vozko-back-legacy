package googlesheet

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/infra/http/middleware"
	gsuc "vozko/usecases/googlesheet"
)

type RowRemovals interface {
	Cancel(ctx context.Context, in gsuc.RemoveRowInput) (*googlesheet.RowRemoval, error)
	Remove(ctx context.Context, in gsuc.RemoveRowInput) (*googlesheet.RowRemoval, error)
	Removals(ctx context.Context, workspaceID, sheetID string, pagination shared.Pagination) (*shared.PaginatedResult[gsuc.RemovalView], error)
}

type RowRemovalHandler struct {
	removals RowRemovals
}

func NewRowRemovalHandler(removals RowRemovals) *RowRemovalHandler {
	if removals == nil {
		panic("googlesheet http: the row removal handler requires the row remover")
	}
	return &RowRemovalHandler{removals: removals}
}

type RemoveRowRequest struct {
	Reason string `json:"reason" example:"Registro duplicado pelo fluxo de cobrança"`
}

type RowRemovalResponse struct {
	ID           string              `json:"id" example:"3b9f4c2e-1d7a-4e5b-9c8d-2a1f0e3d4c5b"`
	RowID        string              `json:"rowId" example:"6f1c2d3e-4a5b-4c6d-8e7f-90a1b2c3d4e5"`
	Kind         string              `json:"kind" enums:"cancel,remove"`
	Reason       string              `json:"reason" example:"Registro duplicado pelo fluxo de cobrança"`
	Values       googlesheet.Cells   `json:"values" swaggertype:"object"`
	RowNumber    int                 `json:"rowNumber,omitempty" example:"14"`
	RemovedAt    time.Time           `json:"removedAt"`
	RemovedBy    *OriginNameResponse `json:"removedBy,omitempty"`
	Origin       string              `json:"origin" enums:"agent,workflow,copilot,manual"`
	OriginDetail RowOriginResponse   `json:"originDetail"`
}

type RowRemovalEnvelope struct {
	Removal RowRemovalResponse `json:"removal"`
}

type RowRemovalListResponse struct {
	Items      []RowRemovalResponse `json:"items"`
	Page       int                  `json:"page"`
	PageSize   int                  `json:"page_size"`
	TotalItems int64                `json:"total_items"`
	TotalPages int                  `json:"total_pages"`
}

func toRowRemovalResponse(removal *googlesheet.RowRemoval, origin gsuc.OriginView, removedBy *gsuc.NamedRef) RowRemovalResponse {
	return RowRemovalResponse{
		ID: removal.ID, RowID: removal.RowID, Kind: string(removal.Kind), Reason: removal.Reason, Values: removal.Cells,
		RowNumber: removal.RowNumber, RemovedAt: removal.RemovedAt, RemovedBy: nameOf(removedBy),
		Origin: string(removal.Origin.Kind), OriginDetail: toRowOriginResponse(origin),
	}
}

func (h *RowRemovalHandler) removal(w http.ResponseWriter, r *http.Request, act func(context.Context, gsuc.RemoveRowInput) (*googlesheet.RowRemoval, error)) {
	var req RemoveRowRequest
	if !decode(w, r, &req) {
		return
	}
	vars := mux.Vars(r)
	removal, err := act(r.Context(), gsuc.RemoveRowInput{
		WorkspaceID: middleware.GetWorkspaceID(r), SheetID: vars["id"], RowID: vars["rowId"], ActorID: userOf(r), Reason: req.Reason,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, RowRemovalEnvelope{Removal: toRowRemovalResponse(removal, gsuc.OriginView{Kind: removal.Origin.Kind}, nil)})
}

// @Summary		Cancelar a gravação de uma linha na fila
// @Description	Tira da fila uma linha que ainda não chegou ao Google (aguardando, retida ou recusada), com o motivo obrigatório (até 500 caracteres). O Google não é chamado. Uma linha que já está sendo gravada responde google_sheet_row_being_written, e uma que já foi gravada responde google_sheet_row_status_changed; nesse caso use remover da planilha. A remoção fica registrada com quem, quando, o motivo e os valores da linha, e libera a chave de linha única para o registro poder ser gravado de novo.
// @Tags			Google Planilhas
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"Planilha"
// @Param			rowId	path		string				true	"Linha"
// @Param			request	body		RemoveRowRequest	true	"Motivo"
// @Success		200		{object}	RowRemovalEnvelope
// @Failure		400		{object}	response.ErrorResponse
// @Failure		404		{object}	response.ErrorResponse
// @Failure		409		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/{id}/rows/{rowId}/cancel [post]
func (h *RowRemovalHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	h.removal(w, r, h.removals.Cancel)
}

// @Summary		Remover uma linha gravada da planilha
// @Description	Apaga no Google exatamente a linha gravada, achada pela referência na coluna A oculta, onde quer que ela esteja depois de ordenações ou movimentos. O motivo é obrigatório (até 500 caracteres). Nada é apagado e a resposta traz o código quando:
// @Description
// @Description	| Código | Quando |
// @Description	| --- | --- |
// @Description	| google_sheet_not_active | A planilha está pausada ou precisa de atenção, ou a linha 1 não é o cabeçalho esperado (a planilha fica pausada com o motivo). |
// @Description	| google_sheet_row_not_in_sheet | A referência não está mais na coluna A da aba. |
// @Description	| google_sheet_row_ref_repeated | A referência aparece mais de uma vez na coluna A. |
// @Description	| google_sheet_busy | Uma gravação está em andamento na planilha; tente de novo em instantes. |
// @Description	| google_sheet_google_busy | A cota do Google deste minuto acabou. |
// @Description	| google_sheet_row_status_changed | A linha ainda não foi gravada; use cancelar a gravação. |
// @Description
// @Description	A remoção fica registrada com quem, quando, o motivo e os valores da linha, e libera a chave de linha única para o registro poder ser gravado de novo.
// @Tags			Google Planilhas
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"Planilha"
// @Param			rowId	path		string				true	"Linha"
// @Param			request	body		RemoveRowRequest	true	"Motivo"
// @Success		200		{object}	RowRemovalEnvelope
// @Failure		400		{object}	response.ErrorResponse
// @Failure		404		{object}	response.ErrorResponse
// @Failure		409		{object}	response.ErrorResponse
// @Failure		429		{object}	response.ErrorResponse
// @Failure		502		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/{id}/rows/{rowId}/remove [post]
func (h *RowRemovalHandler) Remove(w http.ResponseWriter, r *http.Request) {
	h.removal(w, r, h.removals.Remove)
}

// @Summary		Linhas removidas da planilha
// @Description	Registro das gravações canceladas e das linhas removidas da planilha, da mais recente para a mais antiga: quem removeu, quando, o motivo, os valores que a linha tinha e de onde ela veio (originDetail, como na lista de linhas). kind é cancel para uma gravação cancelada antes de chegar ao Google e remove para uma linha apagada da planilha, com rowNumber, a linha em que estava.
// @Tags			Google Planilhas
// @Produce		json
// @Param			id			path		string	true	"Planilha"
// @Param			page		query		int		false	"Página"
// @Param			pageSize	query		int		false	"Tamanho da página"
// @Success		200			{object}	RowRemovalListResponse
// @Failure		404			{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/{id}/removals [get]
func (h *RowRemovalHandler) List(w http.ResponseWriter, r *http.Request) {
	page, err := h.removals.Removals(r.Context(), middleware.GetWorkspaceID(r), mux.Vars(r)["id"], httpx.ParsePagination(r.URL.Query()))
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]RowRemovalResponse, len(page.Items))
	for i, view := range page.Items {
		items[i] = toRowRemovalResponse(view.Removal, view.Origin, view.RemovedBy)
	}
	response.WriteSuccess(w, http.StatusOK, RowRemovalListResponse{Items: items, Page: page.Page, PageSize: page.PageSize, TotalItems: page.TotalItems, TotalPages: page.TotalPages})
}
