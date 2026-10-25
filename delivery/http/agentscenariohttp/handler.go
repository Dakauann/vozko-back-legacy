package agentscenariohttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/agenthttp"
	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/agent"
	"vozko/domain/agentrun"
	"vozko/domain/agentscenario"
	"vozko/domain/shared"
	"vozko/infra/http/middleware"
	agentscenario_usecase "vozko/usecases/agentscenario"
)

const maxScenarioBodyBytes = 4 << 20

var (
	refusalFailed      = response.Refusal{Status: http.StatusInternalServerError, Code: "agent_scenario_failed", Message: "Erro ao processar os cenários do agente"}
	refusalInvalidBody = response.Refusal{Status: http.StatusBadRequest, Code: "agent_scenario_invalid", Message: "Cenário inválido"}
)

var Refusals = []response.ErrorRefusal{
	{Err: agentscenario.ErrNotFound, Refusal: response.Refusal{Status: http.StatusNotFound, Code: "agent_scenario_not_found", Message: "Cenário não encontrado neste agente"}},
	{Err: agentscenario.ErrTooLong, Refusal: response.Refusal{Status: http.StatusBadRequest, Code: "agent_scenario_too_long", Message: "Um cenário tem no máximo 60 mensagens"}},
	{Err: agentscenario.ErrMockTooLarge, Refusal: response.Refusal{Status: http.StatusRequestEntityTooLarge, Code: "agent_scenario_mock_too_large", Message: "A resposta simulada de uma ferramenta passa de 64 KB"}},
	{Err: agentscenario.ErrInvalid, Refusal: refusalInvalidBody},
	{Err: agentrun.ErrRunNotFound, Refusal: response.Refusal{Status: http.StatusNotFound, Code: "agent_run_not_found", Message: "Execução não encontrada ou fora do seu acesso"}},
}

type Scenarios interface {
	List(workspaceID, agentID string) ([]*agentscenario.Scenario, error)
	Create(workspaceID, agentID string, draft agentscenario_usecase.Draft, by string) (*agentscenario.Scenario, error)
	Update(workspaceID, agentID, scenarioID string, draft agentscenario_usecase.Draft) (*agentscenario.Scenario, error)
	Delete(workspaceID, agentID, scenarioID string) error
	Runs(workspaceID, agentID, scenarioID string) ([]*agentscenario.Run, error)
	Run(ctx context.Context, workspaceID, agentID, scenarioID string) (*agentscenario.Run, error)
	RunAll(ctx context.Context, workspaceID, agentID string) ([]*agentscenario.Run, error)
	FromRun(person shared.Person, workspaceID, agentID, runID string) (*agentscenario.Scenario, error)
}

type Handler struct {
	scenarios Scenarios
	agents    agent.GetAgentUseCase
}

func NewHandler(scenarios Scenarios, agents agent.GetAgentUseCase) *Handler {
	return &Handler{scenarios: scenarios, agents: agents}
}

type ScenarioRequest struct {
	Name       string                        `json:"name" example:"Pedido atrasado"`
	Messages   []agentscenario.Message       `json:"messages"`
	Mocks      map[string]agentscenario.Mock `json:"mocks"`
	Assertions []agentscenario.Assertion     `json:"assertions"`
	SourceRun  string                        `json:"sourceRunId"`
}

type ScenarioListResponse struct {
	Scenarios []*agentscenario.Scenario `json:"scenarios"`
}

type RunListResponse struct {
	Runs []*agentscenario.Run `json:"runs"`
}

// @Summary		Listar os cenários de teste de um agente
// @Description	Lista os cenários salvos do agente com o resultado da última execução e da anterior, para mostrar o que passou a falhar e o que voltou a passar.
// @Tags			Agentes
// @Produce		json
// @Param			id	path		string	true	"ID do agente"
// @Success		200	{object}	ScenarioListResponse
// @Failure		403	{object}	response.ErrorResponse	"agent_forbidden"
// @Failure		404	{object}	response.ErrorResponse	"agent_not_found"
// @Security		BearerAuth
// @Router			/agents/{id}/scenarios [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agenthttp.Authorize(w, r, h.agents)
	if !ok {
		return
	}
	list, err := h.scenarios.List(middleware.GetWorkspaceID(r), agentID)
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []*agentscenario.Scenario{}
	}
	response.WriteSuccess(w, http.StatusOK, ScenarioListResponse{Scenarios: list})
}

