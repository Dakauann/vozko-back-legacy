package agenttoolhttp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"vozko/delivery/http/agenthttp"
	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/tools"
	"vozko/infra/http/middleware"
	agent_usecase "vozko/usecases/agent"
	tools_usecase "vozko/usecases/tools"
)

const maxToolBodyBytes = 256 * 1024

type Handler struct {
	validation agenttool.ToolsValidation
	tester     agent_usecase.ToolTester
	tools      agent.AgentToolsUseCase
	agents     agent.GetAgentUseCase
	registry   tools.Service
	mcp        tools_usecase.MCPSourceCatalog
}

type Deps struct {
	Tools      agent.AgentToolsUseCase
	Agents     agent.GetAgentUseCase
	Registry   tools.Service
	Validation agenttool.ToolsValidation
	Tester     agent_usecase.ToolTester
	MCPSources tools_usecase.MCPSourceCatalog
}

func NewHandler(d Deps) (*Handler, error) {
	missing := []string{}
	for _, dep := range []struct {
		name    string
		missing bool
	}{
		{"tools use case", d.Tools == nil}, {"agent reader", d.Agents == nil}, {"tool registry", d.Registry == nil},
		{"tool validation", d.Validation == nil}, {"tool tester", d.Tester == nil}, {"MCP source catalog", d.MCPSources == nil},
	} {
		if dep.missing {
			missing = append(missing, dep.name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("agent tool handler: missing %s", strings.Join(missing, ", "))
	}
	return &Handler{tools: d.Tools, agents: d.Agents, registry: d.Registry, validation: d.Validation, tester: d.Tester, mcp: d.MCPSources}, nil
}

type MCPSourcesResponse struct {
	Sources []tools_usecase.MCPSourceView `json:"sources"`
}

// @Summary		Listar os servidores MCP disponíveis para as ferramentas
// @Description	Lista os servidores MCP conectados ao workspace com as ferramentas de cada um, para configurar uma ferramenta do tipo Servidor MCP (`mcp_source`): o servidor (`source_id`) e as ferramentas permitidas (`tools`) ou todas (`all_tools`). Um servidor que não respondeu aparece com `unavailable: true` e sem ferramentas.
// @Tags			Agentes
// @Produce		json
// @Success		200	{object}	MCPSourcesResponse
// @Security		BearerAuth
// @Router			/agents/tools/mcp-sources [get]
func (h *Handler) MCPSources(w http.ResponseWriter, r *http.Request) {
	sources, err := tools_usecase.ListMCPSources(r.Context(), h.mcp, middleware.GetWorkspaceID(r))
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, MCPSourcesResponse{Sources: sources})
}

// @Summary		Listar as ferramentas de um agente
// @Description	Lista as ferramentas do agente na ordem em que a IA as recebe. Cada ferramenta é uma instância com id, tipo, nome da função que a IA chama, descrição para a IA, configuração, ordem e revisão.
// @Description
// @Description	`testMode` diz como a ferramenta é testada sozinha em /agents/{id}/tools/{toolId}/test, pela mesma regra que o teste aplica:
// @Description
// @Description	| testMode | O teste |
// @Description	|---|---|
// @Description	| run | Chama a API de verdade; métodos que alteram dados pedem `confirmRealCall` |
// @Description	| preview | Mostra o que a ferramenta faria, sem executar nada |
// @Description	| scenario | Não roda sozinha: o teste responde 422 `agent_tool_test_unsupported`; teste num cenário do simulador |
// @Description
// @Description	Credenciais nunca voltam: cabeçalhos de autenticação e valores de consulta sensíveis aparecem só como referência a um segredo do workspace ({{secret.<nome>}}), e um valor antigo ainda em texto aparece como ••••.
// @Tags			Agentes
// @Produce		json
// @Param			id	path		string	true	"ID do agente"
// @Success		200	{object}	ToolListResponse
// @Failure		403	{object}	response.ErrorResponse	"agent_forbidden"
// @Failure		404	{object}	response.ErrorResponse	"agent_not_found"
// @Security		BearerAuth
// @Router			/agents/{id}/tools [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	owner, ok := agenthttp.AuthorizeAgent(w, r, h.agents)
	if !ok {
		return
	}
	list, err := h.tools.List(r.Context(), owner.ID)
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	h.writeList(w, r, owner, list)
}

