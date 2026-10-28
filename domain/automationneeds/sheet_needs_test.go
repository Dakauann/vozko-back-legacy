package automationneeds

import (
	"reflect"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/sheetfit"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

func sheetNode(id string, kind workflow.NodeType, tab, variable string, values map[string]interface{}) workflow.Node {
	return workflow.Node{ID: id, Type: kind, Config: map[string]interface{}{
		workflow.SheetStepSheetKey: tab, workflow.SheetStepVariableKey: variable, workflow.SheetStepValuesKey: values,
	}}
}

func sheetSource() fakeSource {
	return fakeSource{
		agents: map[string]*agent.Agent{
			"cobrador": {ID: "cobrador", WorkspaceID: "ws", Name: "Cobrador", InternalTools: []agent.ToolBinding{
				{Name: workflow.AgentSheetRowTool, Config: map[string]interface{}{workflow.SheetStepSheetKey: "model-pag", workflow.SheetStepVariableKey: "{{campvars.planilha}}"}},
				{Name: workflow.AgentSheetRowTool, Config: map[string]interface{}{workflow.SheetStepSheetKey: "fixa"}},
			}},
		},
		workflows: map[string]*workflow.Workflow{
			"franquias": {ID: "franquias", WorkspaceID: "ws", Name: "Franquias", Graph: nodes(
				sheetNode("exp", workflow.NodeTypeActionExportMessagesSheet, "model-neg", "{{campvars.planilha}}", nil),
				sheetNode("row", workflow.NodeTypeActionAddSheetRow, "model-pag", "{{campvars.planilha}}", map[string]interface{}{"valor": "{{var.valor}}", "parcelas": 3.0}),
				sheetNode("fixed", workflow.NodeTypeActionAddSheetRow, "fixa", "", map[string]interface{}{"x": "y"}),
				aiNode("ai", "cobrador"),
			)},
		},
	}
}

func TestAStepPerConversationNeedsItsPlanilhaWithEveryUseOfTheWalk(t *testing.T) {
	vars, err := VariablesOf(sheetSource(), "ws", workflowRef("franquias"))
	if err != nil {
		t.Fatal(err)
	}
	need := vars.Required.Find("planilha")
	if need == nil || len(vars.Required) != 1 {
		t.Fatalf("required = %+v", vars.Required)
	}
	want := []sheetfit.Use{
		{ModelSheetID: "model-neg", Kind: sheetfit.UseExport, Mapping: map[string]string{}},
		{ModelSheetID: "model-pag", Kind: sheetfit.UseAddRow, Mapping: map[string]string{"valor": "{{var.valor}}", "parcelas": "3"}},
		{ModelSheetID: "model-pag", Kind: sheetfit.UseAgentRow},
	}
	if !reflect.DeepEqual(need.SheetUses, want) {
		t.Fatalf("uses = %+v", need.SheetUses)
	}
}

func TestUnionKeepsTheSheetUsesOfBothSides(t *testing.T) {
	a := Needs{{Name: "planilha", SheetUses: []sheetfit.Use{{ModelSheetID: "m1", Kind: sheetfit.UseExport}}}}
	b := Needs{{Name: "planilha", SheetUses: []sheetfit.Use{{ModelSheetID: "m2", Kind: sheetfit.UseAddRow}}}}
	got := a.Union(b)
	if len(got) != 1 || len(got[0].SheetUses) != 2 {
		t.Fatalf("union = %+v", got)
	}
}

func TestADeclaredDefaultMakesAPlanilhaOptional(t *testing.T) {
	vars := Variables{Required: Needs{{Name: "planilha"}, {Name: "matricula"}}}
	catalog := variabledecl.NewCatalog([]variabledecl.Declaration{
		{Name: "planilha", Type: variabledecl.Type{Kind: variabledecl.KindSheet, Settings: map[string]string{variabledecl.SettingDefault: "sp-default"}}},
	})
	got := vars.WithDeclaredDefaults(catalog)
	if names := got.Required.Names(); !reflect.DeepEqual(names, []string{"matricula"}) {
		t.Fatalf("required = %v", names)
	}
	if len(got.Optional) != 1 || got.Optional[0].Default != "sp-default" {
		t.Fatalf("optional = %+v", got.Optional)
	}
}
