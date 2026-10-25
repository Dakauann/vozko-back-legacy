package openrouter

import (
	"context"
	"testing"

	"vozko/domain/ai"
	"vozko/domain/tools"
)

const instanceCallReply = `{"id":"gen","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"%s","type":"function","function":{"name":"consultar_pedido","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11}}`

type dispatchRecorder struct {
	injectFakeToolService
	handler  string
	config   map[string]interface{}
	instance tools.InstanceRef
}

func (r *dispatchRecorder) ExecuteWithConfig(ctx context.Context, name string, config map[string]interface{}, _ map[string]interface{}) (tools.ExecutionResult, error) {
	r.handler, r.config = name, config
	r.instance, _ = tools.InstanceFrom(ctx)
	return tools.ExecutionResult{Result: "ok"}, nil
}

func TestAnInstanceCallRunsItsHandlerWithItsConfigAndIdentity(t *testing.T) {
	srv, _, _ := scriptedServer(t, []string{instanceCallReply, textReply})
	recorder := &dispatchRecorder{}
	service := loopService(srv.URL)
	service.toolService = recorder
	_, err := service.Generate(context.Background(), ai.GenerateInput{
		Model:             "openai/gpt-4o",
		WorkspaceID:       "ws",
		Messages:          []ai.Message{{Role: ai.RoleUser, Content: "cadê meu pedido?"}},
		Tools:             []tools.Definition{{Name: "consultar_pedido", HandlerName: "http_request", Description: "pedido", Instance: &tools.InstanceRef{ID: "t1", Type: "http_request", Revision: 4}}},
		ToolExecutionMode: ai.ToolExecutionModeAuto,
		ToolConfigs:       map[string]map[string]interface{}{"consultar_pedido": {"url": "https://a.test/orders"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if recorder.handler != "http_request" || recorder.config["url"] != "https://a.test/orders" || recorder.instance.ID != "t1" || recorder.instance.Revision != 4 {
		t.Fatalf("handler %q config %v instance %+v", recorder.handler, recorder.config, recorder.instance)
	}
}
