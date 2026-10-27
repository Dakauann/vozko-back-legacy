package conversation_usecase

import (
	"context"
	"strings"
	"testing"

	"vozko/domain/agent"
)

func TestAnOfficialAgentThatLacksAVariableSendsNothing(t *testing.T) {
	h := newWhatsAppContinuationHarness()
	variables := &scriptedVariables{ok: false}
	h.uc.agentVariables = variables
	err := h.uc.ContinueAgentTurn(context.Background(), officialTurn())
	if reason, skipped := replySkipReason(err); !skipped || reason != replySkipVariablesMissing {
		t.Fatalf("err = %v, want the variables skip", err)
	}
	if len(h.client.sent) != 0 || len(h.ai.inputs) != 0 {
		t.Fatalf("sent %d messages and ran the model %d times, want none", len(h.client.sent), len(h.ai.inputs))
	}
	if len(variables.asked) != 1 || variables.asked[0] != "entry-1/agent-7" {
		t.Fatalf("asked = %v", variables.asked)
	}
}

func TestAnOfficialAgentReadsTheValuesOfTheSharedReadPath(t *testing.T) {
	h := newWhatsAppContinuationHarness()
	h.uc.agentVariables = &scriptedVariables{ok: true, values: map[string]string{"turma": "C"}}
	h.uc.agentRepo = agentsByIDWithPrompt("agent-7", "Você é a Bia da turma {{turma}}.")
	if err := h.uc.ContinueAgentTurn(context.Background(), officialTurn()); err != nil {
		t.Fatal(err)
	}
	if prompt := h.ai.inputs[0].SystemPrompt; !strings.Contains(prompt, "turma C.") {
		t.Fatalf("system prompt = %q", prompt)
	}
}

func agentsByIDWithPrompt(id, prompt string) agentsByID {
	return agentsByID{byID: map[string]*agent.Agent{id: {ID: id, WorkspaceID: "ws-1", Name: "Bia", MessagingPrompt: prompt}}}
}
