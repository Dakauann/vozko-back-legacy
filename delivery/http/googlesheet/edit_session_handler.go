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

type EditSessions interface {
	Editor(ctx context.Context, workspaceID, spreadsheetID string) (*gsuc.EditorView, error)
	Start(ctx context.Context, in gsuc.StartEditInput) (*gsuc.EditView, error)
	Heartbeat(ctx context.Context, in gsuc.EditInput) (*gsuc.EditView, error)
	End(ctx context.Context, in gsuc.EndEditInput) (*gsuc.EditView, error)
	History(ctx context.Context, workspaceID, spreadsheetID string, pagination shared.Pagination) (*shared.PaginatedResult[gsuc.EditView], error)
}

type EditSessionHandler struct {
	edits EditSessions
}

func NewEditSessionHandler(edits EditSessions) *EditSessionHandler {
	if edits == nil {
		panic("googlesheet http: the edit session handler requires the edit sessions")
	}
	return &EditSessionHandler{edits: edits}
}

type EditTabCheckResponse struct {
	SheetID     string `json:"sheetId" example:"3b9f4c2e-1d7a-4e5b-9c8d-2a1f0e3d4c5b"`
	TabID       int64  `json:"tabId" example:"0"`
	TabName     string `json:"tabName" example:"Pagamentos"`
	RenamedFrom string `json:"renamedFrom,omitempty" example:"Pagamentos de outubro"`
	Reason      string `json:"reason,omitempty" enums:"header_mismatch,header_not_first_row,spreadsheet_missing"`
	Detail      string `json:"detail,omitempty" example:"coluna C: esperado \"Valor\", encontrado \"Preço\""`
	Repairable  bool   `json:"repairable"`
	Unchecked   string `json:"unchecked,omitempty"`
	RefWarnings []int  `json:"refWarnings,omitempty"`
}

type EditCheckResponse struct {
	Status string                 `json:"status" enums:"ok,broken,unchecked"`
	Detail string                 `json:"detail,omitempty"`
	Tabs   []EditTabCheckResponse `json:"tabs"`
}

type EditSessionResponse struct {
	ID            string              `json:"id" example:"6f1c2d3e-4a5b-4c6d-8e7f-90a1b2c3d4e5"`
	SpreadsheetID string              `json:"spreadsheetId" example:"1AbC"`
	StartedBy     OriginNameResponse  `json:"startedBy"`
	StartedAt     time.Time           `json:"startedAt"`
	ExpiresAt     time.Time           `json:"expiresAt"`
	Deadline      time.Time           `json:"deadline"`
	Settled       bool                `json:"settled"`
	Mine          bool                `json:"mine"`
	EndedAt       *time.Time          `json:"endedAt,omitempty"`
	EndedBy       *OriginNameResponse `json:"endedBy,omitempty"`
	EndReason     string              `json:"endReason,omitempty" enums:"finished,left,expired,ended_by_admin"`
	WaitedRows    int                 `json:"waitedRows"`
	Check         *EditCheckResponse  `json:"check,omitempty"`
}

type EditSessionEnvelope struct {
	Session EditSessionResponse `json:"session"`
}

type EditorResponse struct {
	Editing          *EditSessionResponse `json:"editing"`
	AccountEmail     string               `json:"accountEmail,omitempty" example:"financeiro@empresa.com.br"`
	HeartbeatSeconds int                  `json:"heartbeatSeconds" example:"30"`
	LeaseSeconds     int                  `json:"leaseSeconds" example:"120"`
}

