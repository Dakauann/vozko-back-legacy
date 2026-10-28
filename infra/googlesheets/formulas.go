package googlesheets

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"vozko/domain/googlesheet"
)

const linkCellFields = "sheets(data(startRow,startColumn,rowData(values(userEnteredValue,effectiveValue(errorValue(type))))))"

type gridCell struct {
	UserEnteredValue struct {
		StringValue  string `json:"stringValue"`
		FormulaValue string `json:"formulaValue"`
	} `json:"userEnteredValue"`
	EffectiveValue struct {
		ErrorValue struct {
			Type string `json:"type"`
		} `json:"errorValue"`
	} `json:"effectiveValue"`
}

type gridData struct {
	StartRow    int `json:"startRow"`
	StartColumn int `json:"startColumn"`
	RowData     []struct {
		Values []gridCell `json:"values"`
	} `json:"rowData"`
}

func (d gridData) cell(i int) (gridCell, bool) {
	if len(d.RowData[i].Values) == 0 {
		return gridCell{}, false
	}
	return d.RowData[i].Values[0], true
}

func (c *Client) LinkCells(ctx context.Context, token string, target googlesheet.Target, columns []int) ([]googlesheet.LinkCell, error) {
	if len(columns) == 0 {
		return nil, nil
	}
	query := url.Values{"fields": {linkCellFields}}
	for _, col := range append([]int{0}, columns...) {
		query.Add("ranges", quotedTab(target.TabName)+"!"+googlesheet.ColumnBodyRange(col))
	}
	endpoint := c.endpoints.Sheets + "/" + url.PathEscape(target.SpreadsheetID) + "?" + query.Encode()
	var out struct {
		Sheets []struct {
			Data []gridData `json:"data"`
		} `json:"sheets"`
	}
	if err := c.do(ctx, http.MethodGet, endpoint, token, nil, &out); err != nil {
		return nil, err
	}
	if len(out.Sheets) != 1 || len(out.Sheets[0].Data) != len(columns)+1 {
		return nil, fmt.Errorf("googlesheets: asked for the ref and %d link columns, google answered another shape", len(columns))
	}
	data := out.Sheets[0].Data
	refs := map[int]string{}
	for i := range data[0].RowData {
		if cell, ok := data[0].cell(i); ok {
			refs[data[0].StartRow+i+1] = cell.UserEnteredValue.StringValue
		}
	}
	var cells []googlesheet.LinkCell
	for k, col := range columns {
		grid := data[k+1]
		if grid.StartColumn != col {
			return nil, fmt.Errorf("googlesheets: asked for column %d, google answered column %d", col, grid.StartColumn)
		}
		for i := range grid.RowData {
			cell, ok := grid.cell(i)
			if !ok || cell.UserEnteredValue.FormulaValue == "" {
				continue
			}
			row := grid.StartRow + i + 1
			cells = append(cells, googlesheet.LinkCell{
				Row: row, Column: col, RowRef: refs[row],
				Formula: googlesheet.Formula(cell.UserEnteredValue.FormulaValue),
				Failed:  cell.EffectiveValue.ErrorValue.Type != "",
			})
		}
	}
	return cells, nil
}

func (c *Client) WriteFormulas(ctx context.Context, token string, target googlesheet.Target, cells []googlesheet.FormulaCell) error {
	requests := make([]map[string]any, 0, len(cells))
	for _, cell := range cells {
		if cell.Row-1 < firstBodyIndex {
			return fmt.Errorf("googlesheets: formulas go below the header, not at row %d", cell.Row)
		}
		requests = append(requests, map[string]any{"updateCells": map[string]any{
			"start":  map[string]any{"sheetId": target.TabID, "rowIndex": cell.Row - 1, "columnIndex": cell.Column},
			"rows":   []map[string]any{{"values": []map[string]any{{"userEnteredValue": map[string]any{"formulaValue": string(cell.Formula)}}}}},
			"fields": "userEnteredValue",
		}})
	}
	return c.batchUpdate(ctx, token, target.SpreadsheetID, requests)
}
