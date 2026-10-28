package googlesheet

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"vozko/domain/googlesheet"
)

func (w *world) damageLikeTheAppendHeuristic(rowsAbove ...[]any) {
	w.api.mu.Lock()
	defer w.api.mu.Unlock()
	grid := append(append([][]any{}, rowsAbove...), w.api.grid(0)...)
	w.api.store(0, grid)
}

func (w *world) column() []any {
	w.api.mu.Lock()
	defer w.api.mu.Unlock()
	var out []any
	for _, row := range w.api.grid(0) {
		if len(row) == 0 {
			out = append(out, "")
			continue
		}
		out = append(out, row[0])
	}
	return out
}

func TestRowsLandUnderTheHeaderAndTheNextBatchRightBelowThem(t *testing.T) {
	w := newWorld(t)
	a := w.enqueue(t, "ka", map[string]any{"nome": "a"}).RowID
	w.write(t)
	b := w.enqueue(t, "kb", map[string]any{"nome": "b"}).RowID
	w.write(t)
	column := w.column()
	if len(column) != 3 || column[0] != googlesheet.RefHeader || column[1] != a || column[2] != b {
		t.Fatalf("column A = %v, want the header first and the rows under it in order", column)
	}
	if w.api.appends[0].at != 2 || w.api.appends[1].at != 3 {
		t.Fatalf("positions = %d, %d", w.api.appends[0].at, w.api.appends[1].at)
	}
}

func TestAHumanRowBelowTheLastWrittenRowIsPushedDownNotOverwritten(t *testing.T) {
	w := newWorld(t)
	w.api.addRows(0, []any{"", "linha digitada"})
	a := w.enqueue(t, "ka", map[string]any{"nome": "a"}).RowID
	w.write(t)
	grid := w.api.grid(0)
	if len(grid) != 3 || grid[1][0] != a || grid[2][1] != "linha digitada" {
		t.Fatalf("grid = %v", grid)
	}
}

func TestAHeaderPushedDownHoldsTheRowsWithItsOwnReason(t *testing.T) {
	w := newWorld(t)
	w.damageLikeTheAppendHeuristic([]any{"old-1", "Ana"})
	w.enqueue(t, "k1", map[string]any{"nome": "a"})
	w.write(t)
	if len(w.api.appends) != 0 {
		t.Fatalf("rows were written while the header is not in row 1: %+v", w.api.appends)
	}
	stored := w.stored(t)
	if stored.Reason != googlesheet.ReasonHeaderNotFirstRow || !strings.Contains(stored.ReasonDetail, "linha 2") {
		t.Fatalf("sheet = %+v", stored)
	}
	if len(w.alerts.attention) != 1 || len(w.rows.byStatus(w.sheet.ID, googlesheet.RowPending)) != 1 {
		t.Fatalf("alerts %v, the row must be kept", w.alerts.attention)
	}
}

func TestTheHeaderIsCheckedBeforeEveryBatchEvenInsideTheCheckWindow(t *testing.T) {
	w := newWorld(t)
	w.enqueue(t, "k1", map[string]any{"nome": "a"})
	w.write(t)
	w.damageLikeTheAppendHeuristic([]any{"someone", "inserted a row"})
	w.enqueue(t, "k2", map[string]any{"nome": "b"})
	w.write(t)
	if len(w.api.appends) != 1 {
		t.Fatalf("appends = %d, the second batch must be held", len(w.api.appends))
	}
	if s := w.stored(t); s.Status != googlesheet.StatusNeedsAttention {
		t.Fatalf("sheet = %+v", s)
	}
}

func TestARowGoogleAlreadyHasIsMarkedWrittenInsteadOfSentAgain(t *testing.T) {
	w := newWorld(t)
	a := w.enqueue(t, "ka", map[string]any{"nome": "a"}).RowID
	w.api.appendErrs = []error{errTransient}
	w.write(t)
	w.api.addRows(0, []any{a, "a"})
	w.now = w.now.Add(time.Hour)
	w.write(t)
	if got := w.api.written(); len(got) != 1 {
		t.Fatalf("google rows = %v, a reply lost after Google wrote the row must not duplicate it", got)
	}
	written := w.rows.byStatus(w.sheet.ID, googlesheet.RowWritten)
	if len(written) != 1 || written[0].RowNumber != 2 {
		t.Fatalf("written = %+v", written)
	}
}

func TestIsolatedRowsKeepTheirPositionsAfterARefusedOne(t *testing.T) {
	w := newWorld(t)
	var ids []string
	for _, name := range []string{"a", "b", "c"} {
		ids = append(ids, w.enqueue(t, "k"+name, map[string]any{"nome": name}).RowID)
	}
	w.api.rejectRef[ids[1]] = true
	w.write(t)
	rows := map[string]int{}
	for _, row := range w.rows.byStatus(w.sheet.ID, googlesheet.RowWritten) {
		rows[row.ID] = row.RowNumber
	}
	if rows[ids[0]] != 2 || rows[ids[2]] != 3 {
		t.Fatalf("row numbers = %v", rows)
	}
	if column := w.column(); len(column) != 3 || column[1] != ids[0] || column[2] != ids[2] {
		t.Fatalf("column A = %v", column)
	}
}