type EditSessionListResponse struct {
	Items      []EditSessionResponse `json:"items"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
	TotalItems int64                 `json:"total_items"`
	TotalPages int                   `json:"total_pages"`
}

type EndEditRequest struct {
	Reason string `json:"reason" enums:"finished,left" example:"finished"`
}

type fileBeingEditedRefusal struct {
	EditedBy     string    `json:"editedBy"`
	EditedByName string    `json:"editedByName,omitempty"`
	Since        time.Time `json:"since"`
}

func toEditCheckResponse(check googlesheet.EditCheck) *EditCheckResponse {
	if !check.Done() {
		return nil
	}
	tabs := make([]EditTabCheckResponse, len(check.Tabs))
	for i, tab := range check.Tabs {
		tabs[i] = EditTabCheckResponse{
			SheetID: tab.SheetID, TabID: tab.TabID, TabName: tab.TabName, RenamedFrom: tab.RenamedFrom, Reason: string(tab.Reason),
			Detail: tab.Detail, Repairable: tab.Repairable(), Unchecked: tab.Unchecked, RefWarnings: tab.RefWarnings,
		}
	}
	return &EditCheckResponse{Status: string(check.Status), Detail: check.Detail, Tabs: tabs}
}

func toEditSessionResponse(view *gsuc.EditView, userID string) EditSessionResponse {
	s := view.Session
	startedBy := OriginNameResponse{ID: s.StartedBy}
	if view.StartedBy != nil {
		startedBy.Name = view.StartedBy.Name
	}
	out := EditSessionResponse{
		ID: s.ID, SpreadsheetID: s.SpreadsheetID, StartedBy: startedBy, StartedAt: s.StartedAt, ExpiresAt: s.ExpiresAt,
		Deadline: s.Deadline(), Settled: s.Settled(), Mine: s.StartedBy == userID, EndedAt: s.EndedAt, EndReason: string(s.EndReason),
		WaitedRows: s.WaitedRows, Check: toEditCheckResponse(s.Check),
	}
	if s.EndedBy != "" {
		out.EndedBy = &OriginNameResponse{ID: s.EndedBy}
		if view.EndedBy != nil {
			out.EndedBy.Name = view.EndedBy.Name
		}
	}
	return out
}

func toEditSessionPointer(view *gsuc.EditView, userID string) *EditSessionResponse {
	if view == nil {
		return nil
	}
	out := toEditSessionResponse(view, userID)
	return &out
}

func (h *EditSessionHandler) answer(w http.ResponseWriter, r *http.Request, view *gsuc.EditView, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, EditSessionEnvelope{Session: toEditSessionResponse(view, userOf(r))})
}

// @Summary		Edição à mão do arquivo
// @Description	Estado da edição à mão do arquivo do Google: editing é a sessão em andamento (quem, desde quando, até quando, se é sua) ou null. accountEmail é a conta Google conectada, dona do arquivo, a quem pedir acesso. heartbeatSeconds é o intervalo do sinal de presença e leaseSeconds o tempo em que uma sessão sem sinal termina sozinha.
// @Tags			Google Planilhas
// @Produce		json
// @Param			spreadsheetId	path		string	true	"Arquivo do Google"
// @Success		200				{object}	EditorResponse
// @Failure		404				{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/files/{spreadsheetId}/edit-session [get]
func (h *EditSessionHandler) Editor(w http.ResponseWriter, r *http.Request) {
	view, err := h.edits.Editor(r.Context(), middleware.GetWorkspaceID(r), mux.Vars(r)["spreadsheetId"])
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, EditorResponse{
		Editing: toEditSessionPointer(view.Editing, userOf(r)), AccountEmail: view.AccountEmail,
		HeartbeatSeconds: int(googlesheet.EditHeartbeatEvery / time.Second), LeaseSeconds: int(googlesheet.EditSessionTTL / time.Second),
	})
}

// @Summary		Começar a edição à mão do arquivo
// @Description	Trava o arquivo inteiro para edição à mão no editor do Google. Enquanto a sessão durar, o Vozko não grava nada no arquivo: as linhas novas ficam na fila, e mudanças de colunas, abas novas, reparos e remoções de linha são recusados com google_sheet_file_being_edited. A trava espera a gravação em andamento terminar; settled diz se já terminou (senão o sinal de presença confere de novo). A sessão dura enquanto o sinal de presença chegar a cada heartbeatSeconds, termina sozinha leaseSeconds depois do último sinal e nunca passa de 4 horas (deadline). Só uma pessoa edita o arquivo por vez: outra recebe 409 google_sheet_file_being_edited com details.editedBy, details.editedByName e details.since.
// @Tags			Google Planilhas
// @Produce		json
// @Param			spreadsheetId	path		string	true	"Arquivo do Google"
// @Success		200				{object}	EditSessionEnvelope
// @Failure		404				{object}	response.ErrorResponse
// @Failure		409				{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/files/{spreadsheetId}/edit-session [post]
func (h *EditSessionHandler) Start(w http.ResponseWriter, r *http.Request) {
	view, err := h.edits.Start(r.Context(), gsuc.StartEditInput{WorkspaceID: middleware.GetWorkspaceID(r), SpreadsheetID: mux.Vars(r)["spreadsheetId"], UserID: userOf(r)})
	h.answer(w, r, view, err)
}

// @Summary		Sinal de presença da edição à mão
// @Description	Mantém a sessão viva por mais leaseSeconds, sem passar do deadline, e confere se a gravação em andamento já terminou (settled). Só quem começou a sessão envia o sinal (403 google_sheet_edit_session_not_yours). Uma sessão que já terminou, inclusive por falta de sinal, nunca volta: a resposta traz endedAt, endReason e a verificação (check).
// @Tags			Google Planilhas
// @Produce		json
// @Param			sessionId	path		string	true	"Sessão de edição"
// @Success		200			{object}	EditSessionEnvelope
// @Failure		403			{object}	response.ErrorResponse
// @Failure		404			{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/edit-sessions/{sessionId}/heartbeat [post]
func (h *EditSessionHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	view, err := h.edits.Heartbeat(r.Context(), gsuc.EditInput{WorkspaceID: middleware.GetWorkspaceID(r), SessionID: mux.Vars(r)["sessionId"], UserID: userOf(r)})
	h.answer(w, r, view, err)
}

// @Summary		Terminar a edição à mão
// @Description	Termina a sua sessão (reason finished ao clicar em terminar, left ao sair da página) e confere na hora cada aba do arquivo em que o Vozko grava: cabeçalho na linha 1, a coluna de referência A e os cabeçalhos das colunas. check.status é ok (as linhas que esperavam, contadas em waitedRows, entram logo abaixo da última linha do Vozko), broken (cada aba quebrada vem com reason, detail e repairable, e fica precisando de atenção) ou unchecked (o Google não pôde ser lido agora; cada gravação confere a aba antes de gravar). Abas renomeadas são seguidas (renamedFrom). refWarnings lista as linhas cuja coluna A tem algo que não é uma referência do Vozko ou repete uma. Terminar de novo devolve o resultado guardado.
// @Tags			Google Planilhas
// @Accept			json
// @Produce		json
// @Param			sessionId	path		string			true	"Sessão de edição"
// @Param			request		body		EndEditRequest	true	"Motivo"
// @Success		200			{object}	EditSessionEnvelope
// @Failure		400			{object}	response.ErrorResponse
// @Failure		403			{object}	response.ErrorResponse
// @Failure		404			{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/edit-sessions/{sessionId}/end [post]
func (h *EditSessionHandler) End(w http.ResponseWriter, r *http.Request) {
	var req EndEditRequest
	if !decode(w, r, &req) {
		return
	}
	reason, err := googlesheet.ParseClientEndReason(req.Reason)
	if err != nil {
		writeError(w, err)
		return
	}
	h.end(w, r, reason)
}

// @Summary		Encerrar a edição à mão de outra pessoa
// @Description	Encerra uma sessão travada de qualquer pessoa, com a mesma verificação de terminar a edição. Exige a permissão de remover planilhas. A sessão guarda quem encerrou (endedBy) e endReason ended_by_admin, e aparece no histórico do arquivo.
// @Tags			Google Planilhas
// @Produce		json
// @Param			sessionId	path		string	true	"Sessão de edição"
// @Success		200			{object}	EditSessionEnvelope
// @Failure		404			{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/edit-sessions/{sessionId}/force-end [post]
func (h *EditSessionHandler) ForceEnd(w http.ResponseWriter, r *http.Request) {
	h.end(w, r, googlesheet.EditEndedByAdmin)
}

func (h *EditSessionHandler) end(w http.ResponseWriter, r *http.Request, reason googlesheet.EditEndReason) {
	view, err := h.edits.End(r.Context(), gsuc.EndEditInput{
		WorkspaceID: middleware.GetWorkspaceID(r), SessionID: mux.Vars(r)["sessionId"], UserID: userOf(r), Reason: reason,
	})
	h.answer(w, r, view, err)
}

// @Summary		Histórico de edições à mão do arquivo
// @Description	Sessões de edição à mão do arquivo, da mais recente para a mais antiga: quem começou e quando, quem terminou, como (finished, left, expired, ended_by_admin), quantas linhas esperaram (waitedRows) e o resultado da verificação (check).
// @Tags			Google Planilhas
// @Produce		json
// @Param			spreadsheetId	path		string	true	"Arquivo do Google"
// @Param			page			query		int		false	"Página"
// @Param			pageSize		query		int		false	"Tamanho da página"
// @Success		200				{object}	EditSessionListResponse
// @Failure		404				{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/files/{spreadsheetId}/edit-sessions [get]
func (h *EditSessionHandler) History(w http.ResponseWriter, r *http.Request) {
	page, err := h.edits.History(r.Context(), middleware.GetWorkspaceID(r), mux.Vars(r)["spreadsheetId"], httpx.ParsePagination(r.URL.Query()))
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]EditSessionResponse, len(page.Items))
	for i := range page.Items {
		items[i] = toEditSessionResponse(&page.Items[i], userOf(r))
	}
	response.WriteSuccess(w, http.StatusOK, EditSessionListResponse{Items: items, Page: page.Page, PageSize: page.PageSize, TotalItems: page.TotalItems, TotalPages: page.TotalPages})
}
