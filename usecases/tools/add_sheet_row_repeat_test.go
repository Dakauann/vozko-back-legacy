package tools_usecase

import (
	"context"
	"strings"
	"testing"

	"vozko/domain/googlesheet"
)

func TestARepeatTellsTheModelNotToRegisterAgainWithoutAnError(t *testing.T) {
	stub := &sheetRowsStub{sheet: sheetForTool(t), repeat: &googlesheet.RepeatError{Code: googlesheet.CodeAlreadyRecordedConversationMonth, RowID: "row-1"}}
	result, err := NewAddSheetRowTool(stub, nil).ExecuteWithConfig(context.Background(), agentConfig(), map[string]interface{}{"produto": "Pizza"})
	if err != nil || result.IsError || result.ErrorCode != "" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if text, _ := result.Result.(string); text != "Esta linha já estava registrada; não registre de novo." {
		t.Fatalf("text = %q", text)
	}
}

func TestAMissingUniqueValueIsExplainedToTheModel(t *testing.T) {
	stub := &sheetRowsStub{sheet: sheetForTool(t), enqErr: &googlesheet.ValueError{Column: "matricula", Header: "Matrícula", Reason: googlesheet.ErrUniqueValueMissing}}
	result, _ := NewAddSheetRowTool(stub, nil).ExecuteWithConfig(context.Background(), agentConfig(), map[string]interface{}{"produto": "Pizza"})
	text, _ := result.Result.(string)
	if !result.IsError || result.ErrorCode != googlesheet.CodeUniqueValueMissing || !strings.Contains(text, "Matrícula") || !strings.Contains(text, "não registrar a mesma linha duas vezes") {
		t.Fatalf("result = %+v", result)
	}
}
