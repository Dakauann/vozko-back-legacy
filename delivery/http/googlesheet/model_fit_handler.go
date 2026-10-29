package googlesheet

import (
	"context"
	"errors"
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	"vozko/domain/googlesheet"
	"vozko/domain/sheetfit"
	"vozko/infra/http/middleware"
	gsuc "vozko/usecases/googlesheet"
)

type ModelFits interface {
	FitFiles(ctx context.Context, in gsuc.FitFilesInput) ([]gsuc.FileFit, error)
	CopyModel(ctx context.Context, in gsuc.CopyModelInput) (*gsuc.ModelCopy, error)
}

type ModelFitHandler struct {
	fits ModelFits
}

func NewModelFitHandler(fits ModelFits) *ModelFitHandler {
	if fits == nil {
		panic("googlesheet http: the model fit handler requires the model fits")
	}
	return &ModelFitHandler{fits: fits}
}

type FitUseRequest struct {
	ModelSheetID string            `json:"modelSheetId" example:"3b9f4c2e-1d7a-4e5b-9c8d-2a1f0e3d4c5b"`
	Kind         string            `json:"kind" example:"add_row" enums:"add_row,export"`
	Mapping      map[string]string `json:"mapping,omitempty" swaggertype:"object,string"`
}

type FitRequest struct {
	Uses []FitUseRequest `json:"uses"`
}

func (r FitRequest) uses() []sheetfit.Use {
	out := make([]sheetfit.Use, len(r.Uses))
	for i, use := range r.Uses {
		out[i] = sheetfit.Use{ModelSheetID: use.ModelSheetID, Kind: sheetfit.UseKind(use.Kind), Mapping: use.Mapping}
	}
	return out
}

type FitTabResponse struct {
	ModelSheetID string `json:"modelSheetId" example:"3b9f4c2e-1d7a-4e5b-9c8d-2a1f0e3d4c5b"`
	SheetID      string `json:"sheetId" example:"9a1b2c3d-4e5f-4a6b-8c7d-0e1f2a3b4c5d"`
	Title        string `json:"title" example:"Pagamentos"`
}

type FitProblemResponse struct {
	Code     string `json:"code" example:"sheet_fit_column_options"`
	SheetID  string `json:"sheetId,omitempty"`
	Tab      string `json:"tab,omitempty" example:"Pagamentos"`
	Column   string `json:"column,omitempty" example:"Baixa"`
	Option   string `json:"option,omitempty" example:"nao"`
	ModelTab string `json:"modelTab,omitempty" example:"Negociações"`
}

type FitCandidateResponse struct {
	SheetID string `json:"sheetId"`
	Title   string `json:"title" example:"Pagamentos novos"`
}

type MisfitResponse struct {
	Code         string                 `json:"code" example:"sheet_fit_tab_missing"`
	ModelSheetID string                 `json:"modelSheetId,omitempty"`
	ModelTab     string                 `json:"modelTab,omitempty" example:"Pagamentos"`
	Problems     []FitProblemResponse   `json:"problems,omitempty"`
	Candidates   []FitCandidateResponse `json:"candidates,omitempty"`
}

type FitTabOptionResponse struct {
	SheetID string              `json:"sheetId" example:"9a1b2c3d-4e5f-4a6b-8c7d-0e1f2a3b4c5d"`
	Title   string              `json:"title" example:"Pagamentos Centro"`
	Fits    bool                `json:"fits"`
	Chosen  bool                `json:"chosen"`
	Value   string              `json:"value" example:"{\"spreadsheet_id\":\"1AbC\",\"tabs\":{\"3b9f4c2e\":\"9a1b2c3d\"}}"`
	Problem *FitProblemResponse `json:"problem,omitempty"`
}

type FitTabChoiceResponse struct {
	ModelSheetID string                 `json:"modelSheetId" example:"3b9f4c2e-1d7a-4e5b-9c8d-2a1f0e3d4c5b"`
	ModelTab     string                 `json:"modelTab" example:"Pagamentos"`
	Options      []FitTabOptionResponse `json:"options"`
}

type FileFitResponse struct {
	SpreadsheetID string                 `json:"spreadsheetId" example:"1AbC"`
	Title         string                 `json:"title" example:"Franquia Centro"`
	URL           string                 `json:"url" example:"https://docs.google.com/spreadsheets/d/1AbC/edit"`
	Status        string                 `json:"status" enums:"active,needs_attention"`
	Fits          bool                   `json:"fits"`
	Choosable     bool                   `json:"choosable"`
	Chosen        bool                   `json:"chosen"`
	Value         string                 `json:"value" example:"1AbC"`
	Tabs          []FitTabResponse       `json:"tabs"`
	Choices       []FitTabChoiceResponse `json:"choices"`
	Misfit        *MisfitResponse        `json:"misfit,omitempty"`
}

