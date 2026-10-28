package googlesheet

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
)

func (w *world) startEdit(t *testing.T, user string) *EditView {
	t.Helper()
	view, err := w.sessions.Start(context.Background(), StartEditInput{WorkspaceID: testWorkspace, SpreadsheetID: w.sheet.SpreadsheetID, UserID: user})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return view
}

func (w *world) endEdit(t *testing.T, id, user string, reason googlesheet.EditEndReason) *EditView {
	t.Helper()
	view, err := w.sessions.End(context.Background(), EndEditInput{WorkspaceID: testWorkspace, SessionID: id, UserID: user, Reason: reason})
	if err != nil {
		t.Fatalf("End: %v", err)
	}
	return view
}

func (w *world) secondTab(t *testing.T) *googlesheet.Sheet {
	t.Helper()
	tab, err := googlesheet.NewSheet(googlesheet.NewSheetInput{
		WorkspaceID: testWorkspace, AccountID: "acct-1", Title: "Pagamentos", TabName: "Pagamentos",
		Columns: []googlesheet.ColumnInput{{Header: "Cliente", Type: googlesheet.TypeText, Required: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tab.SpreadsheetID, tab.TabID = w.sheet.SpreadsheetID, 5
	if err := w.sheets.Create(context.Background(), tab); err != nil {
		t.Fatal(err)
	}
	w.api.mu.Lock()
	w.api.headers[5] = toAny(tab.HeaderRow())
	w.api.workbook.Tabs = append(w.api.workbook.Tabs, googlesheet.TabInfo{ID: 5, Title: "Pagamentos", Rows: 100, Columns: 2})
	w.api.mu.Unlock()
	return tab
}

func (w *world) pending(t *testing.T) int {
	t.Helper()
	health, err := w.rows.Health(context.Background(), w.sheet.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	return health.Pending
}

func TestWhileAPersonEditsTheFileByHandRowsQueueAndNothingReachesGoogle(t *testing.T) {
	w := newWorld(t)
	w.people.names["ana"] = "Ana Souza"
	view := w.startEdit(t, "ana")
	if !view.Session.Settled() || view.StartedBy == nil || view.StartedBy.Name != "Ana Souza" {
		t.Fatalf("view = %+v", view)
	}
	w.enqueue(t, "k1", map[string]any{"nome": "Bia"})
	w.enqueue(t, "k2", map[string]any{"nome": "Caio"})
	w.write(t)
	if len(w.api.appends) != 0 || w.pending(t) != 2 {
		t.Fatalf("appends = %d pending = %d; rows must wait for the edit to end", len(w.api.appends), w.pending(t))
	}
	health, err := w.manager.Health(context.Background(), testWorkspace, w.sheet.ID)
	if err != nil || health.Editing == nil || health.Editing.Session.ID != view.Session.ID || health.Editing.StartedBy.Name != "Ana Souza" {
		t.Fatalf("health editing = %+v err = %v", health.Editing, err)
	}
}

func TestFinishingChecksTheTabsAndTheQueuedRowsGoRightBelowTheLastVozkoRowLeavingManualFormulasAlone(t *testing.T) {
	w := newWorld(t)
	first := w.enqueue(t, "k0", map[string]any{"nome": "Ana"}).RowID
	w.write(t)
	view := w.startEdit(t, "ana")
	w.api.addRows(0, []any{"", "Total", "=SUM(C2:C3)"})
	queued := w.enqueue(t, "k1", map[string]any{"nome": "Bia"}).RowID
	w.write(t)
	ended := w.endEdit(t, view.Session.ID, "ana", googlesheet.EditFinished)
	if ended.Session.EndReason != googlesheet.EditFinished || ended.Session.WaitedRows != 1 || ended.Session.Check.Status != googlesheet.CheckOK {
		t.Fatalf("ended = %+v", ended.Session)
	}
	if len(w.nudger.sheets) == 0 || w.nudger.sheets[len(w.nudger.sheets)-1] != w.sheet.ID {
		t.Fatalf("nudges = %v; the writer must be woken for the queued rows", w.nudger.sheets)
	}
	w.write(t)
	grid := w.api.grid(0)
	if len(grid) != 4 || grid[1][0] != first || grid[2][0] != queued || grid[3][1] != "Total" || grid[3][2] != "=SUM(C2:C3)" {
		t.Fatalf("grid = %v; the queued row goes under the last Vozko row and the manual formula row moves down untouched", grid)
	}
}

func TestFinishingAfterTheHeaderWasChangedPausesTheTabAndSaysWhatBroke(t *testing.T) {
	w := newWorld(t)
	view := w.startEdit(t, "ana")
	w.api.setHeader(0, []any{googlesheet.RefHeader, "Nome", "Preço", "Telefone"})
	w.enqueue(t, "k1", map[string]any{"nome": "Bia"})
	nudgesBefore := len(w.nudger.sheets)
	ended := w.endEdit(t, view.Session.ID, "ana", googlesheet.EditFinished)
	check := ended.Session.Check
	if check.Status != googlesheet.CheckBroken || len(check.Tabs) != 1 || check.Tabs[0].Reason != googlesheet.ReasonHeaderMismatch || !check.Tabs[0].Repairable() || check.Tabs[0].Detail == "" {
		t.Fatalf("check = %+v", check)
	}
	if stored := w.stored(t); stored.Status != googlesheet.StatusNeedsAttention || stored.Reason != googlesheet.ReasonHeaderMismatch {
		t.Fatalf("sheet = %s %s", stored.Status, stored.Reason)
	}
	if len(w.alerts.attention) != 1 {
		t.Fatalf("alerts = %v; the others hear about it through the episode", w.alerts.attention)
	}
	if len(w.nudger.sheets) != nudgesBefore {
		t.Fatal("a broken tab was nudged")
	}
	w.write(t)
	if len(w.api.appends) != 0 {
		t.Fatal("rows were written into a broken tab")
	}
}

func TestARenamedTabIsFollowedAndADeletedOneIsReportedGone(t *testing.T) {
	w := newWorld(t)
	gone := w.secondTab(t)
	view := w.startEdit(t, "ana")
	w.api.mu.Lock()
	w.api.workbook.Tabs = []googlesheet.TabInfo{{ID: 0, Title: "Pedidos de outubro", Rows: 1000, Columns: 10}}
	w.api.mu.Unlock()
	check := w.endEdit(t, view.Session.ID, "ana", googlesheet.EditFinished).Session.Check
	byID := map[string]googlesheet.TabCheck{}
	for _, tab := range check.Tabs {
		byID[tab.SheetID] = tab
	}
	renamed := byID[w.sheet.ID]
	if renamed.Reason != "" || renamed.RenamedFrom != "Pedidos" || renamed.TabName != "Pedidos de outubro" || w.stored(t).TabName != "Pedidos de outubro" {
		t.Fatalf("renamed = %+v stored = %q", renamed, w.stored(t).TabName)
	}
	if byID[gone.ID].Reason != googlesheet.ReasonSpreadsheetGone || check.Status != googlesheet.CheckBroken {
		t.Fatalf("gone = %+v status = %s", byID[gone.ID], check.Status)
	}
}

func TestRefCellsThatAreNotVozkoRefsAreReportedWithoutPausingTheTab(t *testing.T) {
	w := newWorld(t)
	w.enqueue(t, "k0", map[string]any{"nome": "Ana"})
	w.write(t)
	view := w.startEdit(t, "ana")
	w.api.addRows(0, []any{"anotação", "linha digitada"})
	check := w.endEdit(t, view.Session.ID, "ana", googlesheet.EditFinished).Session.Check
	if check.Status != googlesheet.CheckOK || len(check.Tabs[0].RefWarnings) != 1 || check.Tabs[0].RefWarnings[0] != 3 {
		t.Fatalf("check = %+v", check)
	}
	if w.stored(t).Status != googlesheet.StatusActive {
		t.Fatal("a ref warning paused the tab")
	}
}

func TestWhenGoogleCannotBeReadTheLockIsStillReleasedAndTheCheckSaysUnchecked(t *testing.T) {
	w := newWorld(t)
	view := w.startEdit(t, "ana")
	w.api.readErr = errTransient
	ended := w.endEdit(t, view.Session.ID, "ana", googlesheet.EditFinished)
	if !ended.Session.Ended() || ended.Session.Check.Status != googlesheet.CheckUnchecked {
		t.Fatalf("session = %+v", ended.Session)
	}
	if _, found, _ := w.edits.Current(context.Background(), testWorkspace, w.sheet.SpreadsheetID); found {
		t.Fatal("the lock stayed taken because Google was unreadable")
	}
}

func TestOnlyOnePersonEditsAFileAndTheOtherLearnsWhoAndSince(t *testing.T) {
	w := newWorld(t)
	w.secondTab(t)
	w.people.names["ana"] = "Ana Souza"
	w.startEdit(t, "ana")
	_, err := w.sessions.Start(context.Background(), StartEditInput{WorkspaceID: testWorkspace, SpreadsheetID: w.sheet.SpreadsheetID, UserID: "bruno"})
	var taken *googlesheet.EditTakenError
	if !errors.As(err, &taken) || taken.Session.StartedBy != "ana" || taken.HolderName != "Ana Souza" || !taken.Session.StartedAt.Equal(w.now) {
		t.Fatalf("err = %v taken = %+v", err, taken)
	}
	if _, err := w.sessions.Start(context.Background(), StartEditInput{WorkspaceID: testWorkspace, SpreadsheetID: "sp-other", UserID: "bruno"}); !errors.Is(err, googlesheet.ErrSpreadsheetNotFound) {
		t.Fatalf("a file of no sheet of the workspace = %v", err)
	}
}

func TestStartingWaitsForTheBatchInFlightAndTheHeartbeatSettlesItLater(t *testing.T) {
	w := newWorld(t)
	release, busy, err := acquireSheet(context.Background(), w.leases, w.sheet.ID)
	if err != nil || busy {
		t.Fatalf("lease: busy=%v err=%v", busy, err)
	}
	view := w.startEdit(t, "ana")
	if view.Session.Settled() {
		t.Fatal("the session settled while a write batch still held the tab")
	}
	release()
	w.now = w.now.Add(10 * time.Second)
	beat, err := w.sessions.Heartbeat(context.Background(), EditInput{WorkspaceID: testWorkspace, SessionID: view.Session.ID, UserID: "ana"})
	if err != nil || !beat.Session.Settled() || !beat.Session.ExpiresAt.Equal(w.now.Add(googlesheet.EditSessionTTL)) {
		t.Fatalf("beat = %+v err = %v", beat, err)
	}
}

func TestAHeartbeatAfterTheLeaseRanOutEndsTheSessionAsExpiredWithItsCheck(t *testing.T) {
	w := newWorld(t)
	view := w.startEdit(t, "ana")
	w.now = w.now.Add(googlesheet.EditSessionTTL + time.Second)
	beat, err := w.sessions.Heartbeat(context.Background(), EditInput{WorkspaceID: testWorkspace, SessionID: view.Session.ID, UserID: "ana"})
	if err != nil || beat.Session.EndReason != googlesheet.EditExpired || !beat.Session.Check.Done() {
		t.Fatalf("beat = %+v err = %v", beat, err)
	}
}

func TestOnlyTheHolderBeatsAndFinishesAndAnAdminEndsItForThem(t *testing.T) {
	w := newWorld(t)
	w.people.names["bruno"] = "Bruno Lima"
	view := w.startEdit(t, "ana")
	if _, err := w.sessions.Heartbeat(context.Background(), EditInput{WorkspaceID: testWorkspace, SessionID: view.Session.ID, UserID: "bruno"}); !errors.Is(err, googlesheet.ErrEditSessionNotYours) {
		t.Fatalf("someone else's heartbeat = %v", err)
	}
	if _, err := w.sessions.End(context.Background(), EndEditInput{WorkspaceID: testWorkspace, SessionID: view.Session.ID, UserID: "bruno", Reason: googlesheet.EditFinished}); !errors.Is(err, googlesheet.ErrEditSessionNotYours) {
		t.Fatalf("someone else's finish = %v", err)
	}
	ended := w.endEdit(t, view.Session.ID, "bruno", googlesheet.EditEndedByAdmin)
	if ended.Session.EndReason != googlesheet.EditEndedByAdmin || ended.EndedBy == nil || ended.EndedBy.Name != "Bruno Lima" {
		t.Fatalf("ended = %+v by %+v", ended.Session, ended.EndedBy)
	}
	if _, err := w.sessions.End(context.Background(), EndEditInput{WorkspaceID: "ws-2", SessionID: view.Session.ID, UserID: "ana", Reason: googlesheet.EditFinished}); !errors.Is(err, googlesheet.ErrEditSessionNotFound) {
		t.Fatalf("another workspace = %v", err)
	}
}

func TestEndingTwiceAnswersTheStoredEndAndChecksOnce(t *testing.T) {
	w := newWorld(t)
	view := w.startEdit(t, "ana")
	w.endEdit(t, view.Session.ID, "ana", googlesheet.EditFinished)
	calls := w.api.workbookCalls
	again := w.endEdit(t, view.Session.ID, "ana", googlesheet.EditLeft)
	if again.Session.EndReason != googlesheet.EditFinished || w.api.workbookCalls != calls || w.edits.ends != 1 {
		t.Fatalf("again = %+v calls %d -> %d ends = %d", again.Session, calls, w.api.workbookCalls, w.edits.ends)
	}
}

func TestTheExpiryJobEndsAbandonedSessionsAndChecksThem(t *testing.T) {
	w := newWorld(t)
	view := w.startEdit(t, "ana")
	if err := w.sessions.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stored, _ := w.edits.Find(context.Background(), testWorkspace, view.Session.ID); stored.Ended() {
		t.Fatal("a live session was expired")
	}
	w.now = w.now.Add(googlesheet.EditSessionTTL)
	if err := w.sessions.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored, _ := w.edits.Find(context.Background(), testWorkspace, view.Session.ID)
	if stored.EndReason != googlesheet.EditExpired || stored.Check.Status != googlesheet.CheckOK {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestStartingOverAnExpiredSessionEndsItFirst(t *testing.T) {
	w := newWorld(t)
	old := w.startEdit(t, "ana")
	w.now = w.now.Add(googlesheet.EditSessionTTL + time.Minute)
	fresh := w.startEdit(t, "bruno")
	stored, _ := w.edits.Find(context.Background(), testWorkspace, old.Session.ID)
	if stored.EndReason != googlesheet.EditExpired || !stored.Check.Done() || fresh.Session.StartedBy != "bruno" {
		t.Fatalf("old = %+v fresh = %+v", stored, fresh.Session)
	}
}

func TestTheWriterWritesNothingWhenTheLockCannotBeRead(t *testing.T) {
	w := newWorld(t)
	w.enqueue(t, "k1", map[string]any{"nome": "Bia"})
	w.edits.failCurrent = errors.New("database down")
	if err := w.writer.WriteSheet(context.Background(), w.sheet.ID); err == nil {
		t.Fatal("an unreadable lock was treated as free")
	}
	if len(w.api.appends) != 0 {
		t.Fatal("rows were written without knowing whether someone was editing")
	}
}

func TestTheWriterStopsAtTheNextBatchOnceAnEditStarts(t *testing.T) {
	w := newWorld(t)
	for _, key := range []string{"a", "b", "c", "d", "e"} {
		w.enqueue(t, key, map[string]any{"nome": key})
	}
	started := false
	w.api.onInsert = func() {
		if started {
			return
		}
		started = true
		session, _ := googlesheet.NewEditSession(testWorkspace, w.sheet.SpreadsheetID, "ana", w.now)
		if err := w.edits.Start(context.Background(), session); err != nil {
			t.Error(err)
		}
	}
	w.write(t)
	if len(w.api.appends) != 1 || len(w.api.appends[0].refs) != 3 || w.pending(t) != 2 {
		t.Fatalf("appends = %d pending = %d; the batch in flight finishes and the next one waits", len(w.api.appends), w.pending(t))
	}
}

func TestVozkoChangesNothingInGoogleWhileTheFileIsEdited(t *testing.T) {
	r := newRemovalWorld(t)
	rows := r.writtenRows(t, "Ana")
	r.startEdit(t, "ana")
	appliedBefore := r.api.applied
	_, err := r.manager.EditColumns(context.Background(), ColumnsInput{WorkspaceID: testWorkspace, ID: r.sheet.ID, Version: r.sheet.Version, Edit: googlesheet.ColumnsEdit{Add: []googlesheet.ColumnInput{{Header: "Cidade", Type: googlesheet.TypeText}}}})
	if !errors.Is(err, googlesheet.ErrFileBeingEdited) || r.api.applied != appliedBefore {
		t.Fatalf("columns edit = %v applied %d -> %d", err, appliedBefore, r.api.applied)
	}
	if _, err := r.manager.Reactivate(context.Background(), ReactivateInput{WorkspaceID: testWorkspace, ID: r.sheet.ID, RepairHeader: true}); !errors.Is(err, googlesheet.ErrFileBeingEdited) {
		t.Fatalf("repair = %v", err)
	}
	if _, err := r.remover.Remove(context.Background(), r.input(rows[0])); !errors.Is(err, googlesheet.ErrFileBeingEdited) || len(r.api.deletedRows) != 0 {
		t.Fatalf("remove = %v deleted = %v", err, r.api.deletedRows)
	}
	_, err = r.manager.Create(context.Background(), CreateInput{WorkspaceID: testWorkspace, UserID: "ana", SpreadsheetID: r.sheet.SpreadsheetID, Title: "Leads", TabName: "Leads", Columns: []googlesheet.ColumnInput{{Header: "Nome", Type: googlesheet.TypeText}}})
	if !errors.Is(err, googlesheet.ErrFileBeingEdited) || len(r.api.added) != 0 {
		t.Fatalf("new tab = %v added = %v", err, r.api.added)
	}
}

func TestTheEditorViewCarriesTheHolderAndTheConnectedAccount(t *testing.T) {
	w := newWorld(t)
	free, err := w.sessions.Editor(context.Background(), testWorkspace, w.sheet.SpreadsheetID)
	if err != nil || free.Editing != nil || free.AccountEmail != "ops@example.com" {
		t.Fatalf("free = %+v err = %v", free, err)
	}
	view := w.startEdit(t, "ana")
	held, err := w.sessions.Editor(context.Background(), testWorkspace, w.sheet.SpreadsheetID)
	if err != nil || held.Editing == nil || held.Editing.Session.ID != view.Session.ID {
		t.Fatalf("held = %+v err = %v", held, err)
	}
	if _, err := w.sessions.Editor(context.Background(), testWorkspace, "sp-other"); !errors.Is(err, googlesheet.ErrSpreadsheetNotFound) {
		t.Fatalf("unknown file = %v", err)
	}
}

func TestHistoryListsTheFileSessionsWithNames(t *testing.T) {
	w := newWorld(t)
	w.people.names["ana"] = "Ana Souza"
	first := w.startEdit(t, "ana")
	w.endEdit(t, first.Session.ID, "ana", googlesheet.EditFinished)
	w.now = w.now.Add(time.Hour)
	w.startEdit(t, "ana")
	page, err := w.sessions.History(context.Background(), testWorkspace, w.sheet.SpreadsheetID, shared.Pagination{Page: 1, PageSize: 10})
	if err != nil || page.TotalItems != 2 || page.Items[1].Session.ID != first.Session.ID || page.Items[1].EndedBy.Name != "Ana Souza" || page.Items[0].Session.Ended() {
		t.Fatalf("page = %+v err = %v", page, err)
	}
}
