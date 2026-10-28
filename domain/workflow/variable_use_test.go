package workflow

import (
	"encoding/json"
	"reflect"
	"testing"
)

func graphWithUses() Graph {
	return Graph{
		Nodes: []Node{
			{ID: "t", Type: NodeTypeTriggerMessageReceived, Config: map[string]interface{}{}},
			{ID: "m", Type: NodeTypeActionSendText, Config: map[string]interface{}{"message": "Oi {{ campvars.matricula }} {{campvars.turno}}"}},
			{ID: "h", Type: NodeTypeActionHTTPRequest, Config: map[string]interface{}{"url": "https://x.test/{{var.campvars.matricula}}"}},
		},
		Edges:     []Edge{{Source: "t", Target: "m"}, {Source: "m", Target: "h"}},
		Variables: []VariableUse{{Name: "turno", Default: "Manhã"}, {Name: "valor"}},
	}
}

func TestGraphVariablesSurviveCloneStripAndPins(t *testing.T) {
	g := graphWithUses()
	g.Nodes[1].Pinned = &NodePin{}
	if got := g.Clone().Variables; !reflect.DeepEqual(got, g.Variables) {
		t.Fatalf("clone = %v", got)
	}
	if got := StripDraftOnly(g).Variables; !reflect.DeepEqual(got, g.Variables) {
		t.Fatalf("strip = %v", got)
	}
	if got := WithholdPins(g, func(string) bool { return true }).Variables; !reflect.DeepEqual(got, g.Variables) {
		t.Fatalf("withhold = %v", got)
	}
	if got := RestoreWithheldPins(g, g).Variables; !reflect.DeepEqual(got, g.Variables) {
		t.Fatalf("restore = %v", got)
	}
}

func TestAGraphWithoutVariablesKeepsItsHash(t *testing.T) {
	g := graphWithUses()
	g.Variables = nil
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var plain struct {
		Nodes []Node `json:"nodes"`
		Edges []Edge `json:"edges"`
	}
	if err := json.Unmarshal(raw, &plain); err != nil {
		t.Fatal(err)
	}
	if string(raw) != mustJSON(t, plain) {
		t.Fatalf("an empty variables list changed the stored graph: %s", raw)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestVariableUseLookupAndNodeUses(t *testing.T) {
	g := graphWithUses()
	use, ok := g.VariableUse("turno")
	if !ok || use.Default != "Manhã" {
		t.Fatalf("use = %+v %v", use, ok)
	}
	if _, ok := g.VariableUse("matricula"); ok {
		t.Fatal("matricula is not declared by the flow")
	}
	want := map[string][]string{"matricula": {"m", "h"}, "turno": {"m"}}
	if got := g.CampaignVarNodes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("nodes = %v, want %v", got, want)
	}
}

func TestLintRefusesBrokenVariableUses(t *testing.T) {
	cases := []struct {
		name string
		uses []VariableUse
		want int
	}{
		{"fine", []VariableUse{{Name: "turno", Default: "Manhã"}, {Name: "valor"}}, 0},
		{"bad name", []VariableUse{{Name: "minha var"}}, 1},
		{"system key", []VariableUse{{Name: "lead_name"}}, 1},
		{"repeated", []VariableUse{{Name: "a"}, {Name: "a"}}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := graphWithUses()
			g.Variables = tc.uses
			got := 0
			for _, issue := range VariableUseIssues(&g) {
				if issue.Code != LintGraphVariableInvalid || issue.Severity != SeverityBlocking {
					t.Fatalf("issue = %+v", issue)
				}
				got++
			}
			if got != tc.want {
				t.Fatalf("issues = %d, want %d", got, tc.want)
			}
		})
	}
}
