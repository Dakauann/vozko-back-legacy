package workflow

import (
	"errors"
	"testing"
)

func exportStep(id, tab, variable string) Node {
	return Node{ID: id, Type: NodeTypeActionExportMessagesSheet, Config: map[string]interface{}{SheetStepSheetKey: tab, SheetStepVariableKey: variable}}
}

func addRowStep(id, tab, variable string) Node {
	return Node{ID: id, Type: NodeTypeActionAddSheetRow, Config: map[string]interface{}{SheetStepSheetKey: tab, SheetStepVariableKey: variable}}
}

func TestALinkedRowAndItsExportWriteToTheSameSpreadsheet(t *testing.T) {
	links := SheetLinks{"model-pag": {{Column: "negociacao", Header: "Negociação", TargetID: "model-neg", TargetTitle: "Negociações"}}}
	cases := []struct {
		name  string
		nodes []Node
		bad   bool
	}{
		{name: "both per conversation", nodes: []Node{exportStep("e", "model-neg", "{{campvars.planilha}}"), addRowStep("a", "model-pag", "{{campvars.planilha}}")}},
		{name: "both fixed", nodes: []Node{exportStep("e", "model-neg", ""), addRowStep("a", "model-pag", "")}},
		{name: "fixed export, row per conversation", nodes: []Node{exportStep("e", "model-neg", ""), addRowStep("a", "model-pag", "{{campvars.planilha}}")}, bad: true},
		{name: "export per conversation, fixed row", nodes: []Node{exportStep("e", "model-neg", "{{campvars.planilha}}"), addRowStep("a", "model-pag", "")}, bad: true},
		{name: "another variable", nodes: []Node{exportStep("e", "model-neg", "{{campvars.outra}}"), addRowStep("a", "model-pag", "{{campvars.planilha}}")}, bad: true},
		{name: "one matching export is enough", nodes: []Node{exportStep("e1", "model-neg", ""), exportStep("e2", "model-neg", "{{campvars.planilha}}"), addRowStep("a", "model-pag", "{{campvars.planilha}}")}},
		{name: "no export at all is the export lint's job", nodes: []Node{addRowStep("a", "model-pag", "{{campvars.planilha}}")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			problems := SheetVariableLinkProblems(&Graph{Nodes: c.nodes}, links)
			if c.bad != (len(problems) == 1) || (c.bad && !errors.Is(problems[0], ErrNodeSheetVariableInvalid)) {
				t.Fatalf("problems = %v", problems)
			}
		})
	}
}
