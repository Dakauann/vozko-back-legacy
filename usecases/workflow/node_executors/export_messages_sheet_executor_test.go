package node_executors

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/conversation"
	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/domain/workflow"
	gsuc "vozko/usecases/googlesheet"
)

type exportDesk struct {
	position googlesheet.ExportPosition
	startErr error
	recorded []gsuc.ExportInput
	err      error
}

func (d *exportDesk) Start(_ context.Context, target gsuc.ExportTarget) (*googlesheet.Sheet, googlesheet.ExportPosition, error) {
	if d.startErr != nil {
		return nil, googlesheet.ExportPosition{}, d.startErr
	}
	return &googlesheet.Sheet{ID: target.SheetID, Title: "Cobranças", SpreadsheetID: "sp-1"}, d.position, nil
}

func (d *exportDesk) Record(_ context.Context, in gsuc.ExportInput) (*gsuc.ExportResult, error) {
	d.recorded = append(d.recorded, in)
	if d.err != nil {
		return nil, d.err
	}
	return &gsuc.ExportResult{Sheet: &googlesheet.Sheet{ID: in.Target.SheetID, Title: "Cobranças", SpreadsheetID: "sp-1"}, Exported: len(in.Messages)}, nil
}

type conversationTape struct {
	messages []*conversation.Message
	since    conversation.MessageKeyset
	limit    int
	err      error
}

func (c *conversationTape) MessagesSince(_ string, _ shared.EntryType, since conversation.MessageKeyset, limit int) ([]*conversation.Message, error) {
	c.since, c.limit = since, limit
	return c.messages, c.err
}

func exportNodeContext(values map[string]interface{}) *workflow.NodeContext {
	ctx := sheetNodeContext(values)
	ctx.Node.Type = workflow.NodeTypeActionExportMessagesSheet
	return ctx
}

func chargeConversation() []*conversation.Message {
	at := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
	return []*conversation.Message{
		{ID: "m1", MessageType: conversation.MessageTypeUserMessage, Text: "quanto devo?", SentBy: conversation.SentByContact("5511"), CreatedAt: at},
		{ID: "m2", MessageType: conversation.MessageTypeToolCall, Text: "{}", SentBy: conversation.SentByAI("agent-1"), CreatedAt: at.Add(time.Second)},
		{ID: "m3", MessageType: conversation.MessageTypeMedia, Media: &conversation.AttachedMedia{URL: "https://cdn.example.com/receipt.jpg"}, SentBy: conversation.SentByContact("5511"), CreatedAt: at.Add(time.Minute)},
		{ID: "m4", MessageType: conversation.MessageTypeToolResult, Text: "{}", CreatedAt: at.Add(2 * time.Minute)},
	}
}

func TestExportingReadsSinceTheCursorAndRecordsEveryVisibleMessageInOneCall(t *testing.T) {
	cursor := googlesheet.ExportPosition{At: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC), MessageID: "m0"}
	desk := &exportDesk{position: cursor}
	tape := &conversationTape{messages: chargeConversation()}

	result, err := NewExportMessagesSheetExecutor(desk, tape).Execute(exportNodeContext(map[string]interface{}{"produto": "{{var.produto}}"}))
	if err != nil || result.NextNodeID != "ok" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if tape.since.At != cursor.At || tape.since.ID != "m0" || tape.limit != googlesheet.MaxExportMessages {
		t.Fatalf("read since %+v limit %d", tape.since, tape.limit)
	}
	if len(desk.recorded) != 1 {
		t.Fatalf("recorded %d times, want one batch", len(desk.recorded))
	}
	in := desk.recorded[0]
	if len(in.Messages) != 2 || in.Messages[0].ID != "m1" || in.Messages[1].MediaURL != "https://cdn.example.com/receipt.jpg" {
		t.Fatalf("messages = %+v", in.Messages)
	}
	if in.Through.MessageID != "m4" {
		t.Fatalf("through = %+v: the cursor must move past every message read, even hidden ones", in.Through)
	}
	if in.Values["produto"] != "Pizza" || in.Target.SheetID != "sheet-1" || in.Target.Origin.Kind != googlesheet.OriginWorkflow || in.Target.Origin.EntryID != "entry-1" || in.Target.Origin.RunID != "run-1" {
		t.Fatalf("input = %+v", in)
	}
	if result.Output["exported"] != 2 || result.Output["has_more"] != false || result.Output["sheet_title"] != "Cobranças" {
		t.Fatalf("output = %+v", result.Output)
	}
}

