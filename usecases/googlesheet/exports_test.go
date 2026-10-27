package googlesheet

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"vozko/domain/googlesheet"
)

type memoryExports struct {
	mu        sync.Mutex
	positions map[string]googlesheet.ExportPosition
	keys      map[string]bool
	recorded  []googlesheet.MessageExport
	fail      error
}

func newMemoryExports() *memoryExports {
	return &memoryExports{positions: map[string]googlesheet.ExportPosition{}, keys: map[string]bool{}}
}

func exportKey(sheetID, entryID, entryType string) string { return sheetID + "/" + entryType + "/" + entryID }

func (m *memoryExports) Position(_ context.Context, sheetID, entryID, entryType string) (googlesheet.ExportPosition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.positions[exportKey(sheetID, entryID, entryType)], nil
}

func (m *memoryExports) Record(_ context.Context, export googlesheet.MessageExport) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return 0, m.fail
	}
	m.recorded = append(m.recorded, export)
	inserted := 0
	for _, row := range export.Rows {
		if !m.keys[row.SheetID+row.IdempotencyKey] {
			m.keys[row.SheetID+row.IdempotencyKey] = true
			inserted++
		}
	}
	key := exportKey(export.SheetID, export.EntryID, export.EntryType)
	if current := m.positions[key]; current.Before(export.Through) {
		m.positions[key] = export.Through
	}
	return inserted, nil
}

type exportWorld struct {
	*world
	exports *memoryExports
	uc      *Exports
	target  ExportTarget
}

