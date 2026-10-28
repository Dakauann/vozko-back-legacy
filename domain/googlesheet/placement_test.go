package googlesheet

import (
	"strings"
	"testing"
)

func placementSheet(t *testing.T) *Sheet {
	t.Helper()
	s, err := NewSheet(NewSheetInput{
		WorkspaceID: "ws-1", AccountID: "acct-1", Title: "Pagamentos",
		Columns: []ColumnInput{{Header: "Cliente", Type: TypeText}, {Header: "Valor", Type: TypeMoney}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func headerOf(s *Sheet) []any {
	out := make([]any, 0, len(s.Columns)+1)
	for _, h := range s.HeaderRow() {
		out = append(out, h)
	}
	return out
}

func TestNextFreeRowIsRightBelowTheLastRefSoHumanRowsBelowAreNeverOverwritten(t *testing.T) {
	cases := []struct {
		name string
		refs [][]any
		want int
	}{
		{"only the header", [][]any{{RefHeader}}, 2},
		{"blank column", nil, 2},
		{"three written rows", [][]any{{RefHeader}, {"r1"}, {"r2"}, {"r3"}}, 5},
		{"a human row without ref in between", [][]any{{RefHeader}, {"r1"}, {}, {"r2"}}, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := (TabSnapshot{Refs: c.refs}).NextFreeRow(); got != c.want {
				t.Fatalf("NextFreeRow = %d, want %d", got, c.want)
			}
		})
	}
}

func TestRefPositionsSkipTheHeaderAndBlankCells(t *testing.T) {
	positions := (TabSnapshot{Refs: [][]any{{RefHeader}, {"r1"}, {}, {" r2 "}}}).RefPositions()
	if len(positions) != 2 || positions["r1"] != 2 || positions["r2"] != 4 {
		t.Fatalf("positions = %v", positions)
	}
}

func TestPlaceHeaderAcceptsTheHeaderInRowOne(t *testing.T) {
	s := placementSheet(t)
	place := s.PlaceHeader(TabSnapshot{Header: [][]any{headerOf(s)}, Refs: [][]any{{RefHeader}}}, nil)
	if place.Reason != "" || place.Row != 0 {
		t.Fatalf("place = %+v", place)
	}
}

func TestPlaceHeaderFindsTheHeaderPushedDownByRowsWrittenAboveIt(t *testing.T) {
	s := placementSheet(t)
	snapshot := TabSnapshot{
		Header: [][]any{{"r-17", "Ana", 10.0}},
		Refs:   [][]any{{"r-17"}, {"r-16"}, {RefHeader}},
	}
	row, found := snapshot.HeaderCandidate()
	if !found || row != 3 {
		t.Fatalf("HeaderCandidate = %d, %v", row, found)
	}
	place := s.PlaceHeader(snapshot, [][]any{headerOf(s)})
	if place.Reason != ReasonHeaderNotFirstRow || place.Row != 3 || !strings.Contains(place.Detail, "linha 3") {
		t.Fatalf("place = %+v", place)
	}
	if !place.Reason.Repairable() {
		t.Fatal("a header pushed down must be repairable")
	}
}

func TestPlaceHeaderIsAMismatchWhenTheLowerRowIsNotTheWholeHeader(t *testing.T) {
	s := placementSheet(t)
	snapshot := TabSnapshot{Header: [][]any{{"r-1"}}, Refs: [][]any{{"r-1"}, {RefHeader}}}
	place := s.PlaceHeader(snapshot, [][]any{{RefHeader, "Cliente", "Outro"}})
	if place.Reason != ReasonHeaderMismatch || place.Row != 0 || place.Detail == "" {
		t.Fatalf("place = %+v", place)
	}
}

func TestPlaceHeaderIsAMismatchWhenNoLowerHeaderExists(t *testing.T) {
	s := placementSheet(t)
	place := s.PlaceHeader(TabSnapshot{Header: [][]any{{RefHeader, "Nome", "Valor"}}, Refs: [][]any{{RefHeader}}}, nil)
	if place.Reason != ReasonHeaderMismatch || !strings.Contains(place.Detail, "Cliente") {
		t.Fatalf("place = %+v", place)
	}
}

func TestHeaderRepairNeverOverwritesARowThatIsNotAHeader(t *testing.T) {
	s := placementSheet(t)
	cases := []struct {
		name     string
		snapshot TabSnapshot
		lower    [][]any
		want     HeaderRepair
	}{
		{"header pushed down", TabSnapshot{Header: [][]any{{"r-1", "Ana"}}, Refs: [][]any{{"r-1"}, {RefHeader}}}, [][]any{headerOf(s)}, HeaderRepair{MoveFrom: 2}},
		{"renamed column", TabSnapshot{Header: [][]any{{RefHeader, "Nome", "Valor"}}, Refs: [][]any{{RefHeader}}}, nil, HeaderRepair{}},
		{"blank row one", TabSnapshot{}, nil, HeaderRepair{}},
		{"a data row in row one and no header anywhere", TabSnapshot{Header: [][]any{{"r-1", "Ana"}}, Refs: [][]any{{"r-1"}}}, nil, HeaderRepair{Insert: true}},
		{"human text in row one", TabSnapshot{Header: [][]any{{"", "Planilha do João"}}, Refs: nil}, nil, HeaderRepair{Insert: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			place := s.PlaceHeader(c.snapshot, c.lower)
			if got := s.RepairFor(c.snapshot, place); got != c.want {
				t.Fatalf("RepairFor = %+v, want %+v (place %+v)", got, c.want, place)
			}
		})
	}
}

func TestOnlyHeaderReasonsAreRepairable(t *testing.T) {
	for _, reason := range AttentionReasons() {
		want := reason == ReasonHeaderMismatch || reason == ReasonHeaderNotFirstRow
		if reason.Repairable() != want {
			t.Fatalf("%s repairable = %v", reason, reason.Repairable())
		}
	}
}

func TestASheetOffersTheRepairOnlyWhilePausedForAHeaderReason(t *testing.T) {
	s := placementSheet(t)
	if s.Repairable() {
		t.Fatal("an active sheet has nothing to repair")
	}
	s.NeedsAttention(ReasonHeaderNotFirstRow, "linha 18")
	if !s.Repairable() {
		t.Fatal("a header pushed down must offer the repair")
	}
	s.NeedsAttention(ReasonAccessLost, "")
	if s.Repairable() {
		t.Fatal("a lost access is not fixed by a header repair")
	}
}
