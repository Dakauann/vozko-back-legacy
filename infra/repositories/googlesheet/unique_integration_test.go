package googlesheet_repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"vozko/domain/googlesheet"
)

func countIn(t *testing.T, db *gorm.DB, table, sheetID string) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT count(*) FROM "+table+" WHERE sheet_id = ?", sheetID).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func rowsIn(t *testing.T, db *gorm.DB, sheetID string) int64 {
	return countIn(t, db, "google_sheet_rows", sheetID)
}

func claimsIn(t *testing.T, db *gorm.DB, sheetID string) int64 {
	return countIn(t, db, "google_sheet_unique_keys", sheetID)
}

func TestAUniqueKeyKeepsOneRowAndPointsRepeatsAtIt(t *testing.T) {
	db := integrationDB(t)
	sheets, rows := NewSheetRepository(db), NewRowRepository(db)
	ctx := context.Background()
	s := storedSheet(t, sheets)
	now := time.Now().UTC().Truncate(time.Millisecond)
	key := googlesheet.UniqueKey{Period: "2026-10", Hash: "a1"}

	first, err := rows.InsertUnique(ctx, pendingRow(t, s, "wf:run-1:node#0", now), key)
	if err != nil || first.Duplicate || first.RepeatOf != "" || first.Row == nil || first.Row.ID == "" {
		t.Fatalf("first = %+v, %v", first, err)
	}
	repeat, err := rows.InsertUnique(ctx, pendingRow(t, s, "wf:run-2:node#0", now), key)
	if err != nil || repeat.RepeatOf != first.Row.ID || repeat.Row != nil || repeat.Duplicate {
		t.Fatalf("repeat = %+v, %v", repeat, err)
	}
	replay, err := rows.InsertUnique(ctx, pendingRow(t, s, "wf:run-1:node#0", now), key)
	if err != nil || !replay.Duplicate || replay.RepeatOf != "" || replay.Row.ID != first.Row.ID {
		t.Fatalf("an idempotent replay must stay a duplicate, not a repeat: %+v, %v", replay, err)
	}
	nextMonth, err := rows.InsertUnique(ctx, pendingRow(t, s, "wf:run-3:node#0", now), googlesheet.UniqueKey{Period: "2026-11", Hash: "a1"})
	if err != nil || nextMonth.RepeatOf != "" || nextMonth.Row == nil {
		t.Fatalf("another period = %+v, %v", nextMonth, err)
	}
	plain, err := rows.InsertUnique(ctx, pendingRow(t, s, "manual:u:1", now), googlesheet.UniqueKey{})
	if err != nil || plain.Row == nil || plain.RepeatOf != "" {
		t.Fatalf("a sheet without a rule = %+v, %v", plain, err)
	}
	if n := rowsIn(t, db, s.ID); n != 3 {
		t.Fatalf("rows = %d, want 3", n)
	}
	if n := claimsIn(t, db, s.ID); n != 2 {
		t.Fatalf("claims = %d, want 2", n)
	}
}

func TestConcurrentWritersOfTheSameRecordNeverBothPass(t *testing.T) {
	db := integrationDB(t)
	sheets, rows := NewSheetRepository(db), NewRowRepository(db)
	ctx := context.Background()
	s := storedSheet(t, sheets)
	now := time.Now().UTC().Truncate(time.Millisecond)
	key := googlesheet.UniqueKey{Period: "", Hash: "same-payment"}

	const writers = 16
	results := make([]googlesheet.Inserted, writers)
	errs := make([]error, writers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = rows.InsertUnique(ctx, pendingRow(t, s, fmt.Sprintf("ai:conv-1:call-%d", i), now), key)
		}(i)
	}
	close(start)
	wg.Wait()
	winner := ""
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("writer %d: %v", i, errs[i])
		}
		if results[i].Row != nil {
			if winner != "" {
				t.Fatalf("two writers recorded the same payment: %s and %s", winner, results[i].Row.ID)
			}
			winner = results[i].Row.ID
		}
	}
	for i := range results {
		if results[i].Row == nil && results[i].RepeatOf != winner {
			t.Fatalf("writer %d repeat points at %q, want %q", i, results[i].RepeatOf, winner)
		}
	}
	if winner == "" || rowsIn(t, db, s.ID) != 1 || claimsIn(t, db, s.ID) != 1 {
		t.Fatalf("winner %q, rows %d, claims %d", winner, rowsIn(t, db, s.ID), claimsIn(t, db, s.ID))
	}
}

func TestClaimsOutliveThePurgeOfWrittenRows(t *testing.T) {
	db := integrationDB(t)
	sheets, rows := NewSheetRepository(db), NewRowRepository(db)
	ctx := context.Background()
	s := storedSheet(t, sheets)
	old := time.Now().UTC().Add(-40 * 24 * time.Hour).Truncate(time.Millisecond)
	key := googlesheet.UniqueKey{Hash: "matricula-123"}

	first, err := rows.InsertUnique(ctx, pendingRow(t, s, "k1", old), key)
	if err != nil {
		t.Fatal(err)
	}
	if err := rows.MarkWritten(ctx, []googlesheet.WrittenRow{{ID: first.Row.ID, RowNumber: 2}}, old); err != nil {
		t.Fatal(err)
	}
	if purged, err := rows.PurgeWritten(ctx, time.Now().UTC().Add(-30*24*time.Hour), 100); err != nil || purged != 1 {
		t.Fatalf("PurgeWritten = %d, %v", purged, err)
	}
	again, err := rows.InsertUnique(ctx, pendingRow(t, s, "k2", time.Now().UTC()), key)
	if err != nil || again.RepeatOf != first.Row.ID {
		t.Fatalf("after the purge = %+v, %v", again, err)
	}
}

