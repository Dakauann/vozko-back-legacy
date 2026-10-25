package agenttoolhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response/responsetest"
	"vozko/domain/agent"
	domainmcp "vozko/domain/agent/mcp"
	"vozko/domain/agenttool"
	"vozko/domain/auth"
	"vozko/domain/testinput"
	"vozko/domain/tools"
	"vozko/domain/workspace"
	"vozko/infra/http/middleware"
	agent_usecase "vozko/usecases/agent"
	tools_usecase "vozko/usecases/tools"
)

type fakeTools struct {
	agent.AgentToolsUseCase
	list    []agent.ToolBinding
	draft   agent.ToolDraft
	edit    agent.ToolEdit
	by      workspace.Requester
	err     error
	deleted string
}

func (f *fakeTools) List(context.Context, string) ([]agent.ToolBinding, error) { return f.list, f.err }

func (f *fakeTools) Create(_ context.Context, _ string, d agent.ToolDraft, by workspace.Requester) (agent.ToolBinding, error) {
	f.draft, f.by = d, by
	b := agent.ToolBinding{ID: "t-new", Name: d.Type, Function: "consultar_pedido", Description: d.Description, Config: d.Config, Revision: 1}
	f.list = append(f.list, b)
	return b, f.err
}

func (f *fakeTools) Update(_ context.Context, _, id string, e agent.ToolEdit, _ workspace.Requester) (agent.ToolBinding, error) {
	f.edit = e
	return agent.ToolBinding{ID: id, Name: "http_request", Function: e.Name, Revision: e.Revision + 1}, f.err
}

func (f *fakeTools) Delete(_ context.Context, _, id string, _ workspace.Requester) error {
	f.deleted = id
	return f.err
}

type agentOf struct{ a *agent.Agent }

func (g agentOf) Execute(string) (*agent.Agent, error) { return g.a, nil }

type httpCatalog struct{ tools.Service }

func (httpCatalog) Definitions() []tools.Definition {
	return []tools.Definition{{Name: "http_request", ConfigSchema: map[string]tools.ConfigParameter{"headers": {Type: "object", Sensitive: true}}}}
}

func (httpCatalog) Handler(string) (tools.Handler, bool) { return nil, false }

func request(method, path string, body interface{}, vars map[string]string) *http.Request {
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	ctx := context.WithValue(r.Context(), middleware.ClaimsContextKey, &auth.Claims{UserID: "u-1", Role: "user"})
	ctx = context.WithValue(ctx, middleware.WorkspaceIDContextKey, "ws-1")
	return mux.SetURLVars(r.WithContext(ctx), vars)
}

type noMCP struct{}

func (noMCP) Sources(context.Context, domainmcp.WorkspaceID) ([]domainmcp.ToolSource, error) {
	return nil, nil
}

func handlerWith(f *fakeTools, owner *agent.Agent) *Handler {
	h, err := NewHandler(Deps{Tools: f, Agents: agentOf{a: owner}, Registry: httpCatalog{}, Validation: f, Tester: fakeTester{}, MCPSources: noMCP{}})
	if err != nil {
		panic(err)
	}
	return h
}

func TestTheToolHandlerNamesEveryMissingDependency(t *testing.T) {
	_, err := NewHandler(Deps{})
	if err == nil || !strings.Contains(err.Error(), "MCP source catalog") || !strings.Contains(err.Error(), "tool tester") {
		t.Fatalf("err %v", err)
	}
}

