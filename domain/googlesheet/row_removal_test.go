package googlesheet

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	refA = "6f1c2d3e-4a5b-4c6d-8e7f-90a1b2c3d4e5"
	refB = "7a2b3c4d-5e6f-4a7b-9c8d-0e1f2a3b4c5d"
)

func TestEachRowStatusOffersTheRemovalThatFitsIt(t *testing.T) {
	cases := []struct {
		status  RowStatus
		kind    RemovalKind
		offered bool
	}{
		{RowPending, RemovalCancel, true},
		{RowHeld, RemovalCancel, true},
		{RowFailed, RemovalCancel, true},
		{RowWritten, RemovalDelete, true},
		{RowInFlight, "", false},
	}
	for _, c := range cases {
		kind, offered := (&Row{Status: c.status}).Removal()
		if kind != c.kind || offered != c.offered {
			t.Errorf("%s: removal = %q %v, want %q %v", c.status, kind, offered, c.kind, c.offered)
		}
	}
}

func TestQueuedStatusesAreExactlyTheOnesACancelRemoves(t *testing.T) {
	for _, status := range QueuedStatuses() {
		if kind, _ := (&Row{Status: status}).Removal(); kind != RemovalCancel {
			t.Errorf("%s is queued but not cancellable", status)
		}
	}
	if len(QueuedStatuses()) != 3 {
		t.Fatalf("queued = %v", QueuedStatuses())
	}
}

func TestARowBeingWrittenOrThatMovedOnRefusesTheRemovalAskedFor(t *testing.T) {
	if err := (&Row{Status: RowInFlight}).CheckRemoval(RemovalCancel); !errors.Is(err, ErrRowBeingWritten) {
		t.Fatalf("in flight cancel = %v", err)
	}
	if err := (&Row{Status: RowWritten}).CheckRemoval(RemovalCancel); !errors.Is(err, ErrRowStatusChanged) {
		t.Fatalf("written cancel = %v", err)
	}
	if err := (&Row{Status: RowPending}).CheckRemoval(RemovalDelete); !errors.Is(err, ErrRowStatusChanged) {
		t.Fatalf("pending delete = %v", err)
	}
	if err := (&Row{Status: RowHeld}).CheckRemoval(RemovalCancel); err != nil {
		t.Fatalf("held cancel = %v", err)
	}
}

func TestARemovalNeedsAReasonThatFits(t *testing.T) {
	if _, err := CheckRemovalReason("   "); !errors.Is(err, ErrRemovalReasonRequired) {
		t.Fatalf("blank = %v", err)
	}
	if _, err := CheckRemovalReason(strings.Repeat("é", MaxRemovalReasonRunes+1)); !errors.Is(err, ErrRemovalReasonTooLong) {
		t.Fatalf("long = %v", err)
	}
	got, err := CheckRemovalReason("  " + strings.Repeat("é", MaxRemovalReasonRunes) + "\n")
	if err != nil || got != strings.Repeat("é", MaxRemovalReasonRunes) {
		t.Fatalf("fits = %q %v", got, err)
	}
}

func TestARemovalSnapshotsTheRowWithWhoWhenAndWhy(t *testing.T) {
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	row := &Row{
		ID: refA, WorkspaceID: "ws", SheetID: "sheet", Status: RowWritten, PeriodKey: "2026-10", Cells: Cells{"nome": "Ana"},
		Origin: Origin{Kind: OriginWorkflow, WorkflowID: "wf", LeadID: "lead", EntryID: "conv", EntryType: "whatsapp"},
	}
	removal, err := NewRowRemoval(row, RemovalDelete, " user-1 ", " pedido duplicado ", 7, at)
	if err != nil {
		t.Fatal(err)
	}
	if removal.RowID != refA || removal.WorkspaceID != "ws" || removal.SheetID != "sheet" || removal.Kind != RemovalDelete ||
		removal.RemovedBy != "user-1" || removal.Reason != "pedido duplicado" || removal.RowNumber != 7 || removal.PeriodKey != "2026-10" ||
		!removal.RemovedAt.Equal(at) || removal.Cells["nome"] != "Ana" || removal.Origin != row.Origin {
		t.Fatalf("removal = %+v", removal)
	}
	row.Cells["nome"] = "changed"
	if removal.Cells["nome"] != "Ana" {
		t.Fatal("the snapshot must not follow later changes to the row")
	}
}

