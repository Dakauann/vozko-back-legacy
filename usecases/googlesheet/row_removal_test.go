package googlesheet

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
)

type memoryRemovals struct {
	mu       sync.Mutex
	rows     *memoryRows
	removals []*googlesheet.RowRemoval
}

func (m *memoryRemovals) FindRow(_ context.Context, workspaceID, sheetID, rowID string) (*googlesheet.Row, error) {
	m.rows.mu.Lock()
	defer m.rows.mu.Unlock()
	row, ok := m.rows.rows[rowID]
	if !ok || row.WorkspaceID != workspaceID || row.SheetID != sheetID {
		return nil, googlesheet.ErrRowNotFound
	}
	c := *row
	return &c, nil
}

func (m *memoryRemovals) record(removal *googlesheet.RowRemoval) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored := *removal
	stored.ID = uuid.NewString()
	removal.ID = stored.ID
	m.removals = append(m.removals, &stored)
}

func (m *memoryRemovals) CancelQueued(_ context.Context, removal *googlesheet.RowRemoval) error {
	m.rows.mu.Lock()
	row, ok := m.rows.rows[removal.RowID]
	queued := ok && row.SheetID == removal.SheetID && (row.Status == googlesheet.RowPending || row.Status == googlesheet.RowHeld || row.Status == googlesheet.RowFailed)
	if queued {
		delete(m.rows.rows, removal.RowID)
	}
	m.rows.mu.Unlock()
	if !queued {
		return googlesheet.ErrRowStatusChanged
	}
	m.record(removal)
	return nil
}

func (m *memoryRemovals) RecordDeleted(_ context.Context, removal *googlesheet.RowRemoval) error {
	m.rows.mu.Lock()
	if row, ok := m.rows.rows[removal.RowID]; ok && row.Status == googlesheet.RowWritten {
		delete(m.rows.rows, removal.RowID)
	}
	m.rows.mu.Unlock()
	m.record(removal)
	return nil
}

