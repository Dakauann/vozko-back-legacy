package googlesheet_repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/infra/crypto/piigorm"
	"vozko/infra/database"
	"vozko/infra/database/schema"
)

func removalDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := integrationDB(t)
	for _, name := range []string{"idx_google_sheet_row_removals_sheet", "idx_google_sheet_row_removals_lead", "idx_google_sheet_row_removals_entry", "ux_google_sheet_row_removals_row"} {
		stmt, ok := database.PerformanceIndexSQL(name)
		if !ok {
			t.Fatalf("index %s is not declared", name)
		}
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("index %s: %v", name, err)
		}
	}
	return db
}

func seededRow(t *testing.T, db *gorm.DB, s *googlesheet.Sheet, status googlesheet.RowStatus, at time.Time) *googlesheet.Row {
	t.Helper()
	cells, _ := json.Marshal(googlesheet.Cells{"nome": "Ana Souza"})
	lead := uuid.NewString()
	record := &schema.GoogleSheetRow{
		ID: uuid.NewString(), WorkspaceID: s.WorkspaceID, SheetID: s.ID, IdempotencyKey: "k-" + uuid.NewString(),
		Cells: piigorm.NewEncrypted(string(cells)), SheetVersion: s.Version, PeriodKey: "2026-10", OriginKind: string(googlesheet.OriginWorkflow),
		WorkflowID: "wf-1", RunID: "run-1", LeadID: &lead, EntryID: uuid.NewString(), EntryType: "whatsapp",
		Status: string(status), NextAttemptAt: at, CreatedAt: at,
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatal(err)
	}
	row, err := toRowDomain(record)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func removalOf(t *testing.T, row *googlesheet.Row, kind googlesheet.RemovalKind, number int, at time.Time) *googlesheet.RowRemoval {
	t.Helper()
	removal, err := googlesheet.NewRowRemoval(row, kind, "user-1", "lançado em duplicidade por Ana", number, at)
	if err != nil {
		t.Fatal(err)
	}
	return removal
}

func ledgerHas(t *testing.T, db *gorm.DB, id string) bool {
	t.Helper()
	var n int64
	if err := db.Model(&schema.GoogleSheetRow{}).Where("id = ?", id).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestFindRowIsScopedToTheWorkspaceAndTheSheet(t *testing.T) {
	db := removalDB(t)
	repo := NewRowRemovalRepository(db)
	s := storedSheet(t, NewSheetRepository(db))
	row := seededRow(t, db, s, googlesheet.RowWritten, time.Now().UTC())
	ctx := context.Background()

	found, err := repo.FindRow(ctx, s.WorkspaceID, s.ID, row.ID)
	if err != nil || found.ID != row.ID || found.Cells["nome"] != "Ana Souza" || found.Status != googlesheet.RowWritten {
		t.Fatalf("FindRow = %+v, %v", found, err)
	}
	for name, ids := range map[string][3]string{
		"other workspace": {uuid.NewString(), s.ID, row.ID},
		"other sheet":     {s.WorkspaceID, uuid.NewString(), row.ID},
		"unknown row":     {s.WorkspaceID, s.ID, uuid.NewString()},
	} {
		if _, err := repo.FindRow(ctx, ids[0], ids[1], ids[2]); !errors.Is(err, googlesheet.ErrRowNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCancelQueuedDeletesTheRowAndRecordsTheRemovalTogether(t *testing.T) {
	db := removalDB(t)
	repo := NewRowRemovalRepository(db)
	s := storedSheet(t, NewSheetRepository(db))
	now := time.Now().UTC().Truncate(time.Millisecond)
	ctx := context.Background()
	for _, status := range googlesheet.QueuedStatuses() {
		row := seededRow(t, db, s, status, now)
		removal := removalOf(t, row, googlesheet.RemovalCancel, 0, now)
		if err := repo.CancelQueued(ctx, removal); err != nil {
			t.Fatalf("%s: %v", status, err)
		}
		if ledgerHas(t, db, row.ID) || removal.ID == "" {
			t.Fatalf("%s: the row stayed or the removal has no id", status)
		}
	}
	page, err := repo.ListRemovals(ctx, s.WorkspaceID, s.ID, shared.Pagination{Page: 1, PageSize: 10})
	if err != nil || len(page.Items) != 3 {
		t.Fatalf("ListRemovals = %+v, %v", page, err)
	}
}

func TestCancelQueuedRefusesARowTheWriterClaimedAndLeavesNoRecord(t *testing.T) {
	db := removalDB(t)
	repo := NewRowRemovalRepository(db)
	s := storedSheet(t, NewSheetRepository(db))
	now := time.Now().UTC()
	queued := seededRow(t, db, s, googlesheet.RowPending, now)
	removal := removalOf(t, queued, googlesheet.RemovalCancel, 0, now)
	if err := db.Model(&schema.GoogleSheetRow{}).Where("id = ?", queued.ID).Update("status", string(googlesheet.RowInFlight)).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.CancelQueued(context.Background(), removal); !errors.Is(err, googlesheet.ErrRowStatusChanged) {
		t.Fatalf("err = %v", err)
	}
	var records int64
	db.Model(&schema.GoogleSheetRowRemoval{}).Count(&records)
	if !ledgerHas(t, db, queued.ID) || records != 0 {
		t.Fatalf("ledger kept %v, removals %d", ledgerHas(t, db, queued.ID), records)
	}
}

func TestRecordDeletedKeepsTheSnapshotEncryptedAndReadable(t *testing.T) {
	db := removalDB(t)
	repo := NewRowRemovalRepository(db)
	s := storedSheet(t, NewSheetRepository(db))
	now := time.Now().UTC().Truncate(time.Millisecond)
	ctx := context.Background()
	row := seededRow(t, db, s, googlesheet.RowWritten, now)
	removal := removalOf(t, row, googlesheet.RemovalDelete, 5, now)
	if err := repo.RecordDeleted(ctx, removal); err != nil {
		t.Fatal(err)
	}
	if ledgerHas(t, db, row.ID) {
		t.Fatal("the written row stayed in the ledger")
	}
	var raw struct {
		Cells  []byte
		Reason []byte
	}
	if err := db.Raw("SELECT cells, reason FROM google_sheet_row_removals WHERE row_id = ?", row.ID).Row().Scan(&raw.Cells, &raw.Reason); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw.Cells, []byte("Ana")) || bytes.Contains(raw.Reason, []byte("Ana")) {
		t.Fatal("the snapshot or the reason is stored in plaintext")
	}
	page, err := repo.ListRemovals(ctx, s.WorkspaceID, s.ID, shared.Pagination{Page: 1, PageSize: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("ListRemovals = %+v, %v", page, err)
	}
	got := page.Items[0]
	if got.RowID != row.ID || got.Kind != googlesheet.RemovalDelete || got.RowNumber != 5 || got.Reason != removal.Reason || got.RemovedBy != "user-1" ||
		got.Cells["nome"] != "Ana Souza" || got.Origin != row.Origin || got.PeriodKey != "2026-10" || !got.RemovedAt.Equal(now) {
		t.Fatalf("listed = %+v, want %+v", got, removal)
	}
	if err := repo.RecordDeleted(ctx, removalOf(t, row, googlesheet.RemovalDelete, 5, now)); err == nil {
		t.Fatal("a row was recorded as removed twice")
	}
}

func TestRemovalsListNewestFirstAndOnlyForTheirSheet(t *testing.T) {
	db := removalDB(t)
	repo := NewRowRemovalRepository(db)
	sheets := NewSheetRepository(db)
	s, other := storedSheet(t, sheets), storedSheet(t, sheets)
	base := time.Now().UTC().Truncate(time.Millisecond)
	ctx := context.Background()
	var ids []string
	for i := 0; i < 3; i++ {
		row := seededRow(t, db, s, googlesheet.RowPending, base)
		if err := repo.CancelQueued(ctx, removalOf(t, row, googlesheet.RemovalCancel, 0, base.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, row.ID)
	}
	if err := repo.CancelQueued(ctx, removalOf(t, seededRow(t, db, other, googlesheet.RowPending, base), googlesheet.RemovalCancel, 0, base)); err != nil {
		t.Fatal(err)
	}
	page, err := repo.ListRemovals(ctx, s.WorkspaceID, s.ID, shared.Pagination{Page: 1, PageSize: 2})
	if err != nil || page.TotalItems != 3 || len(page.Items) != 2 || page.Items[0].RowID != ids[2] || page.Items[1].RowID != ids[1] {
		t.Fatalf("page = %+v, %v", page, err)
	}
	if empty, err := repo.ListRemovals(ctx, uuid.NewString(), s.ID, shared.Pagination{Page: 1, PageSize: 2}); err != nil || empty.TotalItems != 0 {
		t.Fatalf("another workspace listed %+v, %v", empty, err)
	}
}

func claimFor(t *testing.T, db *gorm.DB, s *googlesheet.Sheet, rowID, hash string) {
	t.Helper()
	key := &schema.GoogleSheetUniqueKey{SheetID: s.ID, PeriodKey: "2026-10", KeyHash: hash, WorkspaceID: s.WorkspaceID, RowID: rowID, CreatedAt: time.Now().UTC()}
	if err := db.Create(key).Error; err != nil {
		t.Fatal(err)
	}
}

func claimedRows(t *testing.T, db *gorm.DB, s *googlesheet.Sheet) []string {
	t.Helper()
	var rows []string
	if err := db.Model(&schema.GoogleSheetUniqueKey{}).Where("sheet_id = ?", s.ID).Order("row_id").Pluck("row_id", &rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestCancellingOrRemovingFreesOnlyTheUniqueKeyOfThatRowInTheSameTransaction(t *testing.T) {
	db := removalDB(t)
	stmt, ok := database.PerformanceIndexSQL("idx_google_sheet_unique_keys_row")
	if !ok {
		t.Fatal("the unique key row index is not declared")
	}
	if err := db.Exec(stmt).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRowRemovalRepository(db)
	s := storedSheet(t, NewSheetRepository(db))
	now := time.Now().UTC()
	ctx := context.Background()
	queued, written, claimed, kept := seededRow(t, db, s, googlesheet.RowPending, now), seededRow(t, db, s, googlesheet.RowWritten, now), seededRow(t, db, s, googlesheet.RowPending, now), seededRow(t, db, s, googlesheet.RowWritten, now)
	for i, row := range []*googlesheet.Row{queued, written, claimed, kept} {
		claimFor(t, db, s, row.ID, fmt.Sprintf("%064d", i))
	}

	if err := repo.CancelQueued(ctx, removalOf(t, queued, googlesheet.RemovalCancel, 0, now)); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordDeleted(ctx, removalOf(t, written, googlesheet.RemovalDelete, 3, now)); err != nil {
		t.Fatal(err)
	}
	refused := removalOf(t, claimed, googlesheet.RemovalCancel, 0, now)
	if err := db.Model(&schema.GoogleSheetRow{}).Where("id = ?", claimed.ID).Update("status", string(googlesheet.RowInFlight)).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.CancelQueued(ctx, refused); !errors.Is(err, googlesheet.ErrRowStatusChanged) {
		t.Fatalf("err = %v", err)
	}
	got := claimedRows(t, db, s)
	want := []string{claimed.ID, kept.ID}
	if claimed.ID > kept.ID {
		want = []string{kept.ID, claimed.ID}
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("claims left = %v, want the refused and the untouched rows %v", got, want)
	}
}

func TestDroppingUnwrittenRowsStillFreesTheirUniqueKeys(t *testing.T) {
	db := removalDB(t)
	s := storedSheet(t, NewSheetRepository(db))
	now := time.Now().UTC()
	pending, written := seededRow(t, db, s, googlesheet.RowPending, now), seededRow(t, db, s, googlesheet.RowWritten, now)
	claimFor(t, db, s, pending.ID, fmt.Sprintf("%064d", 1))
	claimFor(t, db, s, written.ID, fmt.Sprintf("%064d", 2))
	if err := NewRowRepository(db).DeleteUnwritten(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if got := claimedRows(t, db, s); len(got) != 1 || got[0] != written.ID {
		t.Fatalf("claims left = %v, want only the written row", got)
	}
}
