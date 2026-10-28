package automationneeds

import (
	"reflect"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

func flowWithDefaults() fakeSource {
	g := nodes(textNode("m", "{{campvars.turno}} {{campvars.matricula}} {{campvars.cidade}}"), aiNode("ai", "a1"))
	g.Variables = []workflow.VariableUse{{Name: "turno", Default: "Manhã"}, {Name: "matricula"}, {Name: "cidade", Default: "Recife"}, {Name: "sobra", Default: "x"}}
	return fakeSource{
		agents: map[string]*agent.Agent{
			"a1": {ID: "a1", WorkspaceID: "ws", Name: "Agente", MessagingPrompt: "{{cidade}}"},
		},
		workflows: map[string]*workflow.Workflow{"w1": {ID: "w1", WorkspaceID: "ws", Name: "Fluxo", Graph: g}},
	}
}

func TestAFlowDefaultMakesItsVariableOptionalAndRequiredWinsAcrossTheWalk(t *testing.T) {
	vars, err := VariablesOf(flowWithDefaults(), "ws", workflowRef("w1"))
	if err != nil {
		t.Fatal(err)
	}
	if got := vars.Required.Names(); !reflect.DeepEqual(got, []string{"cidade", "matricula"}) {
		t.Fatalf("required = %v", got)
	}
	if len(vars.Optional) != 1 || vars.Optional[0].Name != "turno" || vars.Optional[0].Default != "Manhã" {
		t.Fatalf("optional = %+v", vars.Optional)
	}
	if owner := vars.Optional[0].Owner(); owner.ID != "w1" {
		t.Fatalf("owner = %+v", owner)
	}
}

func TestFillGivesBlankValuesTheirDefaults(t *testing.T) {
	vars := Variables{Optional: Needs{{Name: "turno", Default: "Manhã"}, {Name: "tom", Default: "cordial"}}}
	got := vars.Fill(map[string]string{"turno": " ", "tom": "sério", "x": "1"})
	want := map[string]string{"turno": "Manhã", "tom": "sério", "x": "1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filled = %v, want %v", got, want)
	}
	if filled := vars.Fill(nil); filled["turno"] != "Manhã" {
		t.Fatalf("nil values = %v", filled)
	}
}

func TestLabeledNeedsUseTheDeclarationsAndTheSystemLabels(t *testing.T) {
	catalog := variabledecl.NewCatalog([]variabledecl.Declaration{{Name: "matricula", Label: "Matrícula", Type: variabledecl.Type{Kind: variabledecl.KindNumber}}})
	got := Needs{{Name: "matricula"}, {Name: "livre"}}.Labeled(catalog)
	if got[0].Label != "Matrícula" || !got[0].Declared || got[0].Type.Kind != variabledecl.KindNumber {
		t.Fatalf("declared = %+v", got[0])
	}
	if got[1].Label != "Livre" || got[1].Declared || got[1].Type.Kind != variabledecl.KindText {
		t.Fatalf("undeclared = %+v", got[1])
	}
}
