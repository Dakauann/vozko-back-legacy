package agentrunhttp

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"vozko/delivery/http/agenthttp"
	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/agent"
	"vozko/domain/agentrun"
	"vozko/infra/http/middleware"
	agentrun_usecase "vozko/usecases/agentrun"
)

var (
	refusalFailed        = response.Refusal{Status: http.StatusInternalServerError, Code: "agent_run_failed", Message: "Erro ao ler as execuções do agente"}
	refusalFilterInvalid = response.Refusal{Status: http.StatusBadRequest, Code: "agent_run_filter_invalid", Message: "Filtro de execuções inválido"}
)

var Refusals = []response.ErrorRefusal{
	{Err: agentrun.ErrRunNotFound, Refusal: response.Refusal{Status: http.StatusNotFound, Code: "agent_run_not_found", Message: "Execução não encontrada ou fora do seu acesso"}},
	{Err: agentrun.ErrFilterInvalid, Refusal: refusalFilterInvalid},
}

type Handler struct {
	runs   agentrun_usecase.Runs
	agents agent.GetAgentUseCase
}

func NewHandler(runs agentrun_usecase.Runs, agents agent.GetAgentUseCase) *Handler {
	return &Handler{runs: runs, agents: agents}
}

// @Summary		Listar as execuções de um agente
// @Description	Lista as execuções do agente, da mais recente para a mais antiga. Cada execução é um turno da IA: modelo, situação, duração, tokens, custo informado pelo provedor, quantas ferramentas foram chamadas, quantas falharam e quantas falhas a IA contornou, e a revisão de cada ferramenta oferecida.
// @Description
// @Description	| Filtro | Valores |
// @Description	|---|---|
// @Description	| from, to | datas RFC 3339 |
// @Description	| source | conversation, simulator, scenario, workflow |
// @Description	| status | ok, tool_failed, failed |
// @Description	| toolId | id da ferramenta do agente |
// @Description	| errorKind | tipo do erro de uma chamada (timeout, http_status, arguments, ...) |
// @Description	| entryId | id da conversa |
// @Description	| handledFailure | true para só execuções com falha contornada |
// @Description
// @Description	Execuções de conversas aparecem só para quem pode ver a conversa (departamento e atribuição). Execuções guardadas por 180 dias; o conteúdo das chamadas, por 30.
// @Tags			Agentes
// @Produce		json
// @Param			id				path		string	true	"ID do agente"
// @Param			from			query		string	false	"Início (RFC 3339)"
// @Param			to				query		string	false	"Fim (RFC 3339)"
// @Param			source			query		string	false	"Origem"
// @Param			status			query		string	false	"Situação"
// @Param			toolId			query		string	false	"Ferramenta"
// @Param			errorKind		query		string	false	"Tipo de erro"
// @Param			entryId			query		string	false	"Conversa"
// @Param			handledFailure	query		bool	false	"Só falhas contornadas"
// @Param			page			query		int		false	"Página, a partir de 1"
// @Param			pageSize		query		int		false	"Itens por página"
// @Success		200	{object}	response.PaginatedPayload{data=[]agentrun.Run}
// @Failure		400	{object}	response.ErrorResponse	"agent_run_filter_invalid"
// @Failure		403	{object}	response.ErrorResponse	"agent_forbidden"
// @Failure		404	{object}	response.ErrorResponse	"agent_not_found"
// @Security		BearerAuth
// @Router			/agents/{id}/runs [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agenthttp.Authorize(w, r, h.agents)
	if !ok {
		return
	}
	person, ok := httpx.RequirePerson(w, r)
	if !ok {
		return
	}
	filter, err := filterOf(r.URL.Query())
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := h.runs.List(person, middleware.GetWorkspaceID(r), agentID, filter, httpx.ParsePagination(r.URL.Query()))
	if err != nil {
		writeError(w, err)
		return
	}
	response.WritePaginated(w, http.StatusOK, result.Items, response.PaginationMeta{
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
		TotalItems: result.TotalItems,
	})
}

// @Summary		Ler uma execução de um agente
// @Description	Devolve a execução com as chamadas do modelo (latência, tokens, motivo de parada) e as chamadas de ferramenta na ordem em que aconteceram: argumentos, requisição e resposta HTTP com segredos mascarados, saída mapeada, exatamente o que a IA leu, tipo e cadeia do erro, o que a IA fez depois de uma falha (`followedBy`) e a revisão da ferramenta que rodou.
// @Tags			Agentes
// @Produce		json
// @Param			id		path		string	true	"ID do agente"
// @Param			runId	path		string	true	"ID da execução"
// @Success		200		{object}	agentrun.Detail
// @Failure		403		{object}	response.ErrorResponse	"agent_forbidden"
// @Failure		404		{object}	response.ErrorResponse	"agent_run_not_found"
// @Security		BearerAuth
// @Router			/agents/{id}/runs/{runId} [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agenthttp.Authorize(w, r, h.agents)
	if !ok {
		return
	}
	person, ok := httpx.RequirePerson(w, r)
	if !ok {
		return
	}
	detail, err := h.runs.Get(person, middleware.GetWorkspaceID(r), agentID, mux.Vars(r)["runId"])
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, detail)
}

func filterOf(q url.Values) (agentrun.Filter, error) {
	filter := agentrun.Filter{
		Source:         agentrun.Source(q.Get("source")),
		Status:         agentrun.Status(q.Get("status")),
		ToolInstanceID: q.Get("toolId"),
		ErrorKind:      q.Get("errorKind"),
		EntryID:        q.Get("entryId"),
	}
	var err error
	if filter.From, err = timeOf(q.Get("from")); err != nil {
		return filter, err
	}
	if filter.To, err = timeOf(q.Get("to")); err != nil {
		return filter, err
	}
	if raw := q.Get("handledFailure"); raw != "" {
		if filter.HandledFailure, err = strconv.ParseBool(raw); err != nil {
			return filter, agentrun.ErrFilterInvalid
		}
	}
	return filter, nil
}

func timeOf(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, agentrun.ErrFilterInvalid
	}
	return parsed, nil
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
