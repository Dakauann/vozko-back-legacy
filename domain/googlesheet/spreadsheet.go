package googlesheet

import "time"

type Spreadsheet struct {
	ID     string
	Sheets []*Sheet
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