// @Summary		Criar um cenário de teste
// @Description	Um cenário é uma conversa roteirizada: mensagens do cliente (até 60), respostas simuladas por ferramenta (`mocks`, pelo id da ferramenta, até 64 KB cada) e verificações.
// @Description
// @Description	| Verificação | Campos |
// @Description	|---|---|
// @Description	| tool_called | toolId, e opcionalmente arguments com match exact, subset ou regex |
// @Description	| tool_not_called | toolId |
// @Description	| tool_output_matches | toolId, path (ex.: data.itens[0].id), match exact ou regex, value |
// @Description	| reply_matches | match contains ou regex, value (confere a última resposta) |
// @Description	| reply_judged | rubric: um avaliador de IA lê a conversa e decide; o uso é cobrado como no simulador |
// @Description
// @Description	Ferramentas com resposta simulada não chamam nada de verdade. Sem resposta simulada, só consultas (GET e HEAD, base de conhecimento, agenda, CEP) rodam de verdade; o resto devolve um resultado de simulação.
// @Tags			Agentes
// @Accept			json
// @Produce		json
// @Param			id		path		string			true	"ID do agente"
// @Param			request	body		ScenarioRequest	true	"Cenário"
// @Success		201		{object}	agentscenario.Scenario
// @Failure		400		{object}	response.ErrorResponse	"agent_scenario_invalid, agent_scenario_too_long"
// @Failure		413		{object}	response.ErrorResponse	"agent_scenario_mock_too_large"
// @Security		BearerAuth
// @Router			/agents/{id}/scenarios [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agenthttp.Authorize(w, r, h.agents)
	if !ok {
		return
	}
	var req ScenarioRequest
	if !decode(w, r, &req) {
		return
	}
	created, err := h.scenarios.Create(middleware.GetWorkspaceID(r), agentID, draftOf(req), httpx.RequesterOf(r).UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusCreated, created)
}

// @Summary		Alterar um cenário de teste
// @Description	Substitui nome, mensagens, respostas simuladas e verificações do cenário. As execuções anteriores continuam no histórico.
// @Tags			Agentes
// @Accept			json
// @Produce		json
// @Param			id			path		string			true	"ID do agente"
// @Param			scenarioId	path		string			true	"ID do cenário"
// @Param			request		body		ScenarioRequest	true	"Cenário"
// @Success		200			{object}	agentscenario.Scenario
// @Failure		400			{object}	response.ErrorResponse	"agent_scenario_invalid, agent_scenario_too_long"
// @Failure		404			{object}	response.ErrorResponse	"agent_scenario_not_found"
// @Failure		413			{object}	response.ErrorResponse	"agent_scenario_mock_too_large"
// @Security		BearerAuth
// @Router			/agents/{id}/scenarios/{scenarioId} [put]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agenthttp.Authorize(w, r, h.agents)
	if !ok {
		return
	}
	var req ScenarioRequest
	if !decode(w, r, &req) {
		return
	}
	updated, err := h.scenarios.Update(middleware.GetWorkspaceID(r), agentID, mux.Vars(r)["scenarioId"], draftOf(req))
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, updated)
}

// @Summary		Remover um cenário de teste
// @Description	Remove o cenário e o histórico das suas execuções.
// @Tags			Agentes
// @Param			id			path	string	true	"ID do agente"
// @Param			scenarioId	path	string	true	"ID do cenário"
// @Success		204
// @Failure		404	{object}	response.ErrorResponse	"agent_scenario_not_found"
// @Security		BearerAuth
// @Router			/agents/{id}/scenarios/{scenarioId} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agenthttp.Authorize(w, r, h.agents)
	if !ok {
		return
	}
	if err := h.scenarios.Delete(middleware.GetWorkspaceID(r), agentID, mux.Vars(r)["scenarioId"]); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary		Histórico de execuções de um cenário