func TestARemovalIsRefusedWithoutAPersonOrForTheWrongStatus(t *testing.T) {
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	if _, err := NewRowRemoval(&Row{ID: refA, Status: RowPending}, RemovalCancel, " ", "motivo", 0, at); !errors.Is(err, ErrRemovalActorRequired) {
		t.Fatalf("no actor = %v", err)
	}
	if _, err := NewRowRemoval(&Row{ID: refA, Status: RowInFlight}, RemovalCancel, "u", "motivo", 0, at); !errors.Is(err, ErrRowBeingWritten) {
		t.Fatalf("in flight = %v", err)
	}
	if _, err := NewRowRemoval(&Row{ID: refA, Status: RowWritten}, RemovalDelete, "u", "motivo", 1, at); !errors.Is(err, ErrRowNotInSheet) {
		t.Fatalf("header row = %v", err)
	}
	if removal, err := NewRowRemoval(&Row{ID: refA, Status: RowFailed}, RemovalCancel, "u", "motivo", 0, at); err != nil || removal.RowNumber != 0 {
		t.Fatalf("cancel = %+v %v", removal, err)
	}
}

func refs(cells ...string) [][]any {
	out := make([][]any, len(cells))
	for i, cell := range cells {
		out[i] = []any{}
		if cell != "" {
			out[i] = []any{cell}
		}
	}
	return out
}

func TestLocateFindsTheRefWhereverTheRowWasMoved(t *testing.T) {
	snapshot := TabSnapshot{Refs: refs(RefHeader, refB, "", refA)}
	row, err := snapshot.Locate(refA)
	if err != nil || row != 4 {
		t.Fatalf("row = %d %v, want 4", row, err)
	}
}

func TestLocateRefusesWhatIsNotExactlyOneVozkoRefBelowTheHeader(t *testing.T) {
	cases := map[string]struct {
		snapshot TabSnapshot
		ref      string
		want     error
	}{
		"missing":          {TabSnapshot{Refs: refs(RefHeader, refB)}, refA, ErrRowNotInSheet},
		"repeated":         {TabSnapshot{Refs: refs(RefHeader, refA, refB, refA)}, refA, ErrRowRefRepeated},
		"only the header":  {TabSnapshot{Refs: refs(refA)}, refA, ErrRowNotInSheet},
		"not a vozko ref":  {TabSnapshot{Refs: refs(RefHeader, "linha 7")}, "linha 7", ErrRowNotInSheet},
		"the header label": {TabSnapshot{Refs: refs(RefHeader, RefHeader)}, RefHeader, ErrRowNotInSheet},
		"uppercase ref":    {TabSnapshot{Refs: refs(RefHeader, strings.ToUpper(refA))}, strings.ToUpper(refA), ErrRowNotInSheet},
		"padded ref":       {TabSnapshot{Refs: refs(RefHeader, " "+refA+" ")}, refA, ErrRowNotInSheet},
	}
	for name, c := range cases {
		if row, err := c.snapshot.Locate(c.ref); !errors.Is(err, c.want) || row != 0 {
			t.Errorf("%s: row = %d err = %v, want %v", name, row, err, c.want)
		}
	}
}

func TestOnlyCanonicalRowIDsAreVozkoRefs(t *testing.T) {
	if !IsRowRef(refA) || IsRowRef(strings.ToUpper(refA)) || IsRowRef("row-1") || IsRowRef("") || IsRowRef(RefHeader) {
		t.Fatal("a vozko ref is a canonical lowercase uuid")
	}
}

func TestRemovalErrorsCarryCodes(t *testing.T) {
	for _, err := range []error{
		ErrRowBeingWritten, ErrRowStatusChanged, ErrRowNotInSheet, ErrRowRefRepeated,
		ErrRemovalReasonRequired, ErrRemovalReasonTooLong, ErrRemovalActorRequired, ErrSheetBusy,
	} {
		if code := ErrorCode(err); code == CodeInternal {
			t.Errorf("%v has no code", err)
		}
	}
}