func TestConcurrentWritersNeverPickTheSameRows(t *testing.T) {
	w := newWorld(t)
	w.limiter.writes, w.limiter.reads = 1000, 1000
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		w.enqueue(t, "k"+name, map[string]any{"nome": name})
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.writer.WriteSheet(context.Background(), w.sheet.ID); err != nil {
				t.Errorf("WriteSheet: %v", err)
			}
		}()
	}
	wg.Wait()
	w.write(t)
	seen := map[int]bool{}
	for _, row := range w.rows.byStatus(w.sheet.ID, googlesheet.RowWritten) {
		if seen[row.RowNumber] {
			t.Fatalf("row %d picked twice", row.RowNumber)
		}
		seen[row.RowNumber] = true
	}
	column := w.column()
	if len(seen) != 7 || len(column) != 8 || column[0] != googlesheet.RefHeader {
		t.Fatalf("written %d, column A = %v", len(seen), column)
	}
}

func TestReactivateRepairsAHeaderPushedDownAndTheKeptRowsGoBelowTheMovedOnes(t *testing.T) {
	w := newWorld(t)
	w.damageLikeTheAppendHeuristic([]any{"old-2", "Bia"}, []any{"old-1", "Ana"})
	fresh := w.enqueue(t, "k1", map[string]any{"nome": "a"}).RowID
	w.write(t)
	_, err := w.manager.Reactivate(context.Background(), ReactivateInput{WorkspaceID: testWorkspace, ID: w.sheet.ID})
	if !errors.Is(err, googlesheet.ErrSheetNotActive) || w.stored(t).Reason != googlesheet.ReasonHeaderNotFirstRow {
		t.Fatalf("reactivate without repair = %v, sheet %+v", err, w.stored(t))
	}
	if _, err := w.manager.Reactivate(context.Background(), ReactivateInput{WorkspaceID: testWorkspace, ID: w.sheet.ID, RepairHeader: true}); err != nil {
		t.Fatalf("Reactivate: %v", err)
	}
	if len(w.api.repairs) != 1 || w.api.repairs[0] != (googlesheet.HeaderRepair{MoveFrom: 3}) {
		t.Fatalf("repairs = %+v", w.api.repairs)
	}
	if s := w.stored(t); s.Status != googlesheet.StatusActive || w.alerts.resolved != 1 {
		t.Fatalf("sheet = %+v, resolved %d", s, w.alerts.resolved)
	}
	w.write(t)
	column := w.column()
	if len(column) != 4 || column[0] != googlesheet.RefHeader || column[1] != "old-2" || column[2] != "old-1" || column[3] != fresh {
		t.Fatalf("column A = %v", column)
	}
}

func TestReactivateFindsADamagedTabOnAnActiveSheetAndOffersTheRepair(t *testing.T) {
	w := newWorld(t)
	w.damageLikeTheAppendHeuristic([]any{"old-1", "Ana"})
	_, err := w.manager.Reactivate(context.Background(), ReactivateInput{WorkspaceID: testWorkspace, ID: w.sheet.ID})
	stored := w.stored(t)
	if !errors.Is(err, googlesheet.ErrSheetNotActive) || stored.Status != googlesheet.StatusNeedsAttention || !stored.Reason.Repairable() {
		t.Fatalf("err = %v, sheet = %+v", err, stored)
	}
	if len(w.api.repairs) != 0 {
		t.Fatal("nothing may change in Google without the repair being asked for")
	}
}

func TestRepairNeverOverwritesADataRowSittingInRowOne(t *testing.T) {
	w := newWorld(t)
	w.api.setHeader(0, []any{"old-1", "Ana"})
	if _, err := w.manager.Reactivate(context.Background(), ReactivateInput{WorkspaceID: testWorkspace, ID: w.sheet.ID, RepairHeader: true}); err != nil {
		t.Fatalf("Reactivate: %v", err)
	}
	if len(w.api.repairs) != 1 || !w.api.repairs[0].Insert {
		t.Fatalf("repairs = %+v", w.api.repairs)
	}
	if column := w.column(); len(column) != 2 || column[0] != googlesheet.RefHeader || column[1] != "old-1" {
		t.Fatalf("column A = %v", column)
	}
}

func TestARepairGoogleDidNotApplyKeepsTheSheetHeld(t *testing.T) {
	w := newWorld(t)
	w.damageLikeTheAppendHeuristic([]any{"old-1", "Ana"})
	w.api.repairIgnored = true
	_, err := w.manager.Reactivate(context.Background(), ReactivateInput{WorkspaceID: testWorkspace, ID: w.sheet.ID, RepairHeader: true})
	if !errors.Is(err, googlesheet.ErrSheetNotActive) {
		t.Fatalf("err = %v, the repair must be verified before writes resume", err)
	}
	if s := w.stored(t); s.Status != googlesheet.StatusNeedsAttention || s.Reason != googlesheet.ReasonHeaderNotFirstRow || w.alerts.resolved != 0 {
		t.Fatalf("sheet = %+v, resolved %d", s, w.alerts.resolved)
	}
}