// @Description	As 20 execuções mais recentes do cenário, com o resultado de cada verificação, as respostas do agente por mensagem e os ids das execuções do agente (abra-os em /agents/{id}/runs/{runId}). Guardadas por 180 dias.
// @Tags			Agentes
// @Produce		json
// @Param			id			path		string	true	"ID do agente"
// @Param			scenarioId	path		string	true	"ID do cenário"
// @Success		200			{object}	RunListResponse
// @Failure		404			{object}	response.ErrorResponse	"agent_scenario_not_found"
// @Security		BearerAuth
// @Router			/agents/{id}/scenarios/{scenarioId}/runs [get]
func (h *Handler) Runs(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agenthttp.Authorize(w, r, h.agents)
	if !ok {
		return
	}
	runs, err := h.scenarios.Runs(middleware.GetWorkspaceID(r), agentID, mux.Vars(r)["scenarioId"])
	if err != nil {
		writeError(w, err)
		return
	}
	if runs == nil {
		runs = []*agentscenario.Run{}
	}
	response.WriteSuccess(w, http.StatusOK, RunListResponse{Runs: runs})
}

// @Summary		Executar um cenário de teste
// @Description	Roda o cenário no rascunho do agente, mensagem por mensagem, pelo mesmo caminho do simulador, e confere cada verificação. Cada mensagem vira uma execução do agente com origem `scenario`.
// @Tags			Agentes
// @Produce		json
// @Param			id			path		string	true	"ID do agente"
// @Param			scenarioId	path		string	true	"ID do cenário"
// @Success		200			{object}	agentscenario.Run
// @Failure		404			{object}	response.ErrorResponse	"agent_scenario_not_found"
// @Security		BearerAuth
// @Router			/agents/{id}/scenarios/{scenarioId}/run [post]
func (h *Handler) Run(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agenthttp.Authorize(w, r, h.agents)
	if !ok {
		return
	}
	run, err := h.scenarios.Run(r.Context(), middleware.GetWorkspaceID(r), agentID, mux.Vars(r)["scenarioId"])
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, run)
}

// @Summary		Executar todos os cenários de um agente
// @Description	Roda todos os cenários do agente, até três ao mesmo tempo, e devolve uma execução por cenário.
// @Tags			Agentes
// @Produce		json
// @Param			id	path		string	true	"ID do agente"
// @Success		200	{object}	RunListResponse
// @Security		BearerAuth
// @Router			/agents/{id}/scenarios/run [post]
func (h *Handler) RunAll(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agenthttp.Authorize(w, r, h.agents)
	if !ok {
		return
	}
	runs, err := h.scenarios.RunAll(r.Context(), middleware.GetWorkspaceID(r), agentID)
	if err != nil {
		writeError(w, err)
		return
	}
	if runs == nil {
		runs = []*agentscenario.Run{}
	}
	response.WriteSuccess(w, http.StatusOK, RunListResponse{Runs: runs})
}

// @Summary		Montar um cenário a partir de uma execução
// @Description	Monta um rascunho de cenário (não salvo) com a mensagem do cliente da execução (documentos e credenciais mascarados), a resposta registrada de cada ferramenta como resposta simulada e uma verificação de que as mesmas ferramentas são chamadas. Revise e salve com POST /agents/{id}/scenarios.
// @Tags			Agentes
// @Produce		json
// @Param			id		path		string	true	"ID do agente"
// @Param			runId	path		string	true	"ID da execução"
// @Success		200		{object}	agentscenario.Scenario
// @Failure		404		{object}	response.ErrorResponse	"agent_run_not_found"
// @Security		BearerAuth
// @Router			/agents/{id}/runs/{runId}/scenario [post]
func (h *Handler) FromRun(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agenthttp.Authorize(w, r, h.agents)
	if !ok {
		return
	}
	person, ok := httpx.RequirePerson(w, r)
	if !ok {
		return
	}
	draft, err := h.scenarios.FromRun(person, middleware.GetWorkspaceID(r), agentID, mux.Vars(r)["runId"])
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, draft)
}

func draftOf(req ScenarioRequest) agentscenario_usecase.Draft {
	return agentscenario_usecase.Draft{Name: req.Name, Messages: req.Messages, Mocks: req.Mocks, Assertions: req.Assertions, SourceRun: req.SourceRun}
}

func decode(w http.ResponseWriter, r *http.Request, into interface{}) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, maxScenarioBodyBytes)).Decode(into); err != nil {
		response.WriteRefusal(w, refusalInvalidBody)
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, err error) {
	if response.WriteErrorRefusal(w, err, Refusals) {
		return
	}
	if errors.Is(err, agent.ErrAgentNotFound) {
		agenthttp.WriteError(w, err)
		return
	}
	response.WriteRefusal(w, refusalFailed)
}
