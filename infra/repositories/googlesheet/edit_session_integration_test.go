package googlesheet_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/infra/database"
)

func editDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := integrationDB(t)
	for _, name := range []string{"idx_google_sheet_edit_sessions_file", "idx_google_sheet_edit_sessions_expiry", "ux_google_sheet_edit_sessions_open"} {
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

func openSession(t *testing.T, repo googlesheet.EditSessionRepository, workspaceID, spreadsheetID, userID string, at time.Time) *googlesheet.EditSession {
	t.Helper()
	session, err := googlesheet.NewEditSession(workspaceID, spreadsheetID, userID, at)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Start(context.Background(), session); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return session
}

func TestOnlyOneEditSessionIsOpenPerFileAndTheSecondLearnsWhoHoldsIt(t *testing.T) {
	db := editDB(t)
	repo := NewEditSessionRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	first := openSession(t, repo, uuid.NewString(), "sp-a", "ana", now)
	if first.ID == "" {
		t.Fatal("Start did not give the session an id")
	}
	second, _ := googlesheet.NewEditSession(first.WorkspaceID, "sp-a", "bruno", now.Add(time.Second))
	err := repo.Start(ctx, second)
	var taken *googlesheet.EditTakenError
	if !errors.As(err, &taken) || taken.Session.ID != first.ID || taken.Session.StartedBy != "ana" || !taken.Session.StartedAt.Equal(now) {
		t.Fatalf("second start err = %v taken = %+v", err, taken)
	}
	openSession(t, repo, first.WorkspaceID, "sp-b", "bruno", now)
	openSession(t, repo, uuid.NewString(), "sp-a", "carla", now)
	current, found, err := repo.Current(ctx, first.WorkspaceID, "sp-a")
	if err != nil || !found || current.ID != first.ID {
		t.Fatalf("Current = %+v found=%v err=%v", current, found, err)
	}
	if _, found, err := repo.Current(ctx, first.WorkspaceID, "sp-none"); err != nil || found {
		t.Fatalf("Current of a free file found=%v err=%v", found, err)
	}
}

func TestExtendingIsRefusedOnceTheSessionEndedOrExpiredInTheDatabase(t *testing.T) {
	db := editDB(t)
	repo := NewEditSessionRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	session := openSession(t, repo, uuid.NewString(), "sp-a", "ana", now)
	beat := now.Add(30 * time.Second)
	if err := session.Extend(beat); err != nil {
		t.Fatal(err)
	}
	if err := repo.Extend(ctx, session, beat); err != nil {
		t.Fatalf("Extend: %v", err)
	}
	stored, err := repo.Find(ctx, session.WorkspaceID, session.ID)
	if err != nil || !stored.ExpiresAt.Equal(beat.Add(googlesheet.EditSessionTTL)) {
		t.Fatalf("stored = %+v err = %v", stored, err)
	}
	if err := repo.Extend(ctx, session, stored.ExpiresAt); !errors.Is(err, googlesheet.ErrEditSessionEnded) {
		t.Fatalf("extend at the expiry = %v", err)
	}
	if err := session.End(googlesheet.EditEnd{Reason: googlesheet.EditFinished, By: "ana", At: beat}); err != nil {
		t.Fatal(err)
	}
	if ended, err := repo.End(ctx, session); err != nil || !ended {
		t.Fatalf("End = %v %v", ended, err)
	}
	if err := repo.Extend(ctx, session, beat); !errors.Is(err, googlesheet.ErrEditSessionEnded) {
		t.Fatalf("extend after the end = %v", err)
	}
}

func TestEndingCountsTheRowsOfTheFileStillQueuedAndRunsOnce(t *testing.T) {
	db := editDB(t)
	sheets := NewSheetRepository(db)
	repo := NewEditSessionRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	pagamentos := storedSheet(t, sheets)
	negociacoes := storedSheet(t, sheets)
	negociacoes.WorkspaceID = pagamentos.WorkspaceID
	if err := db.Exec("UPDATE google_sheets SET workspace_id = ?, spreadsheet_id = ?, tab_id = 7 WHERE id = ?", pagamentos.WorkspaceID, pagamentos.SpreadsheetID, negociacoes.ID).Error; err != nil {
		t.Fatal(err)
	}
	negociacoes.SpreadsheetID = pagamentos.SpreadsheetID
	other := storedSheet(t, sheets)
	seededRow(t, db, pagamentos, googlesheet.RowPending, now)
	seededRow(t, db, pagamentos, googlesheet.RowInFlight, now)
	seededRow(t, db, negociacoes, googlesheet.RowPending, now)
	seededRow(t, db, pagamentos, googlesheet.RowWritten, now)
	seededRow(t, db, pagamentos, googlesheet.RowFailed, now)
	seededRow(t, db, other, googlesheet.RowPending, now)
	session := openSession(t, repo, pagamentos.WorkspaceID, pagamentos.SpreadsheetID, "ana", now)
	at := now.Add(time.Minute)
	if err := session.End(googlesheet.EditEnd{Reason: googlesheet.EditFinished, By: "ana", At: at}); err != nil {
		t.Fatal(err)
	}
	ended, err := repo.End(ctx, session)
	if err != nil || !ended || session.WaitedRows != 3 {
		t.Fatalf("End = %v %v waited = %d, want 3 queued rows of the file", ended, err, session.WaitedRows)
	}
	again, _ := googlesheet.NewEditSession(session.WorkspaceID, session.SpreadsheetID, "ana", now)
	again.ID = session.ID
	_ = again.End(googlesheet.EditEnd{Reason: googlesheet.EditLeft, By: "ana", At: at.Add(time.Second)})
	if ended, err := repo.End(ctx, again); err != nil || ended {
		t.Fatalf("second End = %v %v", ended, err)
	}
	stored, err := repo.Find(ctx, session.WorkspaceID, session.ID)
	if err != nil || stored.EndReason != googlesheet.EditFinished || !stored.EndedAt.Equal(at) || stored.EndedBy != "ana" || stored.WaitedRows != 3 {
		t.Fatalf("stored = %+v err = %v", stored, err)
	}
	if _, found, err := repo.Current(ctx, session.WorkspaceID, session.SpreadsheetID); err != nil || found {
		t.Fatalf("the ended session is still current: found=%v err=%v", found, err)
	}
	openSession(t, repo, session.WorkspaceID, session.SpreadsheetID, "bruno", at)
}

func TestTheCheckAndTheSettleAreStoredAndReadBackScopedToTheWorkspace(t *testing.T) {
	db := editDB(t)
	repo := NewEditSessionRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	session := openSession(t, repo, uuid.NewString(), "sp-a", "ana", now)
	session.Settle(now.Add(time.Second))
	if err := repo.Settle(ctx, session); err != nil {
		t.Fatal(err)
	}
	session.Check = googlesheet.NewEditCheck([]googlesheet.TabCheck{
		{SheetID: "s1", TabID: 3, TabName: "Pagamentos", Reason: googlesheet.ReasonHeaderMismatch, Detail: "coluna C"},
		{SheetID: "s2", TabID: 4, TabName: "Pagos", RenamedFrom: "Negociações", RefWarnings: []int{4, 9}},
	})
	if err := repo.SaveCheck(ctx, session); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.Find(ctx, session.WorkspaceID, session.ID)
	if err != nil || !stored.Settled() || !stored.SettledAt.Equal(now.Add(time.Second)) {
		t.Fatalf("stored = %+v err = %v", stored, err)
	}
	if stored.Check.Status != googlesheet.CheckBroken || len(stored.Check.Tabs) != 2 || stored.Check.Tabs[1].RenamedFrom != "Negociações" || stored.Check.Tabs[1].RefWarnings[1] != 9 {
		t.Fatalf("check = %+v", stored.Check)
	}
	if _, err := repo.Find(ctx, uuid.NewString(), session.ID); !errors.Is(err, googlesheet.ErrEditSessionNotFound) {
		t.Fatalf("cross-workspace find = %v", err)
	}
	if _, err := repo.Find(ctx, session.WorkspaceID, "not-a-uuid"); !errors.Is(err, googlesheet.ErrEditSessionNotFound) {
		t.Fatalf("malformed id = %v", err)
	}
}

func TestExpiredListsOnlyOpenSessionsPastTheirLease(t *testing.T) {
	db := editDB(t)
	repo := NewEditSessionRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	workspace := uuid.NewString()
	stale := openSession(t, repo, workspace, "sp-stale", "ana", now.Add(-time.Hour))
	openSession(t, repo, workspace, "sp-live", "ana", now)
	closed := openSession(t, repo, workspace, "sp-closed", "ana", now.Add(-time.Hour))
	_ = closed.End(googlesheet.EditEnd{Reason: googlesheet.EditFinished, By: "ana", At: now.Add(-50 * time.Minute)})
	if _, err := repo.End(ctx, closed); err != nil {
		t.Fatal(err)
	}
	expired, err := repo.Expired(ctx, now, 10)
	if err != nil || len(expired) != 1 || expired[0].ID != stale.ID {
		t.Fatalf("Expired = %+v err = %v", expired, err)
	}
}

func TestHistoryListsTheFileSessionsNewestFirst(t *testing.T) {
	db := editDB(t)
	repo := NewEditSessionRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	workspace := uuid.NewString()
	var ids []string
	for i := range 3 {
		s := openSession(t, repo, workspace, "sp-a", "ana", now.Add(time.Duration(i)*time.Hour))
		_ = s.End(googlesheet.EditEnd{Reason: googlesheet.EditFinished, By: "ana", At: s.StartedAt.Add(time.Minute)})
		if _, err := repo.End(ctx, s); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, s.ID)
	}
	openSession(t, repo, workspace, "sp-b", "ana", now)
	page, err := repo.History(ctx, workspace, "sp-a", shared.Pagination{Page: 1, PageSize: 2})
	if err != nil || page.TotalItems != 3 || len(page.Items) != 2 || page.Items[0].ID != ids[2] || page.Items[1].ID != ids[1] {
		t.Fatalf("History = %+v err = %v", page, err)
	}
}
