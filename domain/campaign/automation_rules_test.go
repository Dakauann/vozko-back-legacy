package campaign

import "testing"

func TestAutomationConfigured(t *testing.T) {
	cases := []struct {
		a    Automation
		want bool
	}{
		{a: Automation{}, want: false},
		{a: Automation{WorkflowID: "  "}, want: false},
		{a: Automation{WorkflowID: "wf-1"}, want: true},
		{a: Automation{AgentID: "agent-1"}, want: true},
	}
	for _, tc := range cases {
		if got := tc.a.Configured(); got != tc.want {
			t.Fatalf("Configured(%+v) = %v, want %v", tc.a, got, tc.want)
		}
	}
}

func TestEntryMetadataOfNamesTheLeadOrTheNumber(t *testing.T) {
	if got := EntryMetadataOf("lead-1", "5584", nil); got.Label != "lead lead-1" {
		t.Fatalf("label = %q", got.Label)
	}
	if got := EntryMetadataOf("", "5584", map[string]any{"escola": "A"}); got.Label != "number 5584" || got.Metadata["escola"] != "A" {
		t.Fatalf("entry = %+v", got)
	}
}

func TestEntryValuesNumberTheTemplateVariablesBesideTheMetadata(t *testing.T) {
	got := EntryMetadataOf("lead-1", "", map[string]any{"turma": "A"}).WithVariables([]string{"Ana"}).Values()
	if got["turma"] != "A" || got["1"] != "Ana" {
		t.Fatalf("values = %v", got)
	}
	if got := EntryMetadataOf("", "5584", nil).Values(); got != nil {
		t.Fatalf("values = %v", got)
	}
}
