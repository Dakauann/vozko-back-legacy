package openrouter

import (
	"context"
	"testing"

	"vozko/domain/ai"
	"vozko/domain/tools"
)

const usageToolCallReply = `{"id":"gen","model":"openai/gpt-4o-2024","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"%s","type":"function","function":{"name":"consultar_pedido","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":120,"completion_tokens":9,"total_tokens":129,"cost":0.5,"prompt_tokens_details":{"cached_tokens":100},"completion_tokens_details":{"reasoning_tokens":4}}}`

func TestEveryLoopIterationIsReportedAsAModelCallAndToolCallsPointAtTheirs(t *testing.T) {
	srv, _, _ := scriptedServer(t, []string{usageToolCallReply, textReply})
	service := loopService(srv.URL)
	service.toolService = &dispatchRecorder{}
	out, err := service.Generate(context.Background(), ai.GenerateInput{
		Model:             "openai/gpt-4o",
		WorkspaceID:       "ws",
		Messages:          []ai.Message{{Role: ai.RoleUser, Content: "cadê meu pedido?"}},
		Tools:             []tools.Definition{{Name: "consultar_pedido", HandlerName: "http_request", Description: "pedido"}},
		ToolExecutionMode: ai.ToolExecutionModeAuto,
		ToolConfigs:       map[string]map[string]interface{}{"consultar_pedido": {"url": "https://a.test/orders"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.ModelCalls) != 2 {
		t.Fatalf("model calls %+v", out.ModelCalls)
	}
	first, second := out.ModelCalls[0], out.ModelCalls[1]
	if first.Index != 0 || first.Model != "openai/gpt-4o-2024" || first.InputTokens != 120 || first.OutputTokens != 9 || first.CachedTokens != 100 || first.ReasoningTokens != 4 || first.CostMicros != 500000 || first.FinishReason != "tool_calls" || first.LatencyMs < 0 {
		t.Fatalf("first %+v", first)
	}
	if second.Index != 1 || second.Model != "openai/gpt-4o" || second.InputTokens != 10 || second.FinishReason != "stop" || second.CostMicros != 0 {
		t.Fatalf("second %+v", second)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].ModelCallIndex != 0 {
		t.Fatalf("tool calls %+v", out.ToolCalls)
	}
}
