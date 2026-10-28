package googlesheet

import (
	"fmt"
	"strings"
)

const (
	HeaderRange    = "1:1"
	RefColumnRange = "A:A"
	firstBodyRow   = 2
)

func RowRange(row int) string { return fmt.Sprintf("%d:%d", row, row) }

type TabSnapshot struct {
	Header [][]any
	Refs   [][]any
}

func cellText(row []any) string {
	if len(row) == 0 {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(row[0]))
}

func (t TabSnapshot) NextFreeRow() int {
	return max(len(t.Refs)+1, firstBodyRow)
}

func (t TabSnapshot) RefPositions() map[string]int {
	positions := make(map[string]int, len(t.Refs))
	for i, row := range t.Refs {
		ref := cellText(row)
		if ref == "" || ref == RefHeader {
			continue
		}
		positions[ref] = i + 1
	}
	return positions
}

func (t TabSnapshot) HeaderCandidate() (int, bool) {
	for i := firstBodyRow - 1; i < len(t.Refs); i++ {
		if cellText(t.Refs[i]) == RefHeader {
			return i + 1, true
		}
	}
	return 0, false
}

type HeaderPlace struct {
	Reason AttentionReason
	Row    int
	Detail string
}

func (p HeaderPlace) InPlace() bool { return p.Reason == "" }

func (s *Sheet) PlaceHeader(snapshot TabSnapshot, candidate [][]any) HeaderPlace {
	expected := s.HeaderRow()
	difference := HeaderDifference(expected, snapshot.Header)
	if difference == "" {
		return HeaderPlace{}
	}
	if row, found := snapshot.HeaderCandidate(); found && candidate != nil && HeaderDifference(expected, candidate) == "" {
		return HeaderPlace{Reason: ReasonHeaderNotFirstRow, Row: row, Detail: fmt.Sprintf("o cabeçalho está na linha %d e há linhas acima dele", row)}
	}
	return HeaderPlace{Reason: ReasonHeaderMismatch, Detail: difference}
}

type HeaderRepair struct {
	MoveFrom int
	Insert   bool
}

func (s *Sheet) RepairFor(snapshot TabSnapshot, place HeaderPlace) HeaderRepair {
	switch {
	case place.Reason == ReasonHeaderNotFirstRow:
		return HeaderRepair{MoveFrom: place.Row}
	case place.InPlace(), HeaderIsBlank(snapshot.Header):
		return HeaderRepair{}
	case len(snapshot.Header) > 0 && cellText(snapshot.Header[0]) == RefHeader:
		return HeaderRepair{}
	}
	return HeaderRepair{Insert: true}
}

func (r AttentionReason) Repairable() bool {
	return r == ReasonHeaderMismatch || r == ReasonHeaderNotFirstRow
}

func (s *Sheet) Repairable() bool {
	return s.Status == StatusNeedsAttention && s.Reason.Repairable()
}
