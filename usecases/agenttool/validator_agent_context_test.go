package agenttool_usecase

import (
	"context"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/tools"
	"vozko/usecases/agentctx"
)

type agentSeeingHandler struct {
	fakeHandler
	seen *[]string
}

func (h agentSeeingHandler) ValidateBinding(ctx context.Context, _ string, _ map[string]interface{}) error {
	if current, ok := agentctx.AgentFromContext(ctx); ok && current != nil {
		*h.seen = append(*h.seen, current.ID)
	}
	return nil
}

func TestABindingCheckKnowsWhichAgentOwnsTheTool(t *testing.T) {
	var seen []string
	deps := validatorDeps()
	def := tools.Definition{Name: "transfer_to_automation", Description: "Transfere", ConfigSchema: map[string]tools.ConfigParameter{"destinations": {Type: "array"}}}
	deps.Registry = fakeRegistry{handlers: map[string]tools.Handler{"transfer_to_automation": agentSeeingHandler{fakeHandler{def: def}, &seen}}}

	NewValidator(deps).Validate(context.Background(), agentWith(agent.ToolBinding{ID: "t9", Name: "transfer_to_automation", Function: "transfer_to_automation", Config: map[string]interface{}{"destinations": []interface{}{}}}), nil)

	if len(seen) != 1 || seen[0] != "ag-1" {
		t.Fatalf("seen = %v", seen)
	}
}
