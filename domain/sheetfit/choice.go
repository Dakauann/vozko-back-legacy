package sheetfit

import (
	"bytes"
	"encoding/json"
	"strings"
)

type Choice struct {
	SpreadsheetID string            `json:"spreadsheet_id"`
	Tabs          map[string]string `json:"tabs,omitempty"`
}

func ParseChoice(value string) (Choice, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Choice{}, refuse(CodeFileNotFound)
	}
	if !strings.HasPrefix(value, "{") {
		return Choice{SpreadsheetID: value}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(value)))
	decoder.DisallowUnknownFields()
	var raw Choice
	if decoder.Decode(&raw) != nil {
		return Choice{}, refuse(CodeFileNotFound)
	}
	out := Choice{SpreadsheetID: strings.TrimSpace(raw.SpreadsheetID)}
	if out.SpreadsheetID == "" {
		return Choice{}, refuse(CodeFileNotFound)
	}
	for model, sheet := range raw.Tabs {
		model, sheet = strings.TrimSpace(model), strings.TrimSpace(sheet)
		if model == "" || sheet == "" {
			return Choice{}, refuse(CodeFileNotFound)
		}
		if out.Tabs == nil {
			out.Tabs = map[string]string{}
		}
		out.Tabs[model] = sheet
	}
	return out, nil
}

func (c Choice) Value() string {
	if len(c.Tabs) == 0 {
		return c.SpreadsheetID
	}
	encoded, _ := json.Marshal(c)
	return string(encoded)
}

func (c Choice) With(modelSheetID, sheetID string) Choice {
	tabs := make(map[string]string, len(c.Tabs)+1)
	for model, sheet := range c.Tabs {
		tabs[model] = sheet
	}
	tabs[modelSheetID] = sheetID
	return Choice{SpreadsheetID: c.SpreadsheetID, Tabs: tabs}
}
