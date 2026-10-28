package googlesheet

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func fileSheet(id, spreadsheetID, title string, created time.Time, status Status, written *time.Time) *Sheet {
	return &Sheet{ID: id, SpreadsheetID: spreadsheetID, Title: title, CreatedAt: created, Status: status, LastWrittenAt: written}
}

func TestGroupBySpreadsheetKeepsEachFileOnceInFirstSeenOrder(t *testing.T) {
	day := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	files := GroupBySpreadsheet([]*Sheet{
		fileSheet("a", "sp-1", "Negociações", day, StatusActive, nil),
		fileSheet("b", "sp-2", "Pedidos", day, StatusActive, nil),
		fileSheet("c", "sp-1", "Pagamentos", day.Add(time.Minute), StatusActive, nil),
	})
	if len(files) != 2 || files[0].ID != "sp-1" || files[1].ID != "sp-2" {
		t.Fatalf("files = %+v", files)
	}
	if len(files[0].Sheets) != 2 || files[0].Sheets[0].ID != "a" || files[0].Sheets[1].ID != "c" {
		t.Fatalf("sheets of sp-1 = %+v", files[0].Sheets)
	}
}

func TestASpreadsheetIsNamedAfterItsOldestSheet(t *testing.T) {
	day := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	file := Spreadsheet{ID: "sp-1", Sheets: []*Sheet{
		fileSheet("c", "sp-1", "Pagamentos", day.Add(time.Minute), StatusActive, nil),
		fileSheet("a", "sp-1", "Negociações", day, StatusActive, nil),
	}}
	if file.Title() != "Negociações" {
		t.Fatalf("title = %q", file.Title())
	}
	if file.URL() != "https://docs.google.com/spreadsheets/d/sp-1/edit" {
		t.Fatalf("url = %q", file.URL())
	}
}

func TestASpreadsheetNeedsAttentionWhenAnyOfItsSheetsDoes(t *testing.T) {
	day := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		statuses []Status
		want     Status
	}{
		{"all active", []Status{StatusActive, StatusActive}, StatusActive},
		{"one needs attention", []Status{StatusActive, StatusNeedsAttention}, StatusNeedsAttention},
		{"every one needs attention", []Status{StatusNeedsAttention, StatusNeedsAttention}, StatusNeedsAttention},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := Spreadsheet{ID: "sp-1"}
			for i, status := range tc.statuses {
				file.Sheets = append(file.Sheets, fileSheet(string(rune('a'+i)), "sp-1", "Aba", day, status, nil))
			}
			if got := file.Status(); got != tc.want {
				t.Fatalf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestASpreadsheetWasLastWrittenWhenItsLatestSheetWas(t *testing.T) {
	day := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	early, late := day.Add(time.Hour), day.Add(2*time.Hour)
	file := Spreadsheet{ID: "sp-1", Sheets: []*Sheet{
		fileSheet("a", "sp-1", "Negociações", day, StatusActive, &late),
		fileSheet("b", "sp-1", "Pagamentos", day, StatusActive, nil),
		fileSheet("c", "sp-1", "Pedidos", day, StatusActive, &early),
	}}
	if got := file.LastWrittenAt(); got == nil || !got.Equal(late) {
		t.Fatalf("last written = %v", got)
	}
	unwritten := Spreadsheet{ID: "sp-2", Sheets: []*Sheet{fileSheet("d", "sp-2", "Novo", day, StatusActive, nil)}}
	if got := unwritten.LastWrittenAt(); got != nil {
		t.Fatalf("a file never written has a last write: %v", got)
	}
}

func TestASpreadsheetIsNamedByItsStoredNameWhenItHasOne(t *testing.T) {
	day := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	file := Spreadsheet{ID: "sp-1", Name: "Altiva", Sheets: []*Sheet{fileSheet("a", "sp-1", "Negociações", day, StatusActive, nil)}}
	if file.Title() != "Altiva" {
		t.Fatalf("title = %q", file.Title())
	}
}

func TestAFileTitleIsTrimmedAndRefusedWhenBlankOrTooLong(t *testing.T) {
	title, err := CheckFileTitle("  CDT Jabaquara  ")
	if err != nil || title != "CDT Jabaquara" {
		t.Fatalf("CheckFileTitle = %q, %v", title, err)
	}
	cases := []struct {
		name  string
		title string
		code  string
		limit int
	}{
		{"blank", "   ", CodeFileTitleRequired, 0},
		{"too long", strings.Repeat("a", MaxFileTitleRunes+1), CodeFileTitleTooLong, MaxFileTitleRunes},
		{"a line break", "Altiva\nCobrança", CodeFileTitleInvalid, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CheckFileTitle(tc.title)
			var def *DefinitionError
			if !errors.As(err, &def) || def.Code != tc.code || def.Field != FieldFileTitle || def.Limit != tc.limit || !errors.Is(err, ErrInvalidSheet) {
				t.Fatalf("CheckFileTitle(%q) = %v", tc.title, err)
			}
		})
	}
}
