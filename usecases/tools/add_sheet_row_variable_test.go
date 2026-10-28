package tools_usecase

import (
	"context"
	"testing"

	"vozko/domain/shared"
	"vozko/domain/workflow"
)

type conversationSheetsStub struct {
	files map[string]workflow.SheetFile
	err   error
	asked string
}

func (c *conversationSheetsStub) AgentSheetFiles(_ context.Context, _, _ string, _ shared.EntryType, agentID string) (map[string]workflow.SheetFile, error) {
	c.asked = agentID
	return c.files, c.err
}

func variableConfig() map[string]interface{} {
	config := agentConfig()
	config[addSheetRowVariableKey] = "{{campvars.planilha}}"
	return config
}

func TestAnAgentWritesToTheSpreadsheetOfTheConversationAndNeverToTheModel(t *testing.T) {
	stub := &sheetRowsStub{sheet: sheetForTool(t)}
	conversations := &conversationSheetsStub{files: map[string]workflow.SheetFile{"planilha": {SpreadsheetID: "sp-centro", Tabs: map[string]string{"sheet-1": "centro-1"}}}}
	result, err := NewAddSheetRowTool(stub, conversations).ExecuteWithConfig(context.Background(), variableConfig(), map[string]interface{}{"produto": "Pizza"})
	if err != nil || result.IsError || len(stub.enqueued) != 1 || stub.enqueued[0].SheetID != "centro-1" || conversations.asked != "agent-1" {
		t.Fatalf("result = %+v, %v, enqueued %+v", result, err, stub.enqueued)
	}
	for name, sheets := range map[string]ConversationSheets{
		"no spreadsheet for the conversation": &conversationSheetsStub{err: context.Canceled},
		"another variable":                    &conversationSheetsStub{files: map[string]workflow.SheetFile{"outra": {Tabs: map[string]string{"sheet-1": "x"}}}},
		"no tab for the model":                &conversationSheetsStub{files: map[string]workflow.SheetFile{"planilha": {Tabs: map[string]string{}}}},
		"not wired":                           nil,
	} {
		refused := &sheetRowsStub{sheet: sheetForTool(t)}
		result, err := NewAddSheetRowTool(refused, sheets).ExecuteWithConfig(context.Background(), variableConfig(), map[string]interface{}{"produto": "Pizza"})
		if err != nil || !result.IsError || result.ErrorCode != workflow.CodeSheetVariableUnresolved || len(refused.enqueued) != 0 {
			t.Fatalf("%s = %+v, %v", name, result, err)
		}
	}
}

func TestTheAgentToolOffersTheChoiceOfAPlanilhaPerConversation(t *testing.T) {
	field, ok := NewAddSheetRowTool(&sheetRowsStub{sheet: sheetForTool(t)}, nil).Definition().ConfigSchema[addSheetRowVariableKey]
	if !ok || field.OptionsSource != ConversationSheetVariablesOptionsSource || field.Required {
		t.Fatalf("field = %+v", field)
	}
}