// @Summary		Adicionar uma ferramenta ao agente
// @Description	Cria uma instância de ferramenta. Tipos que aceitam várias instâncias (como Requisição HTTP) podem aparecer quantas vezes forem precisas, cada uma com seu nome, descrição e configuração. O nome da função é opcional: sem ele o backend propõe um a partir da descrição.
// @Description
// @Description	| Regra | Código |
// @Description	|---|---|
// @Description	| Nome com letras minúsculas, dígitos e _, começando por letra, até 64 caracteres | agent_tool_name_invalid |
// @Description	| Nome já usado por outra função do agente | agent_tool_name_taken |
// @Description	| Tipo de uma instância só que já está no agente | agent_tool_single_type_taken |
// @Description	| Mesmo destino (por exemplo o mesmo fluxo) já usado | agent_tool_target_taken |
// @Description	| Requisição HTTP sem descrição | agent_tool_description_required |
// @Description
// @Description	Credenciais digitadas em cabeçalhos de autenticação ou na consulta da URL vão para os segredos do workspace, e a configuração guarda só a referência.
// @Tags			Agentes
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"ID do agente"
// @Param			request	body		CreateToolRequest	true	"Ferramenta"
// @Success		201		{object}	ToolResponse
// @Failure		400		{object}	response.ErrorResponse	"agent_invalid_body, agent_tool_type_unknown, agent_tool_name_invalid, agent_tool_description_required, agent_tool_invalid"
// @Failure		409		{object}	response.ErrorResponse	"agent_tool_name_taken, agent_tool_target_taken, agent_tool_single_type_taken"
// @Security		BearerAuth
// @Router			/agents/{id}/tools [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	owner, ok := agenthttp.AuthorizeAgent(w, r, h.agents)
	if !ok {
		return
	}
	var req CreateToolRequest
	if !decode(w, r, &req) {
		return
	}
	created, err := h.tools.Create(r.Context(), owner.ID, agent.ToolDraft{
		Type: req.Type, Name: req.Name, Description: req.Description, Config: req.Config, Visibility: visibilityOf(req.Visibility),
	}, httpx.RequesterOf(r))
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	h.writeOne(w, r, http.StatusCreated, owner, created)
}

// @Summary		Substituir uma ferramenta do agente
// @Description	Troca nome, descrição, configuração e visibilidade de uma ferramenta. Envie a revisão que você leu: se outra pessoa salvou antes, a resposta é 409 agent_tool_revision_conflict. Cada mudança grava uma revisão imutável.
// @Tags			Agentes
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"ID do agente"
// @Param			toolId	path		string				true	"ID da ferramenta"
// @Param			request	body		UpdateToolRequest	true	"Ferramenta"
// @Success		200		{object}	ToolResponse
// @Failure		404		{object}	response.ErrorResponse	"agent_tool_not_found"
// @Failure		409		{object}	response.ErrorResponse	"agent_tool_revision_conflict, agent_tool_name_taken, agent_tool_target_taken"
// @Security		BearerAuth
// @Router			/agents/{id}/tools/{toolId} [put]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	owner, ok := agenthttp.AuthorizeAgent(w, r, h.agents)
	if !ok {
		return
	}
	var req UpdateToolRequest
	if !decode(w, r, &req) {
		return
	}
	updated, err := h.tools.Update(r.Context(), owner.ID, mux.Vars(r)["toolId"], agent.ToolEdit{
		Name: req.Name, Description: req.Description, Config: req.Config, Visibility: visibilityOf(req.Visibility), Revision: req.Revision,
	}, httpx.RequesterOf(r))
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	h.writeOne(w, r, http.StatusOK, owner, updated)
}

