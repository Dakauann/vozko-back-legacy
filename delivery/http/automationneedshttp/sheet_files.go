package automationneedshttp

import (
	"net/http"
	"strings"

	"vozko/delivery/http/googlesheet"
	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/conversation"
	"vozko/domain/variabledecl"
)

type SheetModelResponse struct {
	SpreadsheetID string `json:"spreadsheetId" example:"1AbC"`
	Title         string `json:"title" example:"Modelo Altiva"`
}

type SheetVariableResponse struct {
	Name                 string             `json:"name" example:"planilha_franquia"`
	Label                string             `json:"label" example:"Planilha da franquia"`
	Model                SheetModelResponse `json:"model"`
	DefaultSpreadsheetID string             `json:"defaultSpreadsheetId,omitempty"`
}

type SheetFilesResponse struct {
	Variable SheetVariableResponse         `json:"variable"`
	Exact    bool                          `json:"exact"`
	Items    []googlesheet.FileFitResponse `json:"items"`
}

// @Summary		Planilhas que podem ser escolhidas para um dado do tipo Planilha
// @Description	Lista as planilhas do workspace com a conferência de cada uma para o dado da conversa variable, que precisa ser do tipo Planilha. Com kind e id, a conferência usa exatamente os passos e ferramentas daquela automação que gravam nesse dado, seguindo agentes, subfluxos e fluxos chamados (exact=true); sem eles, ou quando a automação não grava nesse dado, a planilha precisa ter todas as abas e colunas do modelo (exact=false). Cada item traz fits, as abas que recebem cada aba do modelo e, quando não serve, misfit com o código e os problemas. A planilha modelo nunca serve como valor. Campanha, passagem da conversa, transferência, dados do teste e painel da conversa usam esta lista; mostra só nomes e conferência.
// @Tags			Automações
// @Produce		json
// @Param			variable	query		string	true	"Nome técnico do dado do tipo Planilha"
// @Param			kind		query		string	false	"agent ou workflow (junto com id)"	Enums(agent, workflow)
// @Param			id			query		string	false	"ID do agente ou do fluxo (junto com kind)"
// @Success		200			{object}	SheetFilesResponse
// @Failure		400			{object}	response.ErrorResponse	"conversation_variable_invalid: kind sem id; conversation_variable_not_sheet: o dado não é do tipo Planilha"
// @Failure		422			{object}	response.ErrorResponse	"automation_needs_not_found"
// @Failure		503			{object}	response.ErrorResponse	"automation_needs_unreadable"
// @Security		BearerAuth
// @Router			/automation-needs/sheet-files [get]
func (h *Handler) SheetFiles(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	kind, id := strings.TrimSpace(q.Get("kind")), strings.TrimSpace(q.Get("id"))
	var target *conversation.Automation
	if kind != "" || id != "" {
		if kind == "" || id == "" {
			response.WriteErrorWithCode(w, http.StatusBadRequest, "conversation_variable_invalid", "Informe kind e id juntos.", nil)
			return
		}
		target = &conversation.Automation{Kind: conversation.AutomationKind(kind), ID: id}
	}
	listed, err := h.queries.SheetCandidates(r.Context(), workspaceID, target, q.Get("variable"))
	if err != nil {
		writeError(w, err)
		return
	}
	settings := listed.Variable.Type.Settings
	out := SheetFilesResponse{
		Variable: SheetVariableResponse{
			Name: listed.Variable.Name, Label: listed.Variable.Label, DefaultSpreadsheetID: settings[variabledecl.SettingDefault],
			Model: SheetModelResponse{SpreadsheetID: settings[variabledecl.SettingModel], Title: settings[variabledecl.SettingModelTitle]},
		},
		Exact: listed.Exact,
		Items: make([]googlesheet.FileFitResponse, 0, len(listed.Candidates)),
	}
	for _, candidate := range listed.Candidates {
		out.Items = append(out.Items, googlesheet.FileFitOf(candidate))
	}
	response.WriteSuccess(w, http.StatusOK, out)
}
