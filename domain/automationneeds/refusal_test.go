package automationneeds

import (
	"errors"
	"fmt"
	"testing"
)

func TestRefusalsCarryTheirCauseAndCode(t *testing.T) {
	cases := []struct {
		err  error
		code string
	}{
		{&Refusal{Cause: ErrReceptiveUnmet}, "automation_variables_unmet"},
		{fmt.Errorf("wrapped: %w", &Refusal{Cause: ErrConversationMissing}), "automation_variables_missing"},
		{&Refusal{Cause: ErrPublishSwitchesOff}, "automation_publish_switches_off"},
		{&Refusal{Cause: ErrVoiceVariables}, "automation_voice_variables"},
		{&Refusal{Cause: ErrTestValuesMissing}, "automation_test_values_missing"},
		{fmt.Errorf("%w: agent x", ErrUnreadable), "automation_needs_unreadable"},
		{fmt.Errorf("%w: agent x", ErrNotFound), "automation_needs_not_found"},
		{errors.New("other"), ""},
	}
	for _, tc := range cases {
		if got := ErrorCode(tc.err); got != tc.code {
			t.Fatalf("ErrorCode(%v) = %q, want %q", tc.err, got, tc.code)
		}
	}
}

func TestRefusalOfFindsTheDetailsBehindAWrappedError(t *testing.T) {
	inner := &Refusal{Cause: ErrReceptiveUnmet, Automation: Link{Name: "Cobrança"}, Needs: Needs{{Name: "valor"}}}
	got, ok := RefusalOf(fmt.Errorf("save: %w", inner))
	if !ok || got.Automation.Name != "Cobrança" || got.Needs[0].Name != "valor" {
		t.Fatalf("refusal = %+v ok=%v", got, ok)
	}
	if !errors.Is(inner, ErrReceptiveUnmet) {
		t.Fatal("a refusal must unwrap to its cause")
	}
	if inner.Error() == "" {
		t.Fatal("a refusal explains itself")
	}
	if _, ok := RefusalOf(errors.New("plain")); ok {
		t.Fatal("a plain error has no refusal")
	}
}

func TestEveryCodeIsListedOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, code := range ErrorCodes() {
		if seen[code] {
			t.Fatalf("duplicate %s", code)
		}
		seen[code] = true
	}
	if len(seen) != 7 {
		t.Fatalf("codes = %v", ErrorCodes())
	}
}

func TestTheSentenceNamesTheAutomationAndTheVariables(t *testing.T) {
	r := &Refusal{Cause: ErrReceptiveUnmet, Automation: Link{Automation: agentRef("a1"), Name: "Escola"}, Needs: Needs{{Name: "turma"}}}
	if got := r.Sentence(); got != "O agente Escola precisa das variáveis turma, que conversas receptivas não trazem. Dê um valor padrão a elas ou escolha outra automação." {
		t.Fatalf("sentence = %q", got)
	}
}

func TestTheTestRefusalAsksForTheTestData(t *testing.T) {
	r := &Refusal{Cause: ErrTestValuesMissing, Automation: Link{Automation: workflowRef("w1"), Name: "Cobrança"}, Needs: Needs{{Name: "matricula"}, {Name: "valor_pendencia"}}}
	if got := r.Sentence(); got != "O fluxo Cobrança precisa das variáveis matricula, valor_pendencia. Preencha os dados do teste antes de iniciar." {
		t.Fatalf("sentence = %q", got)
	}
}
