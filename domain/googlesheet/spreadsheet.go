package googlesheet

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxFileTitleRunes = MaxTitleRunes

type Spreadsheet struct {
	ID     string
	Name   string
	Sheets []*Sheet
}

func CheckFileTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	switch {
	case title == "":
		return "", refuse(FieldFileTitle, CodeFileTitleRequired)
	case utf8.RuneCountInString(title) > MaxFileTitleRunes:
		return "", refuse(FieldFileTitle, CodeFileTitleTooLong).limit(MaxFileTitleRunes)
	case strings.IndexFunc(title, unicode.IsControl) >= 0:
		return "", refuse(FieldFileTitle, CodeFileTitleInvalid).detail("%q", title)
	}
	return title, nil
}

func GroupBySpreadsheet(sheets []*Sheet) []Spreadsheet {
	index := map[string]int{}
	var out []Spreadsheet
	for _, sheet := range sheets {
		at, seen := index[sheet.SpreadsheetID]
		if !seen {
			at = len(out)
			index[sheet.SpreadsheetID] = at
			out = append(out, Spreadsheet{ID: sheet.SpreadsheetID})
		}
		out[at].Sheets = append(out[at].Sheets, sheet)
	}
	return out
}

func SpreadsheetURL(spreadsheetID string) string {
	return "https://docs.google.com/spreadsheets/d/" + spreadsheetID + "/edit"
}

func (s Spreadsheet) URL() string {
	return SpreadsheetURL(s.ID)
}

func (s Spreadsheet) Oldest() *Sheet {
	var oldest *Sheet
	for _, sheet := range s.Sheets {
		if oldest == nil || sheet.CreatedAt.Before(oldest.CreatedAt) {
			oldest = sheet
		}
	}
	return oldest
}

func (s Spreadsheet) Title() string {
	if s.Name != "" {
		return s.Name
	}
	if oldest := s.Oldest(); oldest != nil {
		return oldest.Title
	}
	return ""
}

func (s Spreadsheet) Status() Status {
	for _, sheet := range s.Sheets {
		if sheet.Status != StatusActive {
			return StatusNeedsAttention
		}
	}
	return StatusActive
}

func (s Spreadsheet) LastWrittenAt() *time.Time {
	var latest *time.Time
	for _, sheet := range s.Sheets {
		if sheet.LastWrittenAt != nil && (latest == nil || sheet.LastWrittenAt.After(*latest)) {
			latest = sheet.LastWrittenAt
		}
	}
	return latest
}
