package googlesheets

import (
	"sort"
	"strconv"
	"strings"

	"vozko/domain/googlesheet"
)

var numberFormats = map[googlesheet.ColumnType]map[string]any{
	googlesheet.TypeNumber:   {"type": "NUMBER", "pattern": "#,##0.##########"},
	googlesheet.TypeMoney:    {"type": "CURRENCY", "pattern": "\"R$\" #,##0.00"},
	googlesheet.TypeDate:     {"type": "DATE", "pattern": "dd/mm/yyyy"},
	googlesheet.TypeDateTime: {"type": "DATE_TIME", "pattern": "dd/mm/yyyy hh:mm"},
}

var columnWidths = map[googlesheet.ColumnType]int{
	googlesheet.TypeText:     180,
	googlesheet.TypeLongText: 360,
	googlesheet.TypeNumber:   110,
	googlesheet.TypeMoney:    120,
	googlesheet.TypeDate:     110,
	googlesheet.TypeDateTime: 150,
	googlesheet.TypeBoolean:  90,
	googlesheet.TypeChoice:   140,
	googlesheet.TypeEmail:    220,
	googlesheet.TypePhone:    150,
	googlesheet.TypeURL:      240,
	googlesheet.TypeLink:     150,
}

const (
	white            = "#FFFFFF"
	headerInk        = "#202124"
	refColumns       = 1
	headerProtection = "Cabeçalho usado pelas gravações automáticas"
)

type gridRangeState struct {
	SheetID          int64 `json:"sheetId"`
	StartRowIndex    *int  `json:"startRowIndex"`
	EndRowIndex      *int  `json:"endRowIndex"`
	StartColumnIndex *int  `json:"startColumnIndex"`
	EndColumnIndex   *int  `json:"endColumnIndex"`
}

type conditionalRuleState struct {
	Ranges      []gridRangeState `json:"ranges"`
	BooleanRule *struct {
		Condition struct {
			Type   string `json:"type"`
			Values []any  `json:"values"`
		} `json:"condition"`
	} `json:"booleanRule"`
}

type bandedRangeState struct {
	BandedRangeID int64          `json:"bandedRangeId"`
	Range         gridRangeState `json:"range"`
}

type protectedRangeState struct {
	ProtectedRangeID int64          `json:"protectedRangeId"`
	Description      string         `json:"description"`
	Range            gridRangeState `json:"range"`
}

type tabState struct {
	tabID       int64
	rows        int
	columns     int
	rules       []conditionalRuleState
	bandings    []bandedRangeState
	protections []protectedRangeState
}

