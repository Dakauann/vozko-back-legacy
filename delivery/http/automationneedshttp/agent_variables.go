package automationneedshttp

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/agenthttp"
	"vozko/delivery/http/automationneedsx"
	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/agent"
	domain "vozko/domain/automationneeds"
	workspace_domain "vozko/domain/workspace"
	automationneeds_usecase "vozko/usecases/automationneeds"
)

type AgentVariableQueries interface {
	AgentVariables(ctx context.Context, workspaceID string, candidate *agent.Agent) ([]automationneeds_usecase.AgentVariable, error)
}

type AgentVariablesHandler struct {
	agents  agent.GetAgentUseCase
	queries AgentVariableQueries
}

func NewAgentVariablesHandler(agents agent.GetAgentUseCase, queries AgentVariableQueries) *AgentVariablesHandler {
	if agents == nil || queries == nil {
		panic("automationneedshttp: the agent variables handler needs the agents and the queries")
	}
	return &AgentVariablesHandler{agents: agents, queries: queries}
}

type AgentVariableRequest struct {
	Name         string `json:"name" example:"turno"`
	Description  string `json:"description,omitempty" example:"Período da aula"`
	DefaultValue string `json:"defaultValue,omitempty" example:"Manhã"`
}

type AgentVariablesRequest struct {
	MessagingPrompt string                 `json:"messagingPrompt" example:"Olá! Sua matrícula {{matricula}} é do turno {{turno}}."`
	Variables       []AgentVariableRequest `json:"variables"`
}

type AgentVariableResponse struct {
	automationneedsx.NeedResponse
	Required bool                 `json:"required"`
	InAgent  bool                 `json:"inAgent"`
	InPrompt bool                 `json:"inPrompt"`
	Used     bool                 `json:"used"`
	Default  string               `json:"default,omitempty" example:"Manhã"`
	FilledBy []FlowFillerResponse `json:"filledBy"`
	Problem  string               `json:"problem,omitempty" enums:"agent_variable_name_invalid,agent_variable_name_reserved,agent_variable_duplicate" example:"agent_variable_name_reserved"`
}

type AgentVariablesResponse struct {
	Variables []AgentVariableResponse `json:"variables"`
}

// @Summary		Variáveis iniciais do agente
// @Description	Lista as variáveis iniciais que o agente precisa ou usa, como o painel do editor mostra, lendo o prompt e as variáveis enviados no corpo (ainda não salvos). As ferramentas vêm do rascunho salvo do agente, então os fluxos que ele chama entram na lista. Cada linha traz o nome amigável e o tipo da declaração do workspace (declared falso quando o nome ainda não foi declarado; sem declaração vale texto), se é obrigatória (required; opcional quando o agente dá um valor padrão em default), se o agente a lista (inAgent), se o prompt a usa (inPrompt), se ela conta para o agente (used), a cadeia de automações (via), quem pode preenchê-la (filledBy: default, campaign, handoff, transfer) e, quando o nome não pode ser salvo, o motivo (problem: agent_variable_name_invalid, agent_variable_name_reserved ou agent_variable_duplicate). lead_name, lead_number e campaign_name só aparecem quando o agente os lista, sempre com problem, porque a plataforma os preenche.
// @Tags			Dados da conversa
// @Accept			json
// @Produce		json
// @Param			id		path		string					true	"ID do agente"
// @Param			request	body		AgentVariablesRequest	true	"Prompt e variáveis do formulário"
// @Success		200		{object}	AgentVariablesResponse
// @Failure		400		{object}	response.ErrorResponse	"invalid_body"
// @Failure		403		{object}	response.ErrorResponse	"agent_forbidden"
// @Failure		404		{object}	response.ErrorResponse	"agent_not_found"
// @Failure		422		{object}	response.ErrorResponse	"automation_needs_not_found"
// @Failure		503		{object}	response.ErrorResponse	"automation_needs_unreadable"
// @Security		BearerAuth
// @Router			/agents/{id}/conversation-variables [post]
func (h *AgentVariablesHandler) Saved(w http.ResponseWriter, r *http.Request) {
	h.variables(w, r)
}

// @Summary		Variáveis iniciais de um agente novo
// @Description	Igual a /agents/{id}/conversation-variables para um agente que ainda não foi salvo: só o prompt e as variáveis do corpo contam, porque ele ainda não tem ferramentas salvas.
// @Tags			Dados da conversa
// @Accept			json
// @Produce		json
// @Param			request	body		AgentVariablesRequest	true	"Prompt e variáveis do formulário"
// @Success		200		{object}	AgentVariablesResponse
// @Failure		400		{object}	response.ErrorResponse	"invalid_body"
// @Failure		503		{object}	response.ErrorResponse	"automation_needs_unreadable"
// @Security		BearerAuth
// @Router			/agents/conversation-variables [post]
func (h *AgentVariablesHandler) New(w http.ResponseWriter, r *http.Request) {
	h.variables(w, r)
}

func (h *AgentVariablesHandler) variables(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	candidate := &agent.Agent{WorkspaceID: workspaceID}
	if _, saved := mux.Vars(r)["id"]; saved {
		found, ok := agenthttp.AuthorizeAgent(w, r, h.agents)
		if !ok {
			return
		}
		copied := *found
		candidate = &copied
	}
	var req AgentVariablesRequest
	if !httpx.DecodeStrictJSON(w, r, &req) {
		return
	}
	candidate.MessagingPrompt = req.MessagingPrompt
	candidate.Variables = make([]agent.AgentVariable, 0, len(req.Variables))
	for _, v := range req.Variables {
		candidate.Variables = append(candidate.Variables, agent.AgentVariable{Name: v.Name, Description: v.Description, DefaultValue: v.DefaultValue})
	}
	candidate.Normalize()
	rows, err := h.queries.AgentVariables(r.Context(), workspaceID, candidate)
	if err != nil {
		writeError(w, err)
		return
	}
	out := AgentVariablesResponse{Variables: make([]AgentVariableResponse, 0, len(rows))}
	for _, row := range rows {
		out.Variables = append(out.Variables, AgentVariableResponse{
			NeedResponse: automationneedsx.Needs(domain.Needs{row.Need})[0], Required: row.Required, InAgent: row.InAgent, InPrompt: row.InPrompt,
			Used: row.Used, Default: row.Need.Default, FilledBy: fillers(row.FilledBy), Problem: problemCode(row.Problem),
		})
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func problemCode(problem error) string {
	if problem == nil {
		return ""
	}
	if refusal, ok := response.MatchRefusal(problem, agenthttp.Refusals); ok {
		return refusal.Code
	}
	return agenthttp.RefusalInternal.Code
}

func RegisterAgentVariableRoutes(
	protected *mux.Router,
	h *AgentVariablesHandler,
	ac func(workspace_domain.Resource, workspace_domain.Action, http.HandlerFunc) http.HandlerFunc,
) {
	protected.HandleFunc("/agents/conversation-variables", ac(workspace_domain.ResourceAgents, workspace_domain.ActionRead, h.New)).Methods(http.MethodPost)
	protected.HandleFunc("/agents/{id}/conversation-variables", ac(workspace_domain.ResourceAgents, workspace_domain.ActionRead, h.Saved)).Methods(http.MethodPost)
}
