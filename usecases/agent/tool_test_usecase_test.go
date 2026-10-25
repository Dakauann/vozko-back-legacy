package agent_usecase

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/calltrace"
	"vozko/domain/testinput"
	"vozko/domain/tools"
	httpcall_usecase "vozko/usecases/httpcall"
	tools_usecase "vozko/usecases/tools"
)

func testerFixture(t *testing.T, method string) (ToolTester, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Set-Cookie", "sid=segredo")
		_, _ = w.Write([]byte(`{"data":{"status":"enviado","cpf":"52998224725"}}`))
	}))
	t.Cleanup(srv.Close)
	registry := tools_usecase.NewService(tools_usecase.NewHTTPRequestToolUseCase(httpcall_usecase.Egress{Client: srv.Client()}, nil))
	config := map[string]interface{}{
		"url": srv.URL + "/pedidos/{numero}", "method": method,
		"fields": []interface{}{map[string]interface{}{"name": "numero", "location": "path", "source": "model", "type": "string", "description": "Número", "required": true}},
		"output": map[string]interface{}{"fields": []interface{}{map[string]interface{}{"name": "status", "path": "data.status"}}},
	}
	store := &agentStore{byID: map[string]*agent.Agent{"ag-1": {ID: "ag-1", WorkspaceID: "ws-1", InternalTools: []agent.ToolBinding{{
		ID: "t1", Name: "http_request", Function: "consultar_pedido", Description: "Consulta.", Config: config,
		TestInputs: []testinput.TestInput{{ID: "in-1", Name: "Pedido 42", Input: map[string]interface{}{"numero": "42"}}},
	}}}}}
	return NewToolTester(store, registry), &hits
}

func TestTestingASavedToolRunsTheLivePathAndShowsEveryLayer(t *testing.T) {
	tester, hits := testerFixture(t, "GET")
	res, err := tester.Test(context.Background(), "ag-1", "t1", ToolTestRequest{TestInputID: "in-1"})
	if err != nil {
		t.Fatal(err)
	}
	if *hits != 1 || res.IsError || len(res.Exchanges) != 1 || res.Exchanges[0].Status != 200 {
		t.Fatalf("res %+v", res)
	}
	if res.ModelResult != `{"context_update":"a API respondeu 200","is_error":false,"result":{"status":"enviado"}}` || strings.Contains(res.ModelResult, "segredo") {
		t.Fatalf("model result %s", res.ModelResult)
	}
	if mapped, _ := res.MappedOutput.(map[string]interface{}); mapped["status"] != "enviado" {
		t.Fatalf("mapped %+v", res.MappedOutput)
	}
}

func TestAMethodThatChangesDataNeedsConfirmation(t *testing.T) {
	tester, hits := testerFixture(t, "POST")
	if _, err := tester.Test(context.Background(), "ag-1", "t1", ToolTestRequest{Arguments: map[string]interface{}{"numero": "1"}}); !errors.Is(err, ErrRealCallNotConfirmed) || *hits != 0 {
		t.Fatalf("err %v hits %d", err, *hits)
	}
	if _, err := tester.Test(context.Background(), "ag-1", "t1", ToolTestRequest{Arguments: map[string]interface{}{"numero": "1"}, ConfirmRealCall: true}); err != nil || *hits != 1 {
		t.Fatalf("err %v hits %d", err, *hits)
	}
}

func TestTheDraftTestMatchesTheSavedTestForTheSameConfig(t *testing.T) {
	tester, _ := testerFixture(t, "GET")
	saved, err := tester.Test(context.Background(), "ag-1", "t1", ToolTestRequest{Arguments: map[string]interface{}{"numero": "42"}})
	if err != nil {
		t.Fatal(err)
	}
	store := tester.(*toolTester).agents.(*agentStore)
	draft := store.byID["ag-1"].InternalTools[0]
	draft.ID = ""
	fromDraft, err := tester.TestDraft(context.Background(), "ag-1", draft, ToolTestRequest{Arguments: map[string]interface{}{"numero": "42"}})
	if err != nil || fromDraft.ModelResult != saved.ModelResult {
		t.Fatalf("draft %+v saved %+v err %v", fromDraft, saved, err)
	}
}

func TestBadArgumentsComeBackAsAnArgumentsError(t *testing.T) {
	tester, hits := testerFixture(t, "GET")
	res, err := tester.Test(context.Background(), "ag-1", "t1", ToolTestRequest{Arguments: map[string]interface{}{}})
	if err != nil || !res.IsError || res.ErrorKind != calltrace.ErrorKindArguments || *hits != 0 {
		t.Fatalf("res %+v err %v", res, err)
	}
}

