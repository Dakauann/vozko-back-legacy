package node_executors

import (
	"context"
	"testing"

	"vozko/domain/googlesheet"
	"vozko/domain/workflow"
	gsuc "vozko/usecases/googlesheet"
)

type repeatingSheetSink struct{}

func (repeatingSheetSink) Enqueue(_ context.Context, in gsuc.EnqueueInput) (*gsuc.EnqueueResult, error) {
	sheet := &googlesheet.Sheet{ID: in.SheetID, Title: "Pagamentos", SpreadsheetID: "sp-1"}
	return &gsuc.EnqueueResult{RowID: "row-first", Sheet: sheet, Repeat: &googlesheet.RepeatError{Code: googlesheet.CodeAlreadyRecordedConversationMonth, RowID: "row-first"}}, nil
}

func TestARepeatFollowsTheAlreadyRecordedOutputWhenItIsConnected(t *testing.T) {
	ctx := sheetNodeContext(map[string]interface{}{"produto": "x"})
	ctx.Graph.Nodes = append(ctx.Graph.Nodes, workflow.Node{ID: "seen"})
	ctx.Graph.Edges = append(ctx.Graph.Edges, workflow.Edge{Source: "sheet-node", Target: "seen", Label: workflow.SheetStepAlreadyRecorded})
	result, err := NewAddSheetRowExecutor(repeatingSheetSink{}).Execute(ctx)
	if err != nil || result.NextNodeID != "seen" || result.Branch != workflow.SheetStepAlreadyRecorded {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if result.Output["already_recorded"] != true || result.Output["existing_row_id"] != "row-first" || result.Output["row_id"] != "row-first" {
		t.Fatalf("output = %+v", result.Output)
	}
	if result.ErrorKind != "" {
		t.Fatalf("a repeat is not an error: %s", result.ErrorKind)
	}
}

func TestARepeatFollowsSuccessWhenTheAlreadyRecordedOutputIsNotConnected(t *testing.T) {
	result, err := NewAddSheetRowExecutor(repeatingSheetSink{}).Execute(sheetNodeContext(map[string]interface{}{"produto": "x"}))
	if err != nil || result.NextNodeID != "ok" || result.Output["already_recorded"] != true || result.Output["existing_row_id"] != "row-first" {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestAFreshRowSaysItWasNotRecordedBefore(t *testing.T) {
	result, err := NewAddSheetRowExecutor(&sheetSink{}).Execute(sheetNodeContext(map[string]interface{}{"produto": "x"}))
	if err != nil || result.Output["already_recorded"] != false || result.Output["existing_row_id"] != "" {
		t.Fatalf("output = %+v, %v", result.Output, err)
	}
}

func TestTheSheetStepOffersAnOptionalAlreadyRecordedOutput(t *testing.T) {
	def := NewAddSheetRowExecutor(&sheetSink{}).Definition()
	var found *workflow.HandleDefinition
	for i := range def.Outputs {
		if def.Outputs[i].ID == workflow.SheetStepAlreadyRecorded {
			found = &def.Outputs[i]
		}
	}
	if found == nil || !found.Optional || found.Label != "Já registrada" {
		t.Fatalf("outputs = %+v", def.Outputs)
	}
	keys := map[string]bool{}
	for _, key := range def.OutputKeys {
		keys[key.Key] = true
	}
	if !keys["already_recorded"] || !keys["existing_row_id"] {
		t.Fatalf("output keys = %+v", def.OutputKeys)
	}
}
