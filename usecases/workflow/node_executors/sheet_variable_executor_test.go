package node_executors

import (
	"testing"

	"vozko/domain/workflow"
)

func variableSheetContext(pinned bool) *workflow.NodeContext {
	ctx := sheetNodeContext(map[string]interface{}{"produto": "{{var.produto}}"})
	ctx.Node.Config[workflow.SheetStepVariableKey] = "{{campvars.planilha}}"
	ctx.Node.Config["sheet_id"] = "model-pag"
	if pinned {
		workflow.PinSheetFiles(ctx.State, map[string]workflow.SheetFile{"planilha": {SpreadsheetID: "sp-centro", Tabs: map[string]string{"model-pag": "centro-pag"}}})
	}
	return ctx
}

func TestAStepPerConversationWritesToThePinnedTabAndNeverToTheModel(t *testing.T) {
	sink := &sheetSink{}
	result, err := NewAddSheetRowExecutor(sink).Execute(variableSheetContext(true))
	if err != nil || result.NextNodeID != "ok" || sink.got[0].SheetID != "centro-pag" {
		t.Fatalf("result = %+v, %v, got %+v", result, err, sink.got)
	}
	unpinned := &sheetSink{}
	result, err = NewAddSheetRowExecutor(unpinned).Execute(variableSheetContext(false))
	if err != nil || result.NextNodeID != "bad" || len(unpinned.got) != 0 || result.Output["error_code"] != workflow.CodeSheetVariableUnresolved {
		t.Fatalf("unpinned = %+v, %v, got %+v", result, err, unpinned.got)
	}
}

func TestAnExportPerConversationUsesThePinnedTab(t *testing.T) {
	pinned := variableSheetContext(true)
	pinned.Node.Type = workflow.NodeTypeActionExportMessagesSheet
	desk := &exportDesk{}
	result, err := NewExportMessagesSheetExecutor(desk, &conversationTape{messages: chargeConversation()}).Execute(pinned)
	if err != nil || result.NextNodeID != "ok" || desk.recorded[0].Target.SheetID != "centro-pag" {
		t.Fatalf("result = %+v, %v, recorded %+v", result, err, desk.recorded)
	}
	unpinned := variableSheetContext(false)
	unpinned.Node.Type = workflow.NodeTypeActionExportMessagesSheet
	empty := &exportDesk{}
	result, err = NewExportMessagesSheetExecutor(empty, &conversationTape{messages: chargeConversation()}).Execute(unpinned)
	if err != nil || result.NextNodeID != "bad" || len(empty.recorded) != 0 || result.Output["error_code"] != workflow.CodeSheetVariableUnresolved {
		t.Fatalf("unpinned = %+v, %v", result, err)
	}
}