type FileFitListResponse struct {
	Items []FileFitResponse `json:"items"`
}

type CopyModelRequest struct {
	FileTitle string `json:"fileTitle" example:"Franquia Centro"`
}

type ModelCopyResponse struct {
	File SheetFileResponse `json:"file"`
	Tabs []FitTabResponse  `json:"tabs"`
}

type misfitRefusal struct {
	ModelSheetID string `json:"modelSheetId,omitempty"`
}

// @Summary		Conferir quais arquivos servem para um modelo
// @Description	Confere cada arquivo de planilhas do workspace contra as abas do arquivo modelo (o do caminho) e os usos informados: em qual aba modelo cada passo grava (add_row grava linhas, export exporta mensagens) e quais colunas preenche (mapping, chave da coluna para o texto). Um arquivo serve quando tem uma aba para cada aba modelo usada e para cada aba de destino de link, com as colunas preenchidas presentes, ativas, preenchidas pelo fluxo e do mesmo tipo, as opções de escolha do modelo, nenhuma coluna obrigatória que o fluxo não preenche, colunas de mensagem nas abas de exportação e links apontando para as abas do próprio arquivo. tabs diz qual aba do arquivo recebe cada aba modelo. choices lista, para cada aba modelo, todas as abas do arquivo com fits e, quando não serve, o problema; choosable diz se o arquivo pode ser escolhido aba por aba (quando cada aba modelo tem ao menos uma aba que serve). value é o valor do arquivo com as abas escolhidas automaticamente, e o value de cada aba de choices é o valor que escolhe aquela aba para aquela aba modelo, mantendo as outras abas já escolhidas; chosen marca o arquivo e as abas do valor sendo conferido. Quando não serve, misfit traz o código e, para a aba modelo sem par, o primeiro problema de cada aba do arquivo, começando pela aba de mesmo nome. Uma aba que precisa de atenção não impede a escolha; status mostra isso. O próprio modelo também pode receber as linhas; arquivos de outra conta Google vêm com sheet_fit_file_other_account. Os que servem vêm primeiro.
// @Tags			Google Planilhas
// @Accept			json
// @Produce		json
// @Param			spreadsheetId	path		string		true	"Arquivo modelo"
// @Param			request			body		FitRequest	true	"Usos do modelo"
// @Success		200				{object}	FileFitListResponse
// @Failure		400				{object}	response.ErrorResponse
// @Failure		404				{object}	response.ErrorResponse
// @Failure		412				{object}	response.ErrorResponse
// @Failure		422				{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/files/{spreadsheetId}/fit [post]
func (h *ModelFitHandler) Fit(w http.ResponseWriter, r *http.Request) {
	var req FitRequest
	if !decode(w, r, &req) {
		return
	}
	fits, err := h.fits.FitFiles(r.Context(), gsuc.FitFilesInput{
		WorkspaceID: middleware.GetWorkspaceID(r), ModelSpreadsheetID: mux.Vars(r)["spreadsheetId"], Uses: req.uses(),
	})
	if err != nil {
		writeModelFitError(w, err)
		return
	}
	out := FileFitListResponse{Items: make([]FileFitResponse, len(fits))}
	for i, fit := range fits {
		out.Items[i] = FileFitOf(fit)
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

// @Summary		Criar planilha a partir do modelo
// @Description	Cria um arquivo novo no Google Drive da conta conectada com as abas do arquivo modelo (o do caminho): as mesmas colunas com as mesmas chaves, tipos, opções e cores, obrigatoriedade, instruções, abas por período com o mesmo padrão de nome, estilo, fuso, linha única e colunas de link apontando para as abas do arquivo novo. Colunas arquivadas não são copiadas, e nenhuma linha é copiada. tabs diz qual aba nova veio de cada aba modelo, então o arquivo serve para o modelo sem outra conferência. Recusas de definição e do Google seguem os códigos de Criar planilha, com details.sheet na aba que falhou; se uma aba falha, nada fica criado.
// @Tags			Google Planilhas
// @Accept			json
// @Produce		json
// @Param			spreadsheetId	path		string				true	"Arquivo modelo"
// @Param			request			body		CopyModelRequest	true	"Nome do arquivo novo"
// @Success		201				{object}	ModelCopyResponse
// @Failure		400				{object}	response.ErrorResponse
// @Failure		404				{object}	response.ErrorResponse
// @Failure		409				{object}	response.ErrorResponse
// @Failure		412				{object}	response.ErrorResponse
// @Failure		422				{object}	response.ErrorResponse
// @Failure		429				{object}	response.ErrorResponse
// @Failure		502				{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/files/{spreadsheetId}/copies [post]
func (h *ModelFitHandler) Copy(w http.ResponseWriter, r *http.Request) {
	var req CopyModelRequest
	if !decode(w, r, &req) {
		return
	}
	copied, err := h.fits.CopyModel(r.Context(), gsuc.CopyModelInput{
		WorkspaceID: middleware.GetWorkspaceID(r), UserID: userOf(r), ModelSpreadsheetID: mux.Vars(r)["spreadsheetId"], FileTitle: req.FileTitle,
	})
	if err != nil {
		writeModelFitError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusCreated, ModelCopyResponse{File: toFileResponse(copied.File), Tabs: fitTabs(copied.File, copied.Tabs)})
}

func FileFitOf(fit gsuc.FileFit) FileFitResponse {
	out := FileFitResponse{
		SpreadsheetID: fit.File.ID, Title: fit.File.Title(), URL: fit.File.URL(), Status: string(fit.File.Status()),
		Fits: fit.Misfit == nil, Choosable: fit.Choosable(), Chosen: fit.Chosen, Value: sheetfit.Choice{SpreadsheetID: fit.File.ID}.Value(),
		Tabs: fitTabs(fit.File, fit.Assignment.Tabs), Choices: fitChoices(fit),
	}
	if fit.Misfit != nil {
		out.Misfit = toMisfitResponse(fit.Misfit)
	}
	return out
}

func fitTabs(file googlesheet.Spreadsheet, tabs map[string]string) []FitTabResponse {
	modelOf := make(map[string]string, len(tabs))
	for model, sheet := range tabs {
		modelOf[sheet] = model
	}
	out := []FitTabResponse{}
	for _, sheet := range file.Sheets {
		if model, ok := modelOf[sheet.ID]; ok {
			out = append(out, FitTabResponse{ModelSheetID: model, SheetID: sheet.ID, Title: sheet.Title})
		}
	}
	return out
}

func fitChoices(fit gsuc.FileFit) []FitTabChoiceResponse {
	out := make([]FitTabChoiceResponse, 0, len(fit.Tabs))
	for _, tab := range fit.Tabs {
		choice := FitTabChoiceResponse{ModelSheetID: tab.ModelSheetID, ModelTab: tab.ModelTab, Options: make([]FitTabOptionResponse, 0, len(tab.Options))}
		for _, option := range tab.Options {
			item := FitTabOptionResponse{
				SheetID: option.SheetID, Title: option.Title, Fits: option.Problem == nil,
				Chosen: fit.Choice.Tabs[tab.ModelSheetID] == option.SheetID, Value: fit.Choice.With(tab.ModelSheetID, option.SheetID).Value(),
			}
			if option.Problem != nil {
				problem := toProblemResponse(*option.Problem)
				item.Problem = &problem
			}
			choice.Options = append(choice.Options, item)
		}
		out = append(out, choice)
	}
	return out
}

func toProblemResponse(problem sheetfit.Problem) FitProblemResponse {
	return FitProblemResponse{
		Code: problem.Code, SheetID: problem.SheetID, Tab: problem.Tab, Column: problem.Column, Option: problem.Option, ModelTab: problem.ModelTab,
	}
}

func toMisfitResponse(misfit *sheetfit.Misfit) *MisfitResponse {
	out := &MisfitResponse{Code: misfit.Code, ModelSheetID: misfit.ModelSheetID, ModelTab: misfit.ModelTab}
	for _, problem := range misfit.Problems {
		out.Problems = append(out.Problems, toProblemResponse(problem))
	}
	for _, candidate := range misfit.Candidates {
		out.Candidates = append(out.Candidates, FitCandidateResponse{SheetID: candidate.SheetID, Title: candidate.Title})
	}
	return out
}

func writeModelFitError(w http.ResponseWriter, err error) {
	var misfit *sheetfit.Misfit
	if errors.As(err, &misfit) {
		response.WriteErrorWithDetails(w, http.StatusUnprocessableEntity, misfit.Code, misfit.Error(), misfitRefusal{ModelSheetID: misfit.ModelSheetID})
		return
	}
	writeError(w, err)
}

type sheetInUseRefusal struct {
	Campaigns []string `json:"campaigns"`
}