// @Summary		Remover uma ferramenta do agente
// @Description	Remove a ferramenta do agente. As revisões continuam guardadas para os rastros das execuções.
// @Tags			Agentes
// @Param			id		path	string	true	"ID do agente"
// @Param			toolId	path	string	true	"ID da ferramenta"
// @Success		204
// @Failure		404	{object}	response.ErrorResponse	"agent_tool_not_found"
// @Security		BearerAuth
// @Router			/agents/{id}/tools/{toolId} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	if err := h.tools.Delete(r.Context(), agentID, mux.Vars(r)["toolId"], httpx.RequesterOf(r)); err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary		Duplicar uma ferramenta do agente
// @Description	Copia uma ferramenta de um tipo que aceita várias instâncias, com um nome livre (<nome>_2). Tipos de uma instância só ou presos a um destino respondem 409.
// @Tags			Agentes
// @Produce		json
// @Param			id		path		string	true	"ID do agente"
// @Param			toolId	path		string	true	"ID da ferramenta"
// @Success		201		{object}	ToolResponse
// @Failure		409		{object}	response.ErrorResponse	"agent_tool_single_type_taken, agent_tool_target_taken"
// @Security		BearerAuth
// @Router			/agents/{id}/tools/{toolId}/duplicate [post]
func (h *Handler) Duplicate(w http.ResponseWriter, r *http.Request) {
	owner, ok := agenthttp.AuthorizeAgent(w, r, h.agents)
	if !ok {
		return
	}
	copied, err := h.tools.Duplicate(r.Context(), owner.ID, mux.Vars(r)["toolId"], httpx.RequesterOf(r))
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	h.writeOne(w, r, http.StatusCreated, owner, copied)
}

// @Summary		Reordenar as ferramentas do agente
// @Description	Define a ordem em que a IA recebe as ferramentas. Envie todos os ids do agente, cada um uma vez.
// @Tags			Agentes
// @Accept			json
// @Produce		json
// @Param			id		path		string			true	"ID do agente"
// @Param			request	body		OrderRequest	true	"Ids na nova ordem"
// @Success		200		{object}	ToolListResponse
// @Failure		400		{object}	response.ErrorResponse	"agent_tool_order_mismatch"
// @Security		BearerAuth
// @Router			/agents/{id}/tools/order [put]
func (h *Handler) Reorder(w http.ResponseWriter, r *http.Request) {
	owner, ok := agenthttp.AuthorizeAgent(w, r, h.agents)
	if !ok {
		return
	}
	var req OrderRequest
	if !decode(w, r, &req) {
		return
	}
	list, err := h.tools.Reorder(r.Context(), owner.ID, req.IDs, httpx.RequesterOf(r))
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	h.writeList(w, r, owner, list)
}

// @Summary		Listar as revisões de uma ferramenta
// @Description	Lista as revisões imutáveis da ferramenta, da mais nova para a mais antiga. Cada revisão guarda nome, descrição e configuração (só referências a segredos).
// @Tags			Agentes
// @Produce		json
// @Param			id		path		string	true	"ID do agente"
// @Param			toolId	path		string	true	"ID da ferramenta"
// @Success		200		{object}	RevisionListResponse
// @Failure		404		{object}	response.ErrorResponse	"agent_tool_not_found"
// @Security		BearerAuth
// @Router			/agents/{id}/tools/{toolId}/revisions [get]
func (h *Handler) Revisions(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	list, err := h.tools.Revisions(r.Context(), agentID, mux.Vars(r)["toolId"])
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	out := RevisionListResponse{Revisions: make([]RevisionResponse, 0, len(list))}
	for _, rev := range list {
		out.Revisions = append(out.Revisions, revisionResponse(h.concealRevision(rev)))
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

// @Summary		Ler uma revisão de uma ferramenta
// @Description	Devolve a configuração exata que a ferramenta tinha na revisão pedida.
// @Tags			Agentes
// @Produce		json
// @Param			id			path		string	true	"ID do agente"
// @Param			toolId		path		string	true	"ID da ferramenta"
// @Param			revision	path		int		true	"Número da revisão"
// @Success		200			{object}	RevisionResponse
// @Failure		404			{object}	response.ErrorResponse	"agent_tool_revision_not_found"
// @Security		BearerAuth
// @Router			/agents/{id}/tools/{toolId}/revisions/{revision} [get]
func (h *Handler) Revision(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(mux.Vars(r)["revision"])
	if err != nil || n < 1 {
		agenthttp.WriteError(w, agenttool.ErrRevisionNotFound)
		return
	}
	rev, err := h.tools.Revision(r.Context(), agentID, mux.Vars(r)["toolId"], n)
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, revisionResponse(h.concealRevision(rev)))
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) (string, bool) {
	return agenthttp.Authorize(w, r, h.agents)
}

func (h *Handler) definitions() []tools.Definition {
	return h.registry.Definitions()
}

func (h *Handler) responses(owner *agent.Agent, list []agent.ToolBinding) []ToolResponse {
	concealed := agenttool.ConcealBindings(h.definitions(), list)
	out := make([]ToolResponse, 0, len(concealed))
	for i, b := range concealed {
		out = append(out, toolResponse(b, i, h.inputSchema(owner, list[i]), agenttool.TestModeIn(h.registry, b.Name)))
	}
	return out
}

func (h *Handler) inputSchema(owner *agent.Agent, b agent.ToolBinding) map[string]interface{} {
	def, ok := tools_usecase.ModelDefinition(h.registry, b, tools_usecase.ToolResolverOptions{Agent: owner})
	if !ok {
		return nil
	}
	return def.JSONSchema()
}

func (h *Handler) writeOne(w http.ResponseWriter, r *http.Request, status int, owner *agent.Agent, b agent.ToolBinding) {
	position := 0
	if list, err := h.tools.List(r.Context(), owner.ID); err == nil {
		for i, item := range list {
			if item.ID == b.ID {
				position = i
			}
		}
	}
	response.WriteSuccess(w, status, toolResponse(agenttool.ConcealBindings(h.definitions(), []agent.ToolBinding{b})[0], position, h.inputSchema(owner, b), agenttool.TestModeIn(h.registry, b.Name)))
}

func (h *Handler) concealRevision(rev agent.ToolRevision) agent.ToolRevision {
	concealed := agenttool.ConcealBindings(h.definitions(), []agent.ToolBinding{{Name: rev.Type, Config: rev.Config}})
	rev.Config = concealed[0].Config
	return rev
}

func decode(w http.ResponseWriter, r *http.Request, into interface{}) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, maxToolBodyBytes)).Decode(into); err != nil {
		response.WriteRefusal(w, agenthttp.RefusalInvalidBody)
		return false
	}
	return true
}