func TestExportingANewlyQuietConversationRecordsNothing(t *testing.T) {
	desk := &exportDesk{}
	result, err := NewExportMessagesSheetExecutor(desk, &conversationTape{}).Execute(exportNodeContext(nil))
	if err != nil || result.NextNodeID != "ok" || len(desk.recorded) != 0 || result.Output["exported"] != 0 {
		t.Fatalf("result = %+v, %v, recorded %d", result, err, len(desk.recorded))
	}
}

func TestExportingSaysWhenMoreMessagesWait(t *testing.T) {
	full := make([]*conversation.Message, googlesheet.MaxExportMessages)
	for i := range full {
		full[i] = &conversation.Message{ID: "m", MessageType: conversation.MessageTypeUserMessage, Text: "oi", CreatedAt: time.Unix(int64(i), 0)}
	}
	result, err := NewExportMessagesSheetExecutor(&exportDesk{}, &conversationTape{messages: full}).Execute(exportNodeContext(nil))
	if err != nil || result.Output["has_more"] != true {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestExportingRoutesRefusalsAndRetriesInfrastructure(t *testing.T) {
	ctx := exportNodeContext(map[string]interface{}{"produto": "{{var.inexistente}}"})
	result, err := NewExportMessagesSheetExecutor(&exportDesk{}, &conversationTape{messages: chargeConversation()}).Execute(ctx)
	if err != nil || result.NextNodeID != "bad" || result.Output["error_code"] != googlesheet.CodeUnresolvedVariable {
		t.Fatalf("unresolved = %+v, %v", result, err)
	}

	noColumns := &exportDesk{startErr: googlesheet.ErrNoMessageColumns}
	result, err = NewExportMessagesSheetExecutor(noColumns, &conversationTape{}).Execute(exportNodeContext(nil))
	if err != nil || result.NextNodeID != "bad" || result.Output["error_code"] != googlesheet.CodeNoMessageColumns {
		t.Fatalf("no message columns = %+v, %v", result, err)
	}

	refused := &exportDesk{err: &googlesheet.ValueError{Column: "valor", Header: "Valor", Reason: googlesheet.ErrRequiredColumnMissing}}
	result, err = NewExportMessagesSheetExecutor(refused, &conversationTape{messages: chargeConversation()}).Execute(exportNodeContext(nil))
	if err != nil || result.NextNodeID != "bad" || result.Output["error_code"] != googlesheet.CodeRequiredColumnMissing {
		t.Fatalf("refusal = %+v, %v", result, err)
	}

	if _, err := NewExportMessagesSheetExecutor(&exportDesk{err: errors.New("db down")}, &conversationTape{messages: chargeConversation()}).Execute(exportNodeContext(nil)); err == nil {
		t.Fatal("a database failure must be retried by the engine, so the same messages are tried again")
	}
	if _, err := NewExportMessagesSheetExecutor(&exportDesk{}, &conversationTape{err: errors.New("db down")}).Execute(exportNodeContext(nil)); err == nil {
		t.Fatal("an unreadable conversation must be retried, never exported as empty")
	}
}

func TestExportingNeedsAConversation(t *testing.T) {
	ctx := exportNodeContext(nil)
	ctx.Run.EntryID = ""
	result, err := NewExportMessagesSheetExecutor(&exportDesk{}, &conversationTape{}).Execute(ctx)
	if err != nil || result.NextNodeID != "bad" || result.Output["error_code"] != googlesheet.CodeNeedsConversation {
		t.Fatalf("result = %+v, %v", result, err)
	}
	unwired, err := NewExportMessagesSheetExecutor(nil, nil).Execute(exportNodeContext(nil))
	if err != nil || unwired.NextNodeID != "bad" || unwired.Output["error_code"] != googlesheet.CodeInternal {
		t.Fatalf("unwired = %+v, %v", unwired, err)
	}
}

func TestTheExportNodeIsAConversationExternalCallWithTheSheetPanel(t *testing.T) {
	def := NewExportMessagesSheetExecutor(&exportDesk{}, &conversationTape{}).Definition()
	if def.TestBehavior != workflow.NodeTestExternalCall || def.Type != workflow.NodeTypeActionExportMessagesSheet || !def.NeedsConversation {
		t.Fatalf("definition = %+v", def)
	}
	if err := workflow.TestBehaviorConsistent(def); err != nil {
		t.Fatal(err)
	}
	fields := map[string]workflow.ConfigField{}
	for _, f := range def.ConfigSchema {
		fields[f.Key] = f
	}
	if fields["sheet_id"].OptionsSource != "google_sheets" || fields["values"].Type != "sheet_columns" || fields["values"].Required {
		t.Fatalf("schema = %+v", def.ConfigSchema)
	}
}