func TestMCPSourcesAreListedForTheWorkspace(t *testing.T) {
	rec := httptest.NewRecorder()
	handlerWith(&fakeTools{}, ownAgent()).MCPSources(rec, request(http.MethodGet, "/agents/tools/mcp-sources", nil, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sources":[]`) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func ownAgent() *agent.Agent { return &agent.Agent{ID: "ag-1", WorkspaceID: "ws-1"} }

func TestListingToolsNeverReturnsAPlainCredential(t *testing.T) {
	f := &fakeTools{list: []agent.ToolBinding{{ID: "t1", Name: "http_request", Function: "pedido", Revision: 2, Config: map[string]interface{}{
		"url": "https://a.test?api_key=plain-key", "headers": map[string]interface{}{"Authorization": "Bearer plain", "X-Ref": "{{secret.k}}"},
	}}}}
	rec := httptest.NewRecorder()
	handlerWith(f, ownAgent()).List(rec, request(http.MethodGet, "/agents/ag-1/tools", nil, map[string]string{"id": "ag-1"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "plain") || !strings.Contains(body, `"name":"pedido"`) || !strings.Contains(body, `"revision":2`) {
		t.Fatalf("body %s", body)
	}
}

func TestAnotherWorkspacesAgentIsForbidden(t *testing.T) {
	rec := httptest.NewRecorder()
	handlerWith(&fakeTools{}, &agent.Agent{ID: "ag-1", WorkspaceID: "ws-2"}).List(rec, request(http.MethodGet, "/agents/ag-1/tools", nil, map[string]string{"id": "ag-1"}))
	if rec.Code != http.StatusForbidden || responsetest.CodeOf(t, rec) != "agent_forbidden" {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestCreatingAToolPassesTheDraftAndTheCaller(t *testing.T) {
	f := &fakeTools{}
	rec := httptest.NewRecorder()
	handlerWith(f, ownAgent()).Create(rec, request(http.MethodPost, "/agents/ag-1/tools", map[string]interface{}{
		"type": "http_request", "description": "Consulta pedidos.", "config": map[string]interface{}{"url": "https://a.test"},
	}, map[string]string{"id": "ag-1"}))
	if rec.Code != http.StatusCreated || f.draft.Type != "http_request" || f.draft.Description != "Consulta pedidos." || f.by.UserID != "u-1" {
		t.Fatalf("status %d draft %+v by %+v body %s", rec.Code, f.draft, f.by, rec.Body.String())
	}
}

func TestUpdatingAToolSendsTheRevisionAndAnswersAConflictWithItsCode(t *testing.T) {
	f := &fakeTools{err: agenttool.ErrRevisionConflict}
	rec := httptest.NewRecorder()
	handlerWith(f, ownAgent()).Update(rec, request(http.MethodPut, "/agents/ag-1/tools/t1", map[string]interface{}{"name": "pedido", "revision": 3}, map[string]string{"id": "ag-1", "toolId": "t1"}))
	if rec.Code != http.StatusConflict || responsetest.CodeOf(t, rec) != "agent_tool_revision_conflict" || f.edit.Revision != 3 {
		t.Fatalf("status %d edit %+v body %s", rec.Code, f.edit, rec.Body.String())
	}
}

func TestAMalformedBodyIsRefusedWithACode(t *testing.T) {
	rec := httptest.NewRecorder()
	r := request(http.MethodPost, "/agents/ag-1/tools", nil, map[string]string{"id": "ag-1"})
	r.Body = http.NoBody
	handlerWith(&fakeTools{}, ownAgent()).Create(rec, r)
	if rec.Code != http.StatusBadRequest || responsetest.CodeOf(t, rec) != "agent_invalid_body" {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestDeletingAToolAnswersNoContent(t *testing.T) {
	f := &fakeTools{}
	rec := httptest.NewRecorder()
	handlerWith(f, ownAgent()).Delete(rec, request(http.MethodDelete, "/agents/ag-1/tools/t1", nil, map[string]string{"id": "ag-1", "toolId": "t1"}))
	if rec.Code != http.StatusNoContent || f.deleted != "t1" {
		t.Fatalf("status %d deleted %q", rec.Code, f.deleted)
	}
}

func TestEveryToolRouteIsGatedByAgentPermissions(t *testing.T) {
	got := map[string]workspace.Action{}
	ac := func(res workspace.Resource, act workspace.Action, _ http.HandlerFunc) http.HandlerFunc {
		if res != workspace.ResourceAgents {
			t.Fatalf("resource %v", res)
		}
		return func(w http.ResponseWriter, r *http.Request) {
			got[r.Method+" "+mux.CurrentRoute(r).GetName()+r.URL.Path] = act
		}
	}
	router := mux.NewRouter()
	RegisterRoutes(router, handlerWith(&fakeTools{}, ownAgent()), ac)
	for path, method := range map[string]string{
		"/agents/a/tools":               http.MethodGet,
		"/agents/a/tools/order":         http.MethodPut,
		"/agents/a/tools/t/duplicate":   http.MethodPost,
		"/agents/a/tools/t/revisions/2": http.MethodGet,
	} {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, path, nil))
		if _, ok := got[method+" "+path]; !ok {
			t.Errorf("%s %s is not routed through the permission check", method, path)
		}
	}
	if got["GET /agents/a/tools"] != workspace.ActionReadDetails || got["PUT /agents/a/tools/order"] != workspace.ActionUpdate {
		t.Fatalf("actions %v", got)
	}
}

func (f *fakeTools) Validate(_ context.Context, _ string, draft []agent.ToolBinding) (agenttool.Report, error) {
	if draft != nil && len(draft) > 0 && draft[0].Config["url"] == "http://a.test" {
		return agenttool.Report{Issues: []agenttool.Issue{{Code: agenttool.CodeHTTPURLNotHTTPS, Severity: "blocking", Message: "x"}}}, nil
	}
	return agenttool.Report{}, f.err
}

func TestTheListCarriesTheValidatorVerdict(t *testing.T) {
	rec := httptest.NewRecorder()
	handlerWith(&fakeTools{}, ownAgent()).List(rec, request(http.MethodGet, "/agents/ag-1/tools", nil, map[string]string{"id": "ag-1"}))
	if !strings.Contains(rec.Body.String(), `"green":true`) || !strings.Contains(rec.Body.String(), `"issues":[]`) {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestValidatingADraftReturnsItsIssues(t *testing.T) {
	rec := httptest.NewRecorder()
	handlerWith(&fakeTools{}, ownAgent()).Validate(rec, request(http.MethodPost, "/agents/ag-1/tools/validate", map[string]interface{}{
		"tools": []map[string]interface{}{{"type": "http_request", "config": map[string]interface{}{"url": "http://a.test"}}},
	}, map[string]string{"id": "ag-1"}))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"green":false`) || !strings.Contains(rec.Body.String(), "http_url_not_https") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

type fakeTester struct{}

func (fakeTester) Test(_ context.Context, _, _ string, req agent_usecase.ToolTestRequest) (agent_usecase.ToolTestResult, error) {
	if !req.ConfirmRealCall {
		return agent_usecase.ToolTestResult{}, agent_usecase.ErrRealCallNotConfirmed
	}
	return agent_usecase.ToolTestResult{ModelResult: `{"is_error":false}`}, nil
}

func (fakeTester) TestDraft(context.Context, string, agent.ToolBinding, agent_usecase.ToolTestRequest) (agent_usecase.ToolTestResult, error) {
	return agent_usecase.ToolTestResult{ModelResult: `{"draft":true}`}, nil
}

func TestATestWithoutConfirmationAnswersItsCode(t *testing.T) {
	rec := httptest.NewRecorder()
	handlerWith(&fakeTools{}, ownAgent()).Test(rec, request(http.MethodPost, "/agents/ag-1/tools/t1/test", map[string]interface{}{"arguments": map[string]interface{}{}}, map[string]string{"id": "ag-1", "toolId": "t1"}))
	if rec.Code != http.StatusUnprocessableEntity || responsetest.CodeOf(t, rec) != "agent_tool_test_real_call_not_confirmed" {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	handlerWith(&fakeTools{}, ownAgent()).Test(rec, request(http.MethodPost, "/agents/ag-1/tools/t1/test", map[string]interface{}{"confirmRealCall": true}, map[string]string{"id": "ag-1", "toolId": "t1"}))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "modelResult") {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestListingToolsServesTheModelSchemaAndTheSavedTestInputs(t *testing.T) {
	f := &fakeTools{list: []agent.ToolBinding{{ID: "t1", Name: "http_request", Function: "pedido", Description: "Consulta o pedido.", Revision: 1,
		TestInputs: []testinput.TestInput{{ID: "ex-1", Name: "Pedido 42", Input: map[string]interface{}{"numero": "42"}}},
	}}}
	rec := httptest.NewRecorder()
	handlerWith(f, ownAgent()).List(rec, request(http.MethodGet, "/agents/ag-1/tools", nil, map[string]string{"id": "ag-1"}))
	var out struct {
		Tools []struct {
			InputSchema map[string]interface{} `json:"inputSchema"`
			TestInputs  []testinput.TestInput  `json:"testInputs"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Tools) != 1 {
		t.Fatalf("body %s", rec.Body.String())
	}
	if out.Tools[0].InputSchema["type"] != "object" || len(out.Tools[0].TestInputs) != 1 || out.Tools[0].TestInputs[0].Name != "Pedido 42" {
		t.Fatalf("tool %+v", out.Tools[0])
	}
}

type stubHandler struct{ name string }

func (s stubHandler) Definition() tools.Definition { return tools.Definition{Name: s.name} }

func (s stubHandler) Execute(context.Context, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{}, nil
}

func (s stubHandler) ExecuteWithConfig(context.Context, map[string]interface{}, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{}, nil
}

type previewStubHandler struct{ stubHandler }

func (previewStubHandler) Preview(context.Context, map[string]interface{}, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{}, nil
}

func TestListingToolsTellsHowEachOneIsTested(t *testing.T) {
	f := &fakeTools{list: []agent.ToolBinding{
		{ID: "t1", Name: "http_request", Function: "pedido"},
		{ID: "t2", Name: "add_sheet_row", Function: "planilha"},
		{ID: "t3", Name: "transfer_to_human", Function: "transferir"},
	}}
	registry := tools_usecase.NewService(stubHandler{"http_request"}, previewStubHandler{stubHandler{"add_sheet_row"}}, stubHandler{"transfer_to_human"})
	h, err := NewHandler(Deps{Tools: f, Agents: agentOf{a: ownAgent()}, Registry: registry, Validation: f, Tester: fakeTester{}, MCPSources: noMCP{}})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.List(rec, request(http.MethodGet, "/agents/ag-1/tools", nil, map[string]string{"id": "ag-1"}))
	var out struct {
		Tools []struct {
			ID       string `json:"id"`
			TestMode string `json:"testMode"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Tools) != 3 {
		t.Fatalf("body %s", rec.Body.String())
	}
	got := map[string]string{}
	for _, tool := range out.Tools {
		got[tool.ID] = tool.TestMode
	}
	if got["t1"] != "run" || got["t2"] != "preview" || got["t3"] != "scenario" {
		t.Fatalf("test modes %v", got)
	}
}
