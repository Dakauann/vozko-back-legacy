package workflow

import (
	"reflect"
	"strings"
	"testing"
)

func TestAWriteNeverResolvesToAPlaceholder(t *testing.T) {
	state := NewRunState()
	state.Set("message", "Marcos Infante")
	state.Set("nome", "{{message}}")
	state.Set("_last_sobrenome", "Gimenez")

	cases := []struct {
		name       string
		text       string
		resolved   string
		unresolved []string
	}{
		{"a resolved reply", "{{message}}", "Marcos Infante", nil},
		{"several resolved refs", "{{message}} {{last.sobrenome}}", "Marcos Infante Gimenez", nil},
		{"plain text", "Ana", "Ana", nil},
		{"a ref the run never had", "{{trigger.nome}}", "{{trigger.nome}}", []string{"trigger.nome"}},
		{"a variable that holds a placeholder", "{{var.nome}}", "{{message}}", []string{"message"}},
		{"repeated and unordered refs", "{{b}} {{a}} {{b}}", "{{b}} {{a}} {{b}}", []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved, unresolved := InterpolateForWrite(tc.text, &state)
			if resolved != tc.resolved || !reflect.DeepEqual(unresolved, tc.unresolved) {
				t.Fatalf("InterpolateForWrite(%q) = %q %v, want %q %v", tc.text, resolved, unresolved, tc.resolved, tc.unresolved)
			}
		})
	}
}

func TestTheUnresolvedWriteReasonNamesEveryVariable(t *testing.T) {
	reason := UnresolvedWriteReason([]string{"message", "last.nome", "message"})

	if !strings.Contains(reason, "last.nome, message") || !strings.Contains(reason, "nada foi gravado") {
		t.Fatalf("reason %q, want the sorted variables and that nothing was written", reason)
	}
}
