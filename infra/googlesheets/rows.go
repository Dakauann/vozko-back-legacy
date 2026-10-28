package googlesheets

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"vozko/domain/googlesheet"
)

const (
	firstBodyIndex    = 1
	layoutStateFields = "sheets(properties(sheetId,title,gridProperties),conditionalFormats,bandedRanges,protectedRanges(protectedRangeId,description,range))"
)

func (c *Client) ReadRanges(ctx context.Context, token string, target googlesheet.Target, ranges []string) ([][][]any, error) {
	query := url.Values{"majorDimension": {"ROWS"}, "valueRenderOption": {"UNFORMATTED_VALUE"}}
	for _, a1 := range ranges {
		query.Add("ranges", quotedTab(target.TabName)+"!"+a1)
	}
	endpoint := c.endpoints.Sheets + "/" + url.PathEscape(target.SpreadsheetID) + "/values:batchGet?" + query.Encode()
	var out struct {
		ValueRanges []struct {
			Values [][]any `json:"values"`
		} `json:"valueRanges"`
	}
	if err := c.do(ctx, http.MethodGet, endpoint, token, nil, &out); err != nil {
		return nil, err
	}
	if len(out.ValueRanges) != len(ranges) {
		return nil, fmt.Errorf("googlesheets: asked for %d ranges, google answered %d", len(ranges), len(out.ValueRanges))
	}
	read := make([][][]any, len(ranges))
	for i, r := range out.ValueRanges {
		read[i] = r.Values
	}
	return read, nil
}

func (c *Client) InsertRows(ctx context.Context, token string, target googlesheet.Target, at int, rows [][]any) error {
	start := at - 1
	if start < firstBodyIndex {
		return fmt.Errorf("googlesheets: rows go below the header, not at row %d", at)
	}
	cells, err := enteredRows(rows)
	if err != nil {
		return err
	}
	return c.batchUpdate(ctx, token, target.SpreadsheetID, []map[string]any{
		{"insertDimension": map[string]any{
			"range":             map[string]any{"sheetId": target.TabID, "dimension": "ROWS", "startIndex": start, "endIndex": start + len(rows)},
			"inheritFromBefore": start > firstBodyIndex,
		}},
		{"updateCells": map[string]any{
			"start":  map[string]any{"sheetId": target.TabID, "rowIndex": start, "columnIndex": 0},
			"rows":   cells,
			"fields": "userEnteredValue",
		}},
	})
}

func enteredRows(rows [][]any) ([]map[string]any, error) {
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		values := make([]map[string]any, len(row))
		for j, cell := range row {
			value, err := enteredValue(cell)
			if err != nil {
				return nil, fmt.Errorf("googlesheets: row %d cell %d: %w", i+1, j+1, err)
			}
			values[j] = map[string]any{}
			if value != nil {
				values[j]["userEnteredValue"] = value
			}
		}
		out[i] = map[string]any{"values": values}
	}
	return out, nil
}

func enteredValue(cell any) (map[string]any, error) {
	switch v := cell.(type) {
	case googlesheet.Formula:
		return map[string]any{"formulaValue": string(v)}, nil
	case string:
		if v == "" {
			return nil, nil
		}
		return map[string]any{"stringValue": v}, nil
	case bool:
		return map[string]any{"boolValue": v}, nil
	case float64:
		return map[string]any{"numberValue": v}, nil
	case int:
		return map[string]any{"numberValue": v}, nil
	case int64:
		return map[string]any{"numberValue": v}, nil
	case nil:
		return nil, nil
	}
	return nil, fmt.Errorf("a %T is not a cell value", cell)
}

func (c *Client) RepairHeader(ctx context.Context, token string, target googlesheet.Target, repair googlesheet.HeaderRepair, layout googlesheet.Layout) error {
	return c.relayout(ctx, token, target, layout, func(state tabState) ([]map[string]any, tabState) {
		return repairRequests(state, repair)
	})
}

func repairRequests(state tabState, repair googlesheet.HeaderRepair) ([]map[string]any, tabState) {
	switch {
	case repair.MoveFrom > 1:
		header := repair.MoveFrom - 1
		return []map[string]any{
			{"moveDimension": map[string]any{
				"source":           map[string]any{"sheetId": state.tabID, "dimension": "ROWS", "startIndex": header, "endIndex": header + 1},
				"destinationIndex": 0,
			}},
			{"repeatCell": map[string]any{
				"range":  gridRange(state.tabID, firstBodyIndex, repair.MoveFrom, 0, 0),
				"cell":   map[string]any{"userEnteredFormat": map[string]any{"textFormat": map[string]any{}}},
				"fields": "userEnteredFormat.textFormat",
			}},
		}, state
	case repair.Insert:
		state.rows++
		return []map[string]any{{"insertDimension": map[string]any{
			"range":             map[string]any{"sheetId": state.tabID, "dimension": "ROWS", "startIndex": 0, "endIndex": 1},
			"inheritFromBefore": false,
		}}}, state
	}
	return nil, state
}
