package sitehttp

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	workspace_domain "vozko/domain/workspace"
	sitenext_usecase "vozko/usecases/sitenext"
)

const CodeNextStepsUnavailable = "next_steps_unavailable"

var nextStepsRefusals = []response.ErrorRefusal{
	{Err: sitenext_usecase.ErrUnavailable, Refusal: response.NewRefusal(http.StatusServiceUnavailable, CodeNextStepsUnavailable, "Não foi possível ler os próximos passos do site; tente de novo")},
}

type NextSteps interface {
	Next(ctx context.Context, workspaceID, projectID string) (sitenext_usecase.Result, error)
}

type NextStepsHandler struct {
	steps NextSteps
}

func NewNextStepsHandler(steps NextSteps) *NextStepsHandler {
	return &NextStepsHandler{steps: steps}
}

func RegisterNextStepsRoutes(protected *mux.Router, h *NextStepsHandler, ac AccessControl) {
	if h == nil {
		return
	}
	protected.HandleFunc("/sites/projects/{id}/next-steps", ac(workspace_domain.ResourceSites, workspace_domain.ActionUpdate, h.NextSteps)).Methods(http.MethodGet)
}

type NextStepActionResponse struct {
	Kind   string `json:"kind" enums:"prompt,settings,publish"`
	Prompt string `json:"prompt,omitempty" example:"Adicione perto do formulário o link da política de privacidade."`
	Tab    string `json:"tab,omitempty" enums:"general,domains,forms,bookings,integrations,consent"`
}

type NextStepResponse struct {
	Code   string                 `json:"code" example:"add_booking"`
	Label  string                 `json:"label" example:"Adicionar um bloco de agendamento"`
	Action NextStepActionResponse `json:"action"`
}

type NextStepsResponse struct {
	Checked bool               `json:"checked"`
	Steps   []NextStepResponse `json:"steps"`
}

// @Summary		Próximos passos da página
// @Description	Até três próximos passos para a página, calculados do estado real e sem chamar nenhum modelo: pendências que impedem publicar, formulários sem destino, agendamento citado sem bloco, chat do site, ícone da aba, publicação e domínio próprio. Cada passo traz um código estável, um rótulo em português e uma ação: prompt (texto pronto para a Elo), settings (aba das configurações) ou publish (abrir a publicação). Enquanto a versão de trabalho não foi lida no editor, checked fica false e a lista vem vazia; nada é sugerido sem certeza.
// @Tags			Páginas
// @Produce		json
// @Param			id	path		string	true	"id da página"
// @Success		200	{object}	NextStepsResponse
// @Failure		404	{object}	response.ErrorResponse	"project_not_found"
// @Failure		503	{object}	response.ErrorResponse	"next_steps_unavailable"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/next-steps [get]
func (h *NextStepsHandler) NextSteps(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	result, err := h.steps.Next(r.Context(), workspaceID, mux.Vars(r)["id"])
	if err != nil {
		if !response.WriteErrorRefusal(w, err, nextStepsRefusals) {
			writeError(w, err, "Failed to read the next steps")
		}
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentNextSteps(result))
}

func presentNextSteps(result sitenext_usecase.Result) NextStepsResponse {
	out := NextStepsResponse{Checked: result.Checked, Steps: make([]NextStepResponse, 0, len(result.Steps))}
	for _, step := range result.Steps {
		out.Steps = append(out.Steps, NextStepResponse{
			Code: step.Code, Label: step.Label,
			Action: NextStepActionResponse{Kind: string(step.Action.Kind), Prompt: step.Action.Prompt, Tab: string(step.Action.Tab)},
		})
	}
	return out
}
