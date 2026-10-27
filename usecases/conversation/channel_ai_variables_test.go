package conversation_usecase

import (
	"vozko/domain/agent"
	"context"
	"strings"
	"testing"

	"vozko/domain/channel"
	"vozko/domain/shared"
)

type textRulesAgentsPrompt struct {
	agent.Repository
	prompt string
}

func (p textRulesAgentsPrompt) FindByID(id string) (*agent.Agent, error) {
	return &agent.Agent{ID: id, WorkspaceID: "ws-1", Name: "Bia", MessagingPrompt: p.prompt, MessagingModel: "gpt-x"}, nil
}

type scriptedVariables struct {
	values map[string]string
	ok     bool
	asked  []string
}

func (s *scriptedVariables) AgentValues(_ context.Context, _, entryID string, _ shared.EntryType, agentID string) (map[string]string, bool) {
	s.asked = append(s.asked, entryID+"/"+agentID)
	return s.values, s.ok
}

func TestAChannelAgentThatLacksAVariableDoesNotAnswer(t *testing.T) {
	h := newTextRulesHarness(t, channel.Capabilities{}, "nunca enviado")
	variables := &scriptedVariables{ok: false}
	h.svc.variables = variables
	if _, err := h.svc.Reply(context.Background(), textRulesRequest()); err != nil {
		t.Fatalf("a refused turn is a skipped reply, not a failure: %v", err)
	}
	if len(h.ai.inputs) != 0 || len(h.sender.sent) != 0 {
		t.Fatalf("the model ran %d times and %d texts went out, want none", len(h.ai.inputs), len(h.sender.sent))
	}
	if len(variables.asked) != 1 || variables.asked[0] != "conv-1/agent-1" {
		t.Fatalf("asked = %v", variables.asked)
	}
}

func TestAChannelAgentReadsTheConversationValues(t *testing.T) {
	h := newTextRulesHarness(t, channel.Capabilities{}, "ok")
	h.svc.variables = &scriptedVariables{ok: true, values: map[string]string{"turma": "B"}}
	h.svc.agents = textRulesAgentsPrompt{prompt: "Atenda a turma {{turma}}."}
	if _, err := h.svc.Reply(context.Background(), textRulesRequest()); err != nil {
		t.Fatal(err)
	}
	if prompt := h.ai.inputs[0].SystemPrompt; !strings.Contains(prompt, "Atenda a turma B.") {
		t.Fatalf("system prompt = %q, want the stored value", prompt)
	}
}