func TestDroppingUnwrittenRowsReleasesOnlyTheirClaims(t *testing.T) {
	db := integrationDB(t)
	sheets, rows := NewSheetRepository(db), NewRowRepository(db)
	ctx := context.Background()
	s := storedSheet(t, sheets)
	now := time.Now().UTC().Truncate(time.Millisecond)
	written, err := rows.InsertUnique(ctx, pendingRow(t, s, "k1", now), googlesheet.UniqueKey{Hash: "written"})
	if err != nil {
		t.Fatal(err)
	}
	if err := rows.MarkWritten(ctx, []googlesheet.WrittenRow{{ID: written.Row.ID, RowNumber: 2}}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := rows.InsertUnique(ctx, pendingRow(t, s, "k2", now), googlesheet.UniqueKey{Hash: "queued"}); err != nil {
		t.Fatal(err)
	}
	if err := rows.DeleteUnwritten(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	queued, err := rows.InsertUnique(ctx, pendingRow(t, s, "k3", now), googlesheet.UniqueKey{Hash: "queued"})
	if err != nil || queued.Row == nil {
		t.Fatalf("a dropped row must not block its record: %+v, %v", queued, err)
	}
	kept, err := rows.InsertUnique(ctx, pendingRow(t, s, "k4", now), googlesheet.UniqueKey{Hash: "written"})
	if err != nil || kept.RepeatOf != written.Row.ID {
		t.Fatalf("a written row keeps its claim: %+v, %v", kept, err)
	}
}

func TestLedgerRowsCanBeClaimedOldestFirst(t *testing.T) {
	db := integrationDB(t)
	sheets, rows := NewSheetRepository(db), NewRowRepository(db)
	ctx := context.Background()
	s := storedSheet(t, sheets)
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	var stored []*googlesheet.Row
	for i := 0; i < 3; i++ {
		row, _, err := rows.Insert(ctx, pendingRow(t, s, fmt.Sprintf("k%d", i), base.Add(time.Duration(i)*time.Minute)))
		if err != nil {
			t.Fatal(err)
		}
		stored = append(stored, row)
	}
	page, err := rows.LedgerPage(ctx, s.ID, googlesheet.LedgerCursor{}, 2)
	if err != nil || len(page) != 2 || page[0].ID != stored[0].ID || page[1].ID != stored[1].ID || page[0].Cells["nome"] != "Ana k0" {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	rest, err := rows.LedgerPage(ctx, s.ID, page[1].LedgerCursor(), 2)
	if err != nil || len(rest) != 1 || rest[0].ID != stored[2].ID {
		t.Fatalf("second page = %+v, %v", rest, err)
	}
	key := googlesheet.UniqueKey{Period: "2026-10", Hash: "chat-1"}
	claimed, err := rows.ClaimUnique(ctx, []googlesheet.UniqueClaim{{Row: stored[0], Key: key}, {Row: stored[2], Key: key}})
	if err != nil || claimed != 1 {
		t.Fatalf("ClaimUnique = %d, %v", claimed, err)
	}
	if again, err := rows.ClaimUnique(ctx, []googlesheet.UniqueClaim{{Row: stored[1], Key: key}}); err != nil || again != 0 {
		t.Fatalf("claiming a taken key again = %d, %v", again, err)
	}
	repeat, err := rows.InsertUnique(ctx, pendingRow(t, s, "k9", base), key)
	if err != nil || repeat.RepeatOf != stored[0].ID {
		t.Fatalf("the oldest ledger row must win: %+v, %v", repeat, err)
	}
}

func TestTheUniqueRuleRoundTripsAndOldTabsReadAsNone(t *testing.T) {
	db := integrationDB(t)
	repo := NewSheetRepository(db)
	ctx := context.Background()
	s := storedSheet(t, repo)
	got, err := repo.Get(ctx, s.ID)
	if err != nil || got.Unique.Mode != googlesheet.UniqueNone || got.Unique.OnRepeat != googlesheet.RepeatSkip {
		t.Fatalf("new tab = %+v, %v", got, err)
	}
	if err := db.Exec("UPDATE google_sheets SET unique_rule = '{}'::jsonb WHERE id = ?", s.ID).Error; err != nil {
		t.Fatal(err)
	}
	if old, err := repo.Get(ctx, s.ID); err != nil || old.Unique.Mode != googlesheet.UniqueNone {
		t.Fatalf("a tab from before the rule = %+v, %v", old, err)
	}
	if err := got.ChangeUnique(googlesheet.UniqueRule{Mode: googlesheet.UniqueColumns, Columns: []string{"nome"}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, got, 1); err != nil {
		t.Fatal(err)
	}
	saved, err := repo.Get(ctx, s.ID)
	if err != nil || saved.Unique.Mode != googlesheet.UniqueColumns || len(saved.Unique.Columns) != 1 || saved.Unique.Columns[0] != "nome" || saved.Version != 2 {
		t.Fatalf("saved = %+v, %v", saved.Unique, err)
	}
}