func zeroIfNil(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func (s tabState) ownsRule(rule conditionalRuleState) bool {
	if rule.BooleanRule == nil || rule.BooleanRule.Condition.Type != "TEXT_EQ" || len(rule.BooleanRule.Condition.Values) != 1 || len(rule.Ranges) != 1 {
		return false
	}
	r := rule.Ranges[0]
	start, end := zeroIfNil(r.StartColumnIndex), zeroIfNil(r.EndColumnIndex)
	return r.SheetID == s.tabID && start >= refColumns && end == start+1
}

func (s tabState) ownsBanding(b bandedRangeState) bool {
	r := b.Range
	return r.SheetID == s.tabID && zeroIfNil(r.StartColumnIndex) == 0
}

func (s tabState) ownsProtection(p protectedRangeState) bool {
	return p.Range.SheetID == s.tabID && p.Description == headerProtection
}

func layoutRequests(state tabState, layout googlesheet.Layout) []map[string]any {
	tabID := state.tabID
	width := len(layout.Columns) + refColumns
	var requests []map[string]any
	if missing := width - state.columns; missing > 0 {
		requests = append(requests, map[string]any{"appendDimension": map[string]any{
			"sheetId": tabID, "dimension": "COLUMNS", "length": missing,
		}})
	}
	var owned []int
	for i, rule := range state.rules {
		if state.ownsRule(rule) {
			owned = append(owned, i)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(owned)))
	for _, index := range owned {
		requests = append(requests, map[string]any{"deleteConditionalFormatRule": map[string]any{"sheetId": tabID, "index": index}})
	}
	for _, banding := range state.bandings {
		if state.ownsBanding(banding) {
			requests = append(requests, map[string]any{"deleteBanding": map[string]any{"bandedRangeId": banding.BandedRangeID}})
		}
	}
	for _, protection := range state.protections {
		if state.ownsProtection(protection) {
			requests = append(requests, map[string]any{"deleteProtectedRange": map[string]any{"protectedRangeId": protection.ProtectedRangeID}})
		}
	}
	palette := layout.Style.Palette()
	requests = append(requests, headerRequests(tabID, layout.Columns, palette)...)
	for i, col := range layout.Columns {
		requests = append(requests, columnRequests(tabID, i+refColumns, col, i >= layout.SizeFrom)...)
	}
	if palette.Header != "" {
		requests = append(requests, map[string]any{"addBanding": map[string]any{"bandedRange": map[string]any{
			"range": gridRange(tabID, 0, 0, 0, width),
			"rowProperties": map[string]any{
				"headerColorStyle":     colorStyle(palette.Header),
				"firstBandColorStyle":  colorStyle(white),
				"secondBandColorStyle": colorStyle(palette.Band),
			},
		}}})
	}
	for i, col := range layout.Columns {
		requests = append(requests, optionColorRules(tabID, i+refColumns, col)...)
	}
	return append(requests, protectHeader(tabID))
}

func headerRequests(tabID int64, columns []googlesheet.Column, palette googlesheet.Palette) []map[string]any {
	ink := headerInk
	if palette.HeaderText != "" {
		ink = palette.HeaderText
	}
	header := []map[string]any{headerCell(googlesheet.RefHeader, ink)}
	for _, col := range columns {
		header = append(header, headerCell(col.Header, ink))
	}
	return []map[string]any{
		{"updateCells": map[string]any{
			"range":  gridRange(tabID, 0, 1, 0, len(columns)+refColumns),
			"rows":   []map[string]any{{"values": header}},
			"fields": "userEnteredValue,userEnteredFormat.textFormat",
		}},
		{"updateDimensionProperties": map[string]any{
			"range":      map[string]any{"sheetId": tabID, "dimension": "COLUMNS", "startIndex": 0, "endIndex": refColumns},
			"properties": map[string]any{"hiddenByUser": true},
			"fields":     "hiddenByUser",
		}},
		{"updateSheetProperties": map[string]any{
			"properties": map[string]any{"sheetId": tabID, "gridProperties": map[string]any{"frozenRowCount": 1}},
			"fields":     "gridProperties.frozenRowCount",
		}},
	}
}

func columnRequests(tabID int64, index int, col googlesheet.Column, size bool) []map[string]any {
	body := gridRange(tabID, 1, 0, index, index+1)
	var requests []map[string]any
	if format, ok := numberFormats[col.Type]; ok {
		requests = append(requests, map[string]any{"repeatCell": map[string]any{
			"range":  body,
			"cell":   map[string]any{"userEnteredFormat": map[string]any{"numberFormat": format}},
			"fields": "userEnteredFormat.numberFormat",
		}})
	}
	if col.Type == googlesheet.TypeLongText {
		requests = append(requests, map[string]any{"repeatCell": map[string]any{
			"range":  body,
			"cell":   map[string]any{"userEnteredFormat": map[string]any{"wrapStrategy": "WRAP"}},
			"fields": "userEnteredFormat.wrapStrategy",
		}})
	}
	if rule := validationRule(col); rule != nil {
		requests = append(requests, map[string]any{"setDataValidation": map[string]any{"range": body, "rule": rule}})
	}
	if width, ok := columnWidths[col.Type]; ok && size {
		requests = append(requests, map[string]any{"updateDimensionProperties": map[string]any{
			"range":      map[string]any{"sheetId": tabID, "dimension": "COLUMNS", "startIndex": index, "endIndex": index + 1},
			"properties": map[string]any{"pixelSize": width},
			"fields":     "pixelSize",
		}})
	}
	return requests
}

func optionColorRules(tabID int64, index int, col googlesheet.Column) []map[string]any {
	if col.Archived || col.Type != googlesheet.TypeChoice {
		return nil
	}
	var requests []map[string]any
	for _, option := range col.Options {
		color, ok := col.Colors[option]
		if !ok {
			continue
		}
		fill, text := color.Hex()
		requests = append(requests, map[string]any{"addConditionalFormatRule": map[string]any{
			"index": 0,
			"rule": map[string]any{
				"ranges": []map[string]any{gridRange(tabID, 0, 0, index, index+1)},
				"booleanRule": map[string]any{
					"condition": map[string]any{"type": "TEXT_EQ", "values": []map[string]any{{"userEnteredValue": option}}},
					"format": map[string]any{
						"backgroundColorStyle": colorStyle(fill),
						"textFormat":           map[string]any{"bold": true, "foregroundColorStyle": colorStyle(text)},
					},
				},
			},
		}})
	}
	return requests
}

func validationRule(col googlesheet.Column) map[string]any {
	switch {
	case col.Archived:
		return nil
	case col.Type == googlesheet.TypeBoolean:
		return map[string]any{"condition": map[string]any{"type": "BOOLEAN"}, "strict": true}
	case col.Type == googlesheet.TypeChoice:
		values := make([]map[string]any, len(col.Options))
		for i, option := range col.Options {
			values[i] = map[string]any{"userEnteredValue": option}
		}
		return map[string]any{"condition": map[string]any{"type": "ONE_OF_LIST", "values": values}, "strict": true, "showCustomUi": true}
	}
	return nil
}

func protectHeader(tabID int64) map[string]any {
	return map[string]any{"addProtectedRange": map[string]any{"protectedRange": map[string]any{
		"range":       gridRange(tabID, 0, 1, 0, 0),
		"description": headerProtection,
		"warningOnly": true,
	}}}
}

func headerCell(text, ink string) map[string]any {
	return map[string]any{
		"userEnteredValue":  map[string]any{"stringValue": text},
		"userEnteredFormat": map[string]any{"textFormat": map[string]any{"bold": true, "foregroundColorStyle": colorStyle(ink)}},
	}
}

func colorStyle(hex string) map[string]any {
	return map[string]any{"rgbColor": rgb(hex)}
}

func rgb(hex string) map[string]any {
	value, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		return map[string]any{"red": 0, "green": 0, "blue": 0}
	}
	channel := func(shift uint) float64 { return float64((value>>shift)&0xff) / 255 }
	return map[string]any{"red": channel(16), "green": channel(8), "blue": channel(0)}
}

func gridRange(tabID int64, startRow, endRow, startColumn, endColumn int) map[string]any {
	r := map[string]any{"sheetId": tabID, "startRowIndex": startRow}
	if endRow > 0 {
		r["endRowIndex"] = endRow
	}
	if endColumn > 0 {
		r["startColumnIndex"] = startColumn
		r["endColumnIndex"] = endColumn
	}
	return r
}
