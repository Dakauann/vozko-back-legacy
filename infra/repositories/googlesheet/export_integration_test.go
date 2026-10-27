package googlesheet_repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/googlesheet"
	"vozko/infra/database/schema"
)

func exportOf(t *testing.T, s *googlesheet.Sheet, entryID string, ids []string, through googlesheet.ExportPosition, now time.Time) googlesheet.MessageExport {
	t.Helper()
	in := make([]googlesheet.MessageRow, len(ids))
	for i, id := range ids {
		in[i] = googlesheet.MessageRow{MessageID: id, Cells: googlesheet.Cells{"nome": "mensagem " + id}}
	}
	origin := googlesheet.Origin{Kind: googlesheet.OriginWorkflow, WorkflowID: "wf-1", RunID: "run-1", EntryID: entryID, EntryType: "whatsapp"}
	rows, err := googlesheet.NewMessageRows(s, in, origin, now)
	if err != nil {
		t.Fatal(err)
	}
	return googlesheet.MessageExport{WorkspaceID: s.WorkspaceID, SheetID: s.ID, EntryID: entryID, EntryType: "whatsapp", Rows: rows, Through: through}
}

func TestAnExportWritesItsRowsAndItsCursorTogether(t *testing.T) {
	db := integrationDB(t)
	sheets, exports, rows := NewSheetRepository(db), NewExportRepository(db), NewRowRepository(db)
	ctx := context.Background()
	s := storedSheet(t, sheets)
	entryID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)

	start, err := exports.Position(ctx, s.ID, entryID, "whatsapp")
	if err != nil || !start.IsZero() {
		t.Fatalf("a conversation never exported starts at %+v, %v", start, err)
	}
	through := googlesheet.ExportPosition{At: now.Add(-time.Minute), MessageID: "m3"}
	inserted, err := exports.Record(ctx, exportOf(t, s, entryID, []string{"m1", "m2", "m3"}, through, now))
	if err != nil || inserted != 3 {
		t.Fatalf("Record = %d, %v", inserted, err)
	}
	got, err := exports.Position(ctx, s.ID, entryID, "whatsapp")
	if err != nil || !got.At.Equal(through.At) || got.MessageID != "m3" {
		t.Fatalf("cursor = %+v, %v, want %+v", got, err, through)
	}
	claimed, err := rows.Claim(ctx, s.ID, uuid.NewString(), 10, now.Add(time.Second))
	if err != nil || len(claimed) != 3 {
		t.Fatalf("claimed %d rows, %v: one export is one batch", len(claimed), err)
	}
	for i, row := range claimed {
		if want := fmt.Sprintf("mensagem m%d", i+1); row.Cells["nome"] != want {
			t.Fatalf("claimed[%d] = %v, want %s: rows must reach the sheet in message order", i, row.Cells["nome"], want)
		}
	}
}

func TestARepeatedExportNeitherDuplicatesRowsNorMovesTheCursorBack(t *testing.T) {
	db := integrationDB(t)
	sheets, exports := NewSheetRepository(db), NewExportRepository(db)
	ctx := context.Background()
	s := storedSheet(t, sheets)
	entryID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)
	ahead := googlesheet.ExportPosition{At: now, MessageID: "m2"}
	behind := googlesheet.ExportPosition{At: now.Add(-time.Hour), MessageID: "m1"}

	if _, err := exports.Record(ctx, exportOf(t, s, entryID, []string{"m1", "m2"}, ahead, now)); err != nil {
		t.Fatal(err)
	}
	inserted, err := exports.Record(ctx, exportOf(t, s, entryID, []string{"m1"}, behind, now.Add(time.Second)))
	if err != nil || inserted != 0 {
		t.Fatalf("a retry inserted %d rows, %v", inserted, err)
	}
	var count int64
	db.Model(&schema.GoogleSheetRow{}).Where("sheet_id = ?", s.ID).Count(&count)
	if count != 2 {
		t.Fatalf("rows = %d, want 2", count)
	}
	got, err := exports.Position(ctx, s.ID, entryID, "whatsapp")
	if err != nil || got.MessageID != "m2" {
		t.Fatalf("cursor = %+v, %v: an older export must never rewind it", got, err)
	}
}

func TestAnExportThatFailsLeavesTheCursorWhereItWas(t *testing.T) {
	db := integrationDB(t)
	sheets, exports := NewSheetRepository(db), NewExportRepository(db)
	ctx := context.Background()
	s := storedSheet(t, sheets)
	entryID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)
	broken := exportOf(t, s, entryID, []string{"m1"}, googlesheet.ExportPosition{At: now, MessageID: "m1"}, now)
	broken.Rows[0].WorkspaceID = "not-a-uuid"

	if _, err := exports.Record(ctx, broken); err == nil {
		t.Fatal("a row the database refuses must fail the export")
	}
	got, err := exports.Position(ctx, s.ID, entryID, "whatsapp")
	if err != nil || !got.IsZero() {
		t.Fatalf("cursor = %+v, %v: it moved although nothing was written", got, err)
	}
}

func TestAnExportWithNothingToWriteStillMovesPastWhatItRead(t *testing.T) {
	db := integrationDB(t)
	sheets, exports := NewSheetRepository(db), NewExportRepository(db)
	ctx := context.Background()
	s := storedSheet(t, sheets)
	entryID := uuid.NewString()
	through := googlesheet.ExportPosition{At: time.Now().UTC().Truncate(time.Microsecond), MessageID: "tool-call"}

	inserted, err := exports.Record(ctx, googlesheet.MessageExport{WorkspaceID: s.WorkspaceID, SheetID: s.ID, EntryID: entryID, EntryType: "whatsapp", Through: through})
	if err != nil || inserted != 0 {
		t.Fatalf("Record = %d, %v", inserted, err)
	}
	if got, _ := exports.Position(ctx, s.ID, entryID, "whatsapp"); got.MessageID != "tool-call" {
		t.Fatalf("cursor = %+v", got)
	}
}

func TestConcurrentExportsOfOneConversationWriteEachMessageOnce(t *testing.T) {
	db := integrationDB(t)
	sheets, exports := NewSheetRepository(db), NewExportRepository(db)
	ctx := context.Background()
	s := storedSheet(t, sheets)
	entryID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)
	ids := []string{"m1", "m2", "m3", "m4"}
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			through := googlesheet.ExportPosition{At: now.Add(time.Duration(w) * time.Second), MessageID: ids[w]}
			if _, err := exports.Record(ctx, exportOf(t, s, entryID, ids[:w+1], through, now)); err != nil {
				t.Error(err)
			}
		}(w)
	}
	wg.Wait()
	var count int64
	db.Model(&schema.GoogleSheetRow{}).Where("sheet_id = ?", s.ID).Count(&count)
	if count != 4 {
		t.Fatalf("rows = %d, want 4", count)
	}
	if got, _ := exports.Position(ctx, s.ID, entryID, "whatsapp"); got.MessageID != "m4" {
		t.Fatalf("cursor = %+v, want the furthest export", got)
	}
}