func TestOversizedArgumentsAndUnknownInputsAreRefused(t *testing.T) {
	tester, _ := testerFixture(t, "GET")
	if _, err := tester.Test(context.Background(), "ag-1", "t1", ToolTestRequest{Arguments: map[string]interface{}{"numero": strings.Repeat("x", 70000)}}); !errors.Is(err, ErrTestInputTooLarge) {
		t.Fatalf("err %v", err)
	}
	if _, err := tester.Test(context.Background(), "ag-1", "t1", ToolTestRequest{TestInputID: "ghost"}); !errors.Is(err, ErrTestInputMissing) {
		t.Fatalf("err %v", err)
	}
}

func TestADraftWithAPlainCredentialIsNotCalled(t *testing.T) {
	tester, hits := testerFixture(t, "GET")
	store := tester.(*toolTester).agents.(*agentStore)
	draft := store.byID["ag-1"].InternalTools[0]
	config := map[string]interface{}{}
	for k, v := range draft.Config {
		config[k] = v
	}
	config["auth"] = map[string]interface{}{"mode": "bearer", "token": "plain-token"}
	draft.Config = config
	res, err := tester.TestDraft(context.Background(), "ag-1", draft, ToolTestRequest{Arguments: map[string]interface{}{"numero": "1"}})
	if err != nil || *hits != 0 || len(res.Issues) == 0 || res.Issues[0].Code != "tool_secret_in_plain_text" {
		t.Fatalf("res %+v err %v hits %d", res, err, *hits)
	}
}

func TestPutAndDeleteNeedConfirmationToo(t *testing.T) {
	for _, method := range []string{"PUT", "DELETE", "PATCH"} {
		tester, hits := testerFixture(t, method)
		if _, err := tester.Test(context.Background(), "ag-1", "t1", ToolTestRequest{Arguments: map[string]interface{}{"numero": "1"}}); !errors.Is(err, ErrRealCallNotConfirmed) || *hits != 0 {
			t.Fatalf("%s err %v hits %d", method, err, *hits)
		}
	}
}

type handoffTool struct{ calls int }

func (h *handoffTool) Definition() tools.Definition {
	return tools.Definition{Name: "transfer_to_human", Parameters: map[string]tools.Parameter{"reason": {Type: "string"}}}
}

func (h *handoffTool) Execute(context.Context, map[string]interface{}) (tools.ExecutionResult, error) {
	h.calls++
	return tools.ExecutionResult{}, nil
}

func (h *handoffTool) ExecuteWithConfig(context.Context, map[string]interface{}, map[string]interface{}) (tools.ExecutionResult, error) {
	h.calls++
	return tools.ExecutionResult{}, nil
}

type previewingHandoffTool struct{ handoffTool }

func (h *previewingHandoffTool) Preview(context.Context, map[string]interface{}, map[string]interface{}) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{Result: "previewed"}, nil
}

func handoffTester(handler tools.Handler) ToolTester {
	store := &agentStore{byID: map[string]*agent.Agent{"ag-1": {ID: "ag-1", WorkspaceID: "ws-1", InternalTools: []agent.ToolBinding{{
		ID: "t1", Name: "transfer_to_human", Function: "transfer_to_human", Description: "Passa para uma pessoa.",
	}}}}}
	return NewToolTester(store, tools_usecase.NewService(handler))
}

func TestAToolTypeTestedInAScenarioIsRefusedWithoutRunning(t *testing.T) {
	handler := &handoffTool{}
	_, err := handoffTester(handler).Test(context.Background(), "ag-1", "t1", ToolTestRequest{Arguments: map[string]interface{}{"reason": "cliente pediu"}, ConfirmRealCall: true})
	if !errors.Is(err, ErrToolTestUnsupported) || handler.calls != 0 {
		t.Fatalf("err %v calls %d", err, handler.calls)
	}
}

func TestAToolTypeWithAPreviewIsPreviewedNotExecuted(t *testing.T) {
	handler := &previewingHandoffTool{}
	res, err := handoffTester(handler).Test(context.Background(), "ag-1", "t1", ToolTestRequest{Arguments: map[string]interface{}{"reason": "cliente pediu"}})
	if err != nil || !res.Preview || handler.calls != 0 {
		t.Fatalf("res %+v err %v calls %d", res, err, handler.calls)
	}
}
