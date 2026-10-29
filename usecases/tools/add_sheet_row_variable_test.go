package tools_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agenttool"
	"vozko/domain/automationneeds"
	"vozko/domain/shared"
	"vozko/domain/tools"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

type declaredSheets struct {
	catalog variabledecl.Catalog
}

func (declaredSheets) AgentSheetFiles(context.Context, string, string, shared.EntryType, string) (map[string]workflow.SheetFile, error) {
	return nil, errors.New("not used")
}

func (d declaredSheets) Label(_ context.Context, _ string, needs automationneeds.Needs) automationneeds.Needs {
	return needs.Labeled(d.catalog)
}

func sheetVariableBinding(variable string) map[string]interface{} {
	return map[string]interface{}{addSheetRowConfigKey: "sheet-1", addSheetRowVariableKey: variable}
}

func TestAPerConversationSheetMustBeADeclaredPlanilhaOfTheTabModel(t *testing.T) {
	sheet := sheetForTool(t)
	sheet.SpreadsheetID = "sp-model"
	conversations := declaredSheets{catalog: variabledecl.NewCatalog([]variabledecl.Declaration{
		{Name: "franquia", Label: "Planilha da franquia", Type: variabledecl.Type{Kind: variabledecl.KindSheet, Settings: map[string]string{variabledecl.SettingModel: "sp-model"}}},
		{Name: "outra", Label: "Outra planilha", Type: variabledecl.Type{Kind: variabledecl.KindSheet, Settings: map[string]string{variabledecl.SettingModel: "sp-other"}}},
		{Name: "valor", Label: "Valor", Type: variabledecl.Type{Kind: variabledecl.KindNumber}},
	})}
	validator := NewAddSheetRowTool(&sheetRowsStub{sheet: sheet}, conversations).(tools.BindingValidator)
	if err := validator.ValidateBinding(context.Background(), "ws-1", sheetVariableBinding("{{campvars.franquia}}")); err != nil {
		t.Fatal(err)
	}
	if err := validator.ValidateBinding(context.Background(), "ws-1", map[string]interface{}{addSheetRowConfigKey: "sheet-1"}); err != nil {
		t.Fatalf("a fixed sheet needs no variable: %v", err)
	}
	for _, variable := range []string{"{{campvars.outra}}", "{{campvars.valor}}", "{{campvars.livre}}"} {
		problem, ok := tools.ProblemOf(validator.ValidateBinding(context.Background(), "ws-1", sheetVariableBinding(variable)))
		if !ok || problem.Code != agenttool.CodeSheetVariableInvalid {
			t.Fatalf("%s: problem = %+v %v", variable, problem, ok)
		}
	}
	if problem, ok := tools.ProblemOf(validator.ValidateBinding(context.Background(), "ws-1", sheetVariableBinding("12"))); !ok || problem.Code != agenttool.CodeSheetVariableInvalid {
		t.Fatalf("unreadable variable = %+v %v", problem, ok)
	}
}

func TestAPerConversationSheetFailsClosedWithoutTheDeclarations(t *testing.T) {
	sheet := sheetForTool(t)
	sheet.SpreadsheetID = "sp-model"
	err := NewAddSheetRowTool(&sheetRowsStub{sheet: sheet}, nil).(tools.BindingValidator).ValidateBinding(context.Background(), "ws-1", sheetVariableBinding("{{campvars.franquia}}"))
	if _, isProblem := tools.ProblemOf(err); err == nil || isProblem {
		t.Fatalf("err = %v; a check that cannot run is an outage, never a pass", err)
	}
}