func newExportWorld(t *testing.T) *exportWorld {
	t.Helper()
	w := newWorld(t)
	timeline, err := googlesheet.NewSheet(googlesheet.NewSheetInput{
		WorkspaceID: testWorkspace, AccountID: "acct-1", Title: "Cobranças",
		Columns: []googlesheet.ColumnInput{
			{Header: "Quando", Type: googlesheet.TypeDateTime, Source: googlesheet.SourceMessageSentAt},
			{Header: "Mensagem", Type: googlesheet.TypeLongText, Source: googlesheet.SourceMessageText},
			{Header: "Comprovante", Type: googlesheet.TypeURL, Source: googlesheet.SourceMessageMediaURL},
			{Header: "Telefone", Type: googlesheet.TypePhone, Source: googlesheet.SourceLeadPhone},
			{Header: "Valor", Type: googlesheet.TypeMoney, Required: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	timeline.SpreadsheetID = "sp-2"
	if err := w.sheets.Create(context.Background(), timeline); err != nil {
		t.Fatal(err)
	}
	exports := newMemoryExports()
	uc, err := NewExports(ExportsDeps{Intake: w.intake, Exports: exports})
	if err != nil {
		t.Fatal(err)
	}
	return &exportWorld{world: w, exports: exports, uc: uc, target: ExportTarget{
		WorkspaceID: testWorkspace, SheetID: timeline.ID,
		Origin: googlesheet.Origin{Kind: googlesheet.OriginWorkflow, WorkflowID: "wf-1", RunID: "run-1", EntryID: "conv-1", EntryType: "whatsapp"},
	}}
}

func conversationMessages() []googlesheet.ExportedMessage {
	at := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
	return []googlesheet.ExportedMessage{
		{ID: "m1", At: at, Sender: "Ana", Inbound: true, Text: "quanto devo?"},
		{ID: "m2", At: at.Add(time.Minute), Sender: "Assistente", Text: "R$ 120,50, segue o link"},
		{ID: "m3", At: at.Add(2 * time.Minute), Sender: "Ana", Inbound: true, MediaURL: "https://cdn.example.com/receipt.jpg"},
	}
}

func TestAnExportWritesOneRowPerMessageInOneRecordAndMovesTheCursor(t *testing.T) {
	w := newExportWorld(t)
	ctx := context.Background()
	sheet, start, err := w.uc.Start(ctx, w.target)
	if err != nil || !start.IsZero() || sheet.ID != w.target.SheetID {
		t.Fatalf("Start = %v, %+v, %v", sheet, start, err)
	}
	messages := conversationMessages()

	result, err := w.uc.Record(ctx, ExportInput{Target: w.target, Values: map[string]any{"valor": "120,50"}, Messages: messages, Through: messages[2].Position()})
	if err != nil || result.Exported != 3 || result.Duplicates != 0 {
		t.Fatalf("Record = %+v, %v", result, err)
	}
	if len(w.exports.recorded) != 1 || len(w.exports.recorded[0].Rows) != 3 {
		t.Fatalf("recorded %d exports: one export must be one batch", len(w.exports.recorded))
	}
	rows := w.exports.recorded[0].Rows
	if rows[2].Cells["comprovante"] != "https://cdn.example.com/receipt.jpg" || rows[0].Cells["mensagem"] != "quanto devo?" || rows[1].Cells["telefone"] != "+5511999990000" || rows[0].Cells["valor"] != 120.5 {
		t.Fatalf("rows = %v / %v / %v", rows[0].Cells, rows[1].Cells, rows[2].Cells)
	}
	if w.resolver.calls != 1 {
		t.Fatalf("the customer values were resolved %d times; they are the same on every row", w.resolver.calls)
	}
	if len(w.nudger.sheets) == 0 || w.nudger.sheets[len(w.nudger.sheets)-1] != w.target.SheetID {
		t.Fatalf("the writer was not nudged: %v", w.nudger.sheets)
	}
	_, next, err := w.uc.Start(ctx, w.target)
	if err != nil || next != messages[2].Position() {
		t.Fatalf("cursor = %+v, %v", next, err)
	}

	again, err := w.uc.Record(ctx, ExportInput{Target: w.target, Values: map[string]any{"valor": "120,50"}, Messages: messages, Through: messages[2].Position()})
	if err != nil || again.Exported != 0 || again.Duplicates != 3 {
		t.Fatalf("a retried export = %+v, %v: rows must never be written twice", again, err)
	}
}

func TestAnExportThatTheSheetRefusesLeavesTheCursorAlone(t *testing.T) {
	w := newExportWorld(t)
	ctx := context.Background()
	messages := conversationMessages()

	_, err := w.uc.Record(ctx, ExportInput{Target: w.target, Messages: messages, Through: messages[2].Position()})
	if !errors.Is(err, googlesheet.ErrRequiredColumnMissing) {
		t.Fatalf("err = %v, want the required column refusal", err)
	}
	if len(w.exports.recorded) != 0 {
		t.Fatal("nothing may be recorded when one row is refused")
	}
	w.exports.fail = errors.New("database down")
	if _, err := w.uc.Record(ctx, ExportInput{Target: w.target, Values: map[string]any{"valor": "1"}, Messages: messages, Through: messages[2].Position()}); err == nil {
		t.Fatal("a failed record must reach the caller so the step retries")
	}
	if _, pos, _ := w.uc.Start(ctx, w.target); !pos.IsZero() {
		t.Fatalf("cursor = %+v, it must stay put until the rows are stored", pos)
	}
}

func TestAnExportWithOnlyHiddenMessagesStillMovesPastThem(t *testing.T) {
	w := newExportWorld(t)
	through := googlesheet.ExportPosition{At: time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC), MessageID: "tool-call"}

	result, err := w.uc.Record(context.Background(), ExportInput{Target: w.target, Through: through})
	if err != nil || result.Exported != 0 {
		t.Fatalf("Record = %+v, %v", result, err)
	}
	if _, pos, _ := w.uc.Start(context.Background(), w.target); pos != through {
		t.Fatalf("cursor = %+v", pos)
	}
}

func TestAnExportIsRefusedWithoutAConversationOrMessageColumns(t *testing.T) {
	w := newExportWorld(t)
	ctx := context.Background()
	noConversation := w.target
	noConversation.Origin.EntryID = ""
	if _, _, err := w.uc.Start(ctx, noConversation); !errors.Is(err, googlesheet.ErrNeedsConversation) {
		t.Fatalf("err = %v", err)
	}
	if _, err := w.uc.Record(ctx, ExportInput{Target: noConversation}); !errors.Is(err, googlesheet.ErrNeedsConversation) {
		t.Fatalf("err = %v", err)
	}
	plain := w.target
	plain.SheetID = w.sheet.ID
	if _, _, err := w.uc.Start(ctx, plain); !errors.Is(err, googlesheet.ErrNoMessageColumns) {
		t.Fatalf("err = %v: a sheet without message columns would repeat one row per message", err)
	}
	other := w.target
	other.WorkspaceID = "another-workspace"
	if _, _, err := w.uc.Start(ctx, other); !errors.Is(err, googlesheet.ErrSheetNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestExportsRefuseToStartWithoutTheirDependencies(t *testing.T) {
	if _, err := NewExports(ExportsDeps{}); err == nil {
		t.Fatal("an exporter without the intake and the export store must not be built")
	}
}
