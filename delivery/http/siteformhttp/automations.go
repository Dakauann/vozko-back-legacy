package siteformhttp

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/infra/http/middleware"
	"vozko/usecases/siteforms"
)

const formAutomations = "/sites/projects/{id}/forms/{formId}/automations"

type AutomationDesk interface {
	Automations(ctx context.Context, workspaceID, projectID, formID string) (siteforms.FormAutomations, error)
	ScaffoldAutomation(ctx context.Context, workspaceID, userID, projectID, formID string) (string, error)
}

type FormAutomationResponse struct {
	WorkflowID string `json:"workflowId" format:"uuid"`
	Name       string `json:"name" example:"Respostas do formulário contato"`
	Active     bool   `json:"active"`
}

type FormAutomationsResponse struct {
	FormKey string                   `json:"formKey" example:"f_8Jd2kq0Lr7"`
	Items   []FormAutomationResponse `json:"items"`
}

type ScaffoldedAutomationResponse struct {
	WorkflowID string `json:"workflowId" format:"uuid"`
}

// @Summary		Automações de um formulário da página
// @Description	Diz o que acontece quando o formulário da página é enviado: os fluxos cujo gatilho "formulário enviado" é este formulário (rascunhos e ativos; `active` diz se a versão publicada do fluxo roda agora). É a mesma regra do gatilho, então só estes fluxos podem ser escolhidos nas configurações do formulário. Reserva a chave pública do formulário, que o gatilho usa, mesmo antes da primeira publicação. Exige `sites:read`. Códigos: 404 `project_not_found`; 422 `form_id_invalid`.
// @Tags			Páginas
// @Produce		json
// @Param			id		path		string	true	"id da página"
// @Param			formId	path		string	true	"id do formulário no código"	example(contato)
// @Success		200		{object}	FormAutomationsResponse
// @Failure		403		{object}	response.ErrorResponse
// @Failure		404		{object}	response.ErrorResponse	"project_not_found"
// @Failure		422		{object}	response.ErrorResponse	"form_id_invalid"
// @Failure		500		{object}	response.ErrorResponse	"internal_error"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/forms/{formId}/automations [get]
func (h *Handler) Automations(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	vars := mux.Vars(r)
	view, err := h.forms.Automations(r.Context(), workspaceID, vars["id"], vars["formId"])
	if err != nil {
		writeError(w, err)
		return
	}
	out := FormAutomationsResponse{FormKey: view.FormKey, Items: make([]FormAutomationResponse, 0, len(view.Automations))}
	for _, a := range view.Automations {
		out.Items = append(out.Items, FormAutomationResponse{WorkflowID: a.WorkflowID, Name: a.Name, Active: a.Active})
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

// @Summary		Criar automação para o formulário da página
// @Description	Cria um fluxo em rascunho cujo gatilho é este formulário (mesmo modelo dos Formulários do WhatsApp: etiqueta e negociação no funil padrão), para revisar e publicar no editor de fluxos. Depois ele aparece nas automações do formulário e pode ser escolhido nas configurações. Exige `workflows:create`. Códigos: 401 `unauthenticated`; 404 `project_not_found`; 422 `form_id_invalid`, `no_default_deal_funnel`.
// @Tags			Páginas
// @Produce		json
// @Param			id		path		string	true	"id da página"
// @Param			formId	path		string	true	"id do formulário no código"	example(contato)
// @Success		201		{object}	ScaffoldedAutomationResponse
// @Failure		401		{object}	response.ErrorResponse	"unauthenticated"
// @Failure		403		{object}	response.ErrorResponse
// @Failure		404		{object}	response.ErrorResponse	"project_not_found"
// @Failure		422		{object}	response.ErrorResponse	"form_id_invalid ou no_default_deal_funnel"
// @Failure		500		{object}	response.ErrorResponse	"internal_error"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/forms/{formId}/automations [post]
func (h *Handler) ScaffoldAutomation(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	claims := middleware.GetClaims(r)
	if claims == nil || claims.UserID == "" {
		response.WriteCodedError(w, http.StatusUnauthorized, "unauthenticated", "Usuário não identificado")
		return
	}
	r = httpx.WithCreationScope(r, "")
	vars := mux.Vars(r)
	id, err := h.forms.ScaffoldAutomation(r.Context(), workspaceID, claims.UserID, vars["id"], vars["formId"])
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusCreated, ScaffoldedAutomationResponse{WorkflowID: id})
}