// @Summary		Conferir as ferramentas de um agente
// @Description	Roda o mesmo validador do salvamento, da ativação e da Elo. Sem corpo, confere as ferramentas salvas; com `tools`, confere esse conjunto como rascunho, sem salvar. Cada problema traz código, gravidade (`blocking`, `advisory`), a ferramenta (`toolId`) e uma dica.
// @Description
// @Description	Se uma conferência externa (segredos, endereço do servidor, destino da ferramenta, catálogo de modelos) não responde, o resultado traz `agent_tool_validation_unavailable` como bloqueante: nada passa sem conferir.
// @Tags			Agentes
// @Accept			json
// @Produce		json
// @Param			id		path		string			true	"ID do agente"
// @Param			request	body		ValidateRequest	false	"Conjunto rascunho (opcional)"
// @Success		200		{object}	ValidationResponse
// @Failure		400		{object}	response.ErrorResponse	"agent_invalid_body, agent_tool_type_unknown"
// @Security		BearerAuth
// @Router			/agents/{id}/tools/validate [post]
func (h *Handler) Validate(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	var req ValidateRequest
	if r.ContentLength != 0 && !decode(w, r, &req) {
		return
	}
	report, err := h.validation.Validate(r.Context(), agentID, draftBindings(req.Tools))
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, validationResponse(report))
}

func (h *Handler) writeList(w http.ResponseWriter, r *http.Request, owner *agent.Agent, list []agent.ToolBinding) {
	report, err := h.validation.Validate(r.Context(), owner.ID, nil)
	if err != nil {
		report = agenttool.Unavailable("o agente não pôde ser conferido")
	}
	checked := validationResponse(report)
	response.WriteSuccess(w, http.StatusOK, ToolListResponse{Tools: h.responses(owner, list), Green: checked.Green, Issues: checked.Issues})
}