func (m *memoryRemovals) ListRemovals(_ context.Context, workspaceID, sheetID string, p shared.Pagination) (*shared.PaginatedResult[*googlesheet.RowRemoval], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*googlesheet.RowRemoval
	for _, removal := range m.removals {
		if removal.WorkspaceID == workspaceID && removal.SheetID == sheetID {
			c := *removal
			out = append(out, &c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RemovedAt.After(out[j].RemovedAt) })
	return shared.NewPaginatedResult(out, p, int64(len(out))), nil
}

type removalWorld struct {
	*world
	removals *memoryRemovals
	remover  *RowRemover
}

func newRemovalWorld(t *testing.T) *removalWorld {
	t.Helper()
	w := newWorld(t)
	r := &removalWorld{world: w, removals: &memoryRemovals{rows: w.rows}}
	r.remover = NewRowRemover(RowRemoverDeps{Manager: w.manager, Removals: r.removals, Leases: w.leases})
	return r
}

func (r *removalWorld) input(rowID string) RemoveRowInput {
	return RemoveRowInput{WorkspaceID: testWorkspace, SheetID: r.sheet.ID, RowID: rowID, ActorID: "user-1", Reason: "registro duplicado"}
}

func (r *removalWorld) writtenRows(t *testing.T, names ...string) []string {
	t.Helper()
	ids := make([]string, len(names))
	for i, name := range names {
		ids[i] = r.enqueue(t, "k-"+name, map[string]any{"nome": name}).RowID
	}
	r.write(t)
	return ids
}

func (r *removalWorld) arrange(rows ...[]any) {
	r.api.mu.Lock()
	defer r.api.mu.Unlock()
	r.api.store(0, append([][]any{r.api.grid(0)[0]}, rows...))
}

func (r *removalWorld) googleCalls() int {
	r.api.mu.Lock()
	defer r.api.mu.Unlock()
	return r.api.readCalls + r.api.workbookCalls + len(r.api.deletedRows)
}

func (r *removalWorld) assertUntouched(t *testing.T, rowID string) {
	t.Helper()
	if _, ok := r.rows.rows[rowID]; !ok {
		t.Fatal("the ledger row was removed although the removal was refused")
	}
	if len(r.removals.removals) != 0 {
		t.Fatalf("a refused removal left traces: %+v", r.removals.removals)
	}
}

func TestCancellingAQueuedRowTakesItOutBeforeGoogleSeesIt(t *testing.T) {
	r := newRemovalWorld(t)
	queued := r.enqueue(t, "k1", map[string]any{"nome": "Ana", "valor": 10})

	removal, err := r.remover.Cancel(context.Background(), r.input(queued.RowID))
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if removal.Kind != googlesheet.RemovalCancel || removal.RemovedBy != "user-1" || removal.Reason != "registro duplicado" || removal.Cells["nome"] != "Ana" || !removal.RemovedAt.Equal(r.now) {
		t.Fatalf("removal = %+v", removal)
	}
	if _, ok := r.rows.rows[queued.RowID]; ok {
		t.Fatal("the queued row is still in the outbox")
	}
	if r.googleCalls() != 0 {
		t.Fatal("cancelling a queued row must not call google")
	}
	r.write(t)
	if len(r.api.appends) != 0 {
		t.Fatalf("a cancelled row reached google: %+v", r.api.appends)
	}
}

func TestCancellingAHeldOrFailedRowWorksEvenWhileTheSheetNeedsAttention(t *testing.T) {
	r := newRemovalWorld(t)
	queued := r.enqueue(t, "k1", map[string]any{"nome": "Ana"})
	if err := r.rows.Hold(context.Background(), []string{queued.RowID}, googlesheet.CodeWriteAccessLost, "no access"); err != nil {
		t.Fatal(err)
	}
	stored := r.stored(t)
	stored.NeedsAttention(googlesheet.ReasonAccessLost, "no access")
	if err := r.sheets.SaveStatus(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	if _, err := r.remover.Cancel(context.Background(), r.input(queued.RowID)); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
}

func TestCancelRefusesARowTheWriterClaimedOrAlreadyWrote(t *testing.T) {
	r := newRemovalWorld(t)
	claimed := r.enqueue(t, "k1", map[string]any{"nome": "Ana"}).RowID
	r.rows.set([]string{claimed}, func(row *googlesheet.Row) { row.Status = googlesheet.RowInFlight })
	if _, err := r.remover.Cancel(context.Background(), r.input(claimed)); !errors.Is(err, googlesheet.ErrRowBeingWritten) {
		t.Fatalf("in flight = %v", err)
	}
	r.assertUntouched(t, claimed)
	r.rows.set([]string{claimed}, func(row *googlesheet.Row) { row.Status = googlesheet.RowWritten })
	if _, err := r.remover.Cancel(context.Background(), r.input(claimed)); !errors.Is(err, googlesheet.ErrRowStatusChanged) {
		t.Fatalf("written = %v", err)
	}
	r.assertUntouched(t, claimed)
}

func TestCancelThatLosesTheRaceToTheWriterChangesNothing(t *testing.T) {
	r := newRemovalWorld(t)
	queued := r.enqueue(t, "k1", map[string]any{"nome": "Ana"}).RowID
	racing := &racingRemovals{memoryRemovals: r.removals, before: func() {
		r.rows.set([]string{queued}, func(row *googlesheet.Row) { row.Status = googlesheet.RowInFlight })
	}}
	remover := NewRowRemover(RowRemoverDeps{Manager: r.manager, Removals: racing, Leases: r.leases})
	if _, err := remover.Cancel(context.Background(), r.input(queued)); !errors.Is(err, googlesheet.ErrRowStatusChanged) {
		t.Fatalf("err = %v", err)
	}
	r.assertUntouched(t, queued)
}

type racingRemovals struct {
	*memoryRemovals
	before func()
}

func (r *racingRemovals) CancelQueued(ctx context.Context, removal *googlesheet.RowRemoval) error {
	r.before()
	return r.memoryRemovals.CancelQueued(ctx, removal)
}

func TestRemovingAWrittenRowDeletesItWhereverItWasMoved(t *testing.T) {
	r := newRemovalWorld(t)
	ids := r.writtenRows(t, "Ana", "Bia", "Caio")
	r.arrange([]any{ids[2], "Caio"}, []any{"", "linha digitada"}, []any{ids[0], "Ana"}, []any{ids[1], "Bia"})

	removal, err := r.remover.Remove(context.Background(), r.input(ids[0]))
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(r.api.deletedRows) != 1 || r.api.deletedRows[0] != 4 {
		t.Fatalf("deleted rows = %v, want row 4 where the ref moved to", r.api.deletedRows)
	}
	if got := r.column(); len(got) != 4 || got[1] != ids[2] || got[2] != "" || got[3] != ids[1] {
		t.Fatalf("column A = %v", got)
	}
	if removal.Kind != googlesheet.RemovalDelete || removal.RowNumber != 4 || removal.Cells["nome"] != "Ana" || removal.RowID != ids[0] {
		t.Fatalf("removal = %+v", removal)
	}
	if _, ok := r.rows.rows[ids[0]]; ok {
		t.Fatal("the written row is still in the ledger")
	}
	if len(r.leases.held) != 0 {
		t.Fatalf("the sheet lease was kept: %v", r.leases.held)
	}
}

func TestRemovingARowOfAnEarlierMonthDeletesItInThatMonthsTab(t *testing.T) {
	r := newRemovalWorld(t)
	payments := r.periodicPayments(t)
	r.enqueueOn(t, payments, "oct", map[string]any{"valor": 1})
	if err := r.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	october := r.rows.byStatus(payments.ID, googlesheet.RowWritten)[0].ID
	r.now = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	r.enqueueOn(t, payments, "nov", map[string]any{"valor": 2})
	if err := r.writer.WriteSheet(context.Background(), payments.ID); err != nil {
		t.Fatal(err)
	}
	if stored, _ := r.sheets.Get(context.Background(), payments.ID); stored.TabID == 51 {
		t.Fatal("november must have opened its own tab")
	}
	in := RemoveRowInput{WorkspaceID: testWorkspace, SheetID: payments.ID, RowID: october, ActorID: "user-1", Reason: "lançado errado"}
	if _, err := r.remover.Remove(context.Background(), in); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	for _, row := range r.api.grid(51)[1:] {
		if len(row) > 0 && row[0] == october {
			t.Fatal("the october row is still in the october tab")
		}
	}
}

func TestRemovingIsRefusedWhenTheRefIsNotExactlyOnceInColumnA(t *testing.T) {
	r := newRemovalWorld(t)
	ids := r.writtenRows(t, "Ana", "Bia")
	r.arrange([]any{ids[1], "Bia"})
	if _, err := r.remover.Remove(context.Background(), r.input(ids[0])); !errors.Is(err, googlesheet.ErrRowNotInSheet) {
		t.Fatalf("gone = %v", err)
	}
	r.assertUntouched(t, ids[0])
	r.arrange([]any{ids[0], "Ana"}, []any{ids[1], "Bia"}, []any{ids[0], "cópia"})
	if _, err := r.remover.Remove(context.Background(), r.input(ids[0])); !errors.Is(err, googlesheet.ErrRowRefRepeated) {
		t.Fatalf("repeated = %v", err)
	}
	r.assertUntouched(t, ids[0])
	if len(r.api.deletedRows) != 0 {
		t.Fatalf("google deleted rows %v", r.api.deletedRows)
	}
}

func TestRemovingIsRefusedWhileTheSheetNeedsAttention(t *testing.T) {
	r := newRemovalWorld(t)
	ids := r.writtenRows(t, "Ana")
	stored := r.stored(t)
	stored.NeedsAttention(googlesheet.ReasonAccessLost, "no access")
	if err := r.sheets.SaveStatus(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	before := r.googleCalls()
	if _, err := r.remover.Remove(context.Background(), r.input(ids[0])); !errors.Is(err, googlesheet.ErrSheetNotActive) {
		t.Fatalf("err = %v", err)
	}
	if r.googleCalls() != before {
		t.Fatal("a paused sheet must not be read or changed")
	}
	r.assertUntouched(t, ids[0])
}

func TestRemovingIsRefusedAndTheSheetPausedWhenRowOneIsNotTheHeader(t *testing.T) {
	r := newRemovalWorld(t)
	ids := r.writtenRows(t, "Ana")
	r.api.setHeader(0, []any{googlesheet.RefHeader, "Cliente", "Valor", "Telefone"})
	if _, err := r.remover.Remove(context.Background(), r.input(ids[0])); !errors.Is(err, googlesheet.ErrSheetNotActive) {
		t.Fatalf("err = %v", err)
	}
	if stored := r.stored(t); stored.Status != googlesheet.StatusNeedsAttention || stored.Reason != googlesheet.ReasonHeaderMismatch {
		t.Fatalf("sheet = %s %s", stored.Status, stored.Reason)
	}
	if len(r.api.deletedRows) != 0 {
		t.Fatalf("deleted %v under a wrong header", r.api.deletedRows)
	}
	r.assertUntouched(t, ids[0])
}

func TestRemovingWaitsForNoWriteBatchAndRefusesWhileOneHoldsTheSheet(t *testing.T) {
	r := newRemovalWorld(t)
	ids := r.writtenRows(t, "Ana")
	r.leases.held[sheetLeaseKey(r.sheet.ID)] = true
	before := r.googleCalls()
	if _, err := r.remover.Remove(context.Background(), r.input(ids[0])); !errors.Is(err, googlesheet.ErrSheetBusy) {
		t.Fatalf("err = %v", err)
	}
	if r.googleCalls() != before {
		t.Fatal("google was called while the writer held the sheet")
	}
	r.assertUntouched(t, ids[0])
}

func TestRemovingSpendsTheSameQuotaAsTheWriter(t *testing.T) {
	r := newRemovalWorld(t)
	ids := r.writtenRows(t, "Ana")
	r.limiter.writes = 0
	if _, err := r.remover.Remove(context.Background(), r.input(ids[0])); !errors.Is(err, googlesheet.ErrGoogleBusy) {
		t.Fatalf("no write quota = %v", err)
	}
	if len(r.api.deletedRows) != 0 {
		t.Fatal("deleted without write quota")
	}
	r.limiter.writes, r.limiter.reads = 10, 0
	if _, err := r.remover.Remove(context.Background(), r.input(ids[0])); !errors.Is(err, googlesheet.ErrGoogleBusy) {
		t.Fatalf("no read quota = %v", err)
	}
	r.assertUntouched(t, ids[0])
}

func TestAGoogleRefusalLeavesTheLedgerAsItWas(t *testing.T) {
	r := newRemovalWorld(t)
	ids := r.writtenRows(t, "Ana")
	r.api.deleteErr = errForbidden
	_, err := r.remover.Remove(context.Background(), r.input(ids[0]))
	var refusal *GoogleRefusal
	if !errors.As(err, &refusal) || refusal.Failure.Kind != googlesheet.FailurePermission {
		t.Fatalf("err = %v", err)
	}
	r.assertUntouched(t, ids[0])
}

func TestRemovingRefusesAnotherGoogleAccount(t *testing.T) {
	r := newRemovalWorld(t)
	ids := r.writtenRows(t, "Ana")
	r.access.access.AccountID = "acct-2"
	if _, err := r.remover.Remove(context.Background(), r.input(ids[0])); !errors.Is(err, googlesheet.ErrAccountChanged) {
		t.Fatalf("err = %v", err)
	}
	r.assertUntouched(t, ids[0])
}

func TestEachActionOnlyTakesTheRowItIsFor(t *testing.T) {
	r := newRemovalWorld(t)
	written := r.writtenRows(t, "Ana")[0]
	queued := r.enqueue(t, "k2", map[string]any{"nome": "Bia"}).RowID
	if _, err := r.remover.Remove(context.Background(), r.input(queued)); !errors.Is(err, googlesheet.ErrRowStatusChanged) {
		t.Fatalf("remove queued = %v", err)
	}
	if _, err := r.remover.Cancel(context.Background(), r.input(written)); !errors.Is(err, googlesheet.ErrRowStatusChanged) {
		t.Fatalf("cancel written = %v", err)
	}
	if len(r.api.deletedRows) != 0 || len(r.removals.removals) != 0 {
		t.Fatal("a mismatched action changed something")
	}
}

func TestRemovalsCheckTheReasonThePersonAndTheWorkspaceFirst(t *testing.T) {
	r := newRemovalWorld(t)
	ids := r.writtenRows(t, "Ana")
	cases := map[string]struct {
		in   RemoveRowInput
		want error
	}{
		"blank reason":    {RemoveRowInput{WorkspaceID: testWorkspace, SheetID: r.sheet.ID, RowID: ids[0], ActorID: "user-1", Reason: " "}, googlesheet.ErrRemovalReasonRequired},
		"no person":       {RemoveRowInput{WorkspaceID: testWorkspace, SheetID: r.sheet.ID, RowID: ids[0], Reason: "x"}, googlesheet.ErrRemovalActorRequired},
		"other workspace": {RemoveRowInput{WorkspaceID: "other", SheetID: r.sheet.ID, RowID: ids[0], ActorID: "user-1", Reason: "x"}, googlesheet.ErrSheetNotFound},
		"not a ref":       {RemoveRowInput{WorkspaceID: testWorkspace, SheetID: r.sheet.ID, RowID: "ref", ActorID: "user-1", Reason: "x"}, googlesheet.ErrRowNotFound},
		"unknown row":     {RemoveRowInput{WorkspaceID: testWorkspace, SheetID: r.sheet.ID, RowID: uuid.NewString(), ActorID: "user-1", Reason: "x"}, googlesheet.ErrRowNotFound},
	}
	before := r.googleCalls()
	for name, c := range cases {
		if _, err := r.remover.Remove(context.Background(), c.in); !errors.Is(err, c.want) {
			t.Errorf("%s remove: %v, want %v", name, err, c.want)
		}
		if _, err := r.remover.Cancel(context.Background(), c.in); !errors.Is(err, c.want) {
			t.Errorf("%s cancel: %v, want %v", name, err, c.want)
		}
	}
	if r.googleCalls() != before {
		t.Fatal("a refused request reached google")
	}
	r.assertUntouched(t, ids[0])
}

func TestTheRemovedListShowsWhoRemovedWhatAndWhy(t *testing.T) {
	r := newRemovalWorld(t)
	r.people.names["user-1"] = "Ana Paula"
	r.workflows.names["wf-1"] = "Cobrança"
	ids := r.writtenRows(t, "Ana")
	if _, err := r.remover.Remove(context.Background(), r.input(ids[0])); err != nil {
		t.Fatal(err)
	}
	r.now = r.now.Add(time.Minute)
	queued := r.enqueue(t, "k2", map[string]any{"nome": "Bia"}).RowID
	if _, err := r.remover.Cancel(context.Background(), r.input(queued)); err != nil {
		t.Fatal(err)
	}

	page, err := r.remover.Removals(context.Background(), testWorkspace, r.sheet.ID, shared.Pagination{Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].Removal.RowID != queued || page.Items[1].Removal.RowID != ids[0] {
		t.Fatalf("items = %+v", page.Items)
	}
	first := page.Items[1]
	if first.RemovedBy == nil || first.RemovedBy.Name != "Ana Paula" || first.Origin.Workflow == nil || first.Origin.Workflow.Name != "Cobrança" ||
		first.Removal.Reason != "registro duplicado" || first.Removal.Cells["nome"] != "Ana" {
		t.Fatalf("first = %+v", first)
	}
	if _, err := r.remover.Removals(context.Background(), "other", r.sheet.ID, shared.Pagination{}); !errors.Is(err, googlesheet.ErrSheetNotFound) {
		t.Fatalf("cross workspace = %v", err)
	}
}

func TestTheRemoverRefusesToStartWithoutItsDependencies(t *testing.T) {
	r := newRemovalWorld(t)
	for name, deps := range map[string]RowRemoverDeps{
		"manager":  {Removals: r.removals, Leases: r.leases},
		"removals": {Manager: r.manager, Leases: r.leases},
		"leases":   {Manager: r.manager, Removals: r.removals},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("no %s accepted", name)
				}
			}()
			NewRowRemover(deps)
		}()
	}
}
