package workflow_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/automationneeds"
	"vozko/domain/googlesheet"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

type sheetDeclarations struct {
	types map[string]variabledecl.Type
}

func (s sheetDeclarations) Rejected(context.Context, string, map[string]string) ([]string, error) {
	return nil, nil
}

func (s sheetDeclarations) Label(_ context.Context, _ string, needs automationneeds.Needs) automationneeds.Needs {
	out := make(automationneeds.Needs, len(needs))
	for i, need := range needs {
		need.Type = variabledecl.Type{Kind: variabledecl.KindText}
		if typ, ok := s.types[need.Name]; ok {
			need.Declared, need.Type = true, typ
		}
		out[i] = need
	}
	return out
}

func modelTab(t *testing.T) *googlesheet.Sheet {
	t.Helper()
	sheet, err := googlesheet.NewSheet(googlesheet.NewSheetInput{
		WorkspaceID: "ws-1", AccountID: "a", Title: "Pagamentos",
		Columns: []googlesheet.ColumnInput{{Header: "Valor", Type: googlesheet.TypeMoney}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sheet.ID, sheet.SpreadsheetID = "model-pag", "sp-model"
	return sheet
}

func variableStep(variable string) *workflow.Node {
	return &workflow.Node{ID: "n1", Type: workflow.NodeTypeActionAddSheetRow, Config: map[string]interface{}{
		"sheet_id": "model-pag", "values": map[string]interface{}{"valor": "1"}, workflow.SheetStepVariableKey: variable,
	}}
}

func TestAStepThatChangesSpreadsheetPerConversationNeedsAPlanilhaWhoseModelHoldsItsTab(t *testing.T) {
	declarations := sheetDeclarations{types: map[string]variabledecl.Type{
		"planilha": {Kind: variabledecl.KindSheet, Settings: map[string]string{variabledecl.SettingModel: "sp-model"}},
		"outra":    {Kind: variabledecl.KindSheet, Settings: map[string]string{variabledecl.SettingModel: "sp-other"}},
		"texto":    {Kind: variabledecl.KindText},
	}}
	v := &googleSheetValidator{sheets: sheetLookup{sheet: modelTab(t)}, workspaceID: "ws-1", declarations: declarations}
	if err := v.Validate(variableStep("{{campvars.planilha}}")); err != nil {
		t.Fatalf("a valid step was refused: %v", err)
	}
	for name, variable := range map[string]string{
		"undeclared":           "{{campvars.nada}}",
		"not a Planilha":       "{{campvars.texto}}",
		"another model":        "{{campvars.outra}}",
		"not one conversation": "{{lead.name}}",
	} {
		if err := v.Validate(variableStep(variable)); !errors.Is(err, workflow.ErrNodeSheetVariableInvalid) {
			t.Fatalf("%s = %v", name, err)
		}
	}
	voice := &googleSheetValidator{sheets: sheetLookup{sheet: modelTab(t)}, workspaceID: "ws-1", declarations: declarations, voice: true}
	if err := voice.Validate(variableStep("{{campvars.planilha}}")); !errors.Is(err, workflow.ErrNodeSheetVariableInvalid) {
		t.Fatalf("a voice workflow = %v", err)
	}
	blind := &googleSheetValidator{sheets: sheetLookup{sheet: modelTab(t)}, workspaceID: "ws-1"}
	if err := blind.Validate(variableStep("{{campvars.planilha}}")); !errors.Is(err, workflow.ErrValidationUnavailable) {
		t.Fatalf("without declarations = %v", err)
	}
	fixed := &googleSheetValidator{sheets: sheetLookup{sheet: modelTab(t)}, workspaceID: "ws-1"}
	if err := fixed.Validate(variableStep("")); err != nil {
		t.Fatalf("a fixed step = %v", err)
	}
}