// @Summary		Testar uma ferramenta salva
// @Description	Roda a ferramenta sozinha pelo mesmo caminho de uma conversa: confere os argumentos contra o esquema, monta a requisição com os segredos do workspace, chama a API pelo mesmo cliente com proteção de rede e aplica o mapeamento da resposta. Use `arguments` ou um exemplo salvo (`testInputId`).
// @Description
// @Description	POST, PUT, PATCH e DELETE alteram dados no sistema externo: sem `confirmRealCall: true` a resposta é 422 `agent_tool_test_real_call_not_confirmed`. Uma configuração com problema bloqueante não é chamada e volta com a lista em `issues`.
// @Description
// @Description	A resposta traz a requisição e a resposta (com segredos mascarados), a saída mapeada, exatamente o que a IA leria (`modelResult`), o tipo e a cadeia do erro e a duração.
// @Tags			Agentes
// @Accept			json
// @Produce		json
// @Param			id		path		string		true	"ID do agente"
// @Param			toolId	path		string		true	"ID da ferramenta"
// @Param			request	body		TestRequest	true	"Argumentos ou exemplo salvo"
// @Success		200		{object}	TestResponse
// @Failure		404		{object}	response.ErrorResponse	"agent_tool_not_found, agent_tool_test_input_not_found"
// @Failure		413		{object}	response.ErrorResponse	"agent_tool_test_input_too_large"
// @Failure		422		{object}	response.ErrorResponse	"agent_tool_test_real_call_not_confirmed, agent_tool_test_unsupported"
// @Security		BearerAuth
// @Router			/agents/{id}/tools/{toolId}/test [post]
func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	var req TestRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.tester.Test(r.Context(), agentID, mux.Vars(r)["toolId"], agent_usecase.ToolTestRequest{
		Arguments: req.Arguments, TestInputID: req.TestInputID, ConfirmRealCall: req.ConfirmRealCall, Context: testContext(req.Context),
	})
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, testResponse(res))
}

// @Summary		Testar uma ferramenta antes de salvar
// @Description	Mesmo teste de uma ferramenta salva, para a configuração que está no formulário e ainda não foi salva. O resultado é idêntico ao do teste da ferramenta salva com a mesma configuração.
// @Tags			Agentes
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"ID do agente"
// @Param			request	body		DraftTestRequest	true	"Ferramenta e argumentos"
// @Success		200		{object}	TestResponse
// @Failure		422		{object}	response.ErrorResponse	"agent_tool_test_real_call_not_confirmed, agent_tool_test_unsupported"
// @Security		BearerAuth
// @Router			/agents/{id}/tools/test [post]
func (h *Handler) TestDraft(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	var req DraftTestRequest
	if !decode(w, r, &req) {
		return
	}
	drafts := draftBindings([]DraftTool{req.Tool})
	res, err := h.tester.TestDraft(r.Context(), agentID, drafts[0], agent_usecase.ToolTestRequest{
		Arguments: req.Arguments, ConfirmRealCall: req.ConfirmRealCall, Context: testContext(req.Context),
	})
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, testResponse(res))
}

// @Summary		Salvar os exemplos de teste de uma ferramenta
// @Description	Substitui os exemplos de argumentos salvos da ferramenta (nome único, JSON até 64 KB cada). Os exemplos não entram nas revisões nem chegam à IA.
// @Tags			Agentes
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"ID do agente"
// @Param			toolId	path		string				true	"ID da ferramenta"
// @Param			request	body		TestInputsRequest	true	"Exemplos"
// @Success		200		{object}	TestInputsResponse
// @Failure		400		{object}	response.ErrorResponse	"agent_tool_test_input_invalid"
// @Security		BearerAuth
// @Router			/agents/{id}/tools/{toolId}/test-inputs [put]
func (h *Handler) SaveTestInputs(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	var req TestInputsRequest
	if !decode(w, r, &req) {
		return
	}
	saved, err := h.tools.SaveTestInputs(r.Context(), agentID, mux.Vars(r)["toolId"], req.TestInputs)
	if err != nil {
		agenthttp.WriteError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, TestInputsResponse{TestInputs: saved})
}

func testContext(c TestContext) agent_usecase.ToolTestContext {
	return agent_usecase.ToolTestContext{EntryID: c.EntryID, EntryType: c.EntryType, LeadID: c.LeadID, Vars: c.Vars}
}

func testResponse(res agent_usecase.ToolTestResult) TestResponse {
	return TestResponse{
		Exchanges: res.Exchanges, MappedOutput: res.MappedOutput, ModelResult: res.ModelResult, IsError: res.IsError,
		ErrorKind: string(res.ErrorKind), ErrorChain: res.ErrorChain, DurationMs: res.DurationMs, Issues: res.Issues, Preview: res.Preview,
	}
}
