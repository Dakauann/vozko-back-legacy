package googlesheets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"vozko/domain/googlesheet"
)

const (
	requestTimeout  = 30 * time.Second
	maxResponseBody = 32 << 20
)

type Endpoints struct {
	Sheets string
	Drive  string
}

var GoogleEndpoints = Endpoints{
	Sheets: "https://sheets.googleapis.com/v4/spreadsheets",
	Drive:  "https://www.googleapis.com/drive/v3/files",
}

type Client struct {
	endpoints Endpoints
	http      *http.Client
}

var _ googlesheet.SheetsAPI = (*Client)(nil)

func New(endpoints Endpoints) *Client {
	if endpoints == (Endpoints{}) {
		endpoints = GoogleEndpoints
	}
	return &Client{endpoints: endpoints, http: &http.Client{Timeout: requestTimeout}}
}

type gridProperties struct {
	RowCount       int `json:"rowCount,omitempty"`
	ColumnCount    int `json:"columnCount,omitempty"`
	FrozenRowCount int `json:"frozenRowCount,omitempty"`
}

type sheetProperties struct {
	SheetID        int64          `json:"sheetId"`
	Title          string         `json:"title"`
	GridProperties gridProperties `json:"gridProperties"`
}

type sheetBody struct {
	Properties         sheetProperties        `json:"properties"`
	ConditionalFormats []conditionalRuleState `json:"conditionalFormats,omitempty"`
	BandedRanges       []bandedRangeState     `json:"bandedRanges,omitempty"`
	ProtectedRanges    []protectedRangeState  `json:"protectedRanges,omitempty"`
}

type spreadsheetBody struct {
	SpreadsheetID string `json:"spreadsheetId,omitempty"`
	Properties    struct {
		Title    string `json:"title"`
		Locale   string `json:"locale,omitempty"`
		TimeZone string `json:"timeZone,omitempty"`
	} `json:"properties"`
	Sheets []sheetBody `json:"sheets"`
}

func newTabProperties(id int64, title string, layout googlesheet.Layout) sheetProperties {
	return sheetProperties{SheetID: id, Title: title, GridProperties: gridProperties{
		RowCount: googlesheet.NewTabRows, ColumnCount: len(layout.Columns) + refColumns, FrozenRowCount: 1,
	}}
}

func freshTab(id int64, layout googlesheet.Layout) tabState {
	return tabState{tabID: id, rows: googlesheet.NewTabRows, columns: len(layout.Columns) + refColumns}
}

func (c *Client) CreateSpreadsheet(ctx context.Context, token string, spec googlesheet.SpreadsheetSpec) (googlesheet.CreatedSpreadsheet, error) {
	var body spreadsheetBody
	body.Properties.Title = spec.Title
	body.Properties.Locale = spec.Locale
	body.Properties.TimeZone = spec.TimeZone
	props := newTabProperties(0, spec.TabName, spec.Layout)
	body.Sheets = []sheetBody{{Properties: sheetProperties{Title: props.Title, GridProperties: props.GridProperties}}}
	var created spreadsheetBody
	if err := c.do(ctx, http.MethodPost, c.endpoints.Sheets, token, body, &created); err != nil {
		return googlesheet.CreatedSpreadsheet{}, err
	}
	if created.SpreadsheetID == "" || len(created.Sheets) == 0 {
		return googlesheet.CreatedSpreadsheet{}, fmt.Errorf("googlesheets: create returned no spreadsheet")
	}
	out := googlesheet.CreatedSpreadsheet{SpreadsheetID: created.SpreadsheetID, TabID: created.Sheets[0].Properties.SheetID}
	if err := c.batchUpdate(ctx, token, out.SpreadsheetID, layoutRequests(freshTab(out.TabID, spec.Layout), spec.Layout)); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) TrashSpreadsheet(ctx context.Context, token, spreadsheetID string) error {
	return c.do(ctx, http.MethodPatch, c.endpoints.Drive+"/"+url.PathEscape(spreadsheetID), token, map[string]any{"trashed": true}, nil)
}

func (c *Client) AddTab(ctx context.Context, token, spreadsheetID string, tab googlesheet.TabSpec) error {
	requests := []map[string]any{{"addSheet": map[string]any{"properties": newTabProperties(tab.ID, tab.Title, tab.Layout)}}}
	requests = append(requests, layoutRequests(freshTab(tab.ID, tab.Layout), tab.Layout)...)
	return c.batchUpdate(ctx, token, spreadsheetID, requests)
}

func (c *Client) RenameSpreadsheet(ctx context.Context, token, spreadsheetID, title string) error {
	return c.batchUpdate(ctx, token, spreadsheetID, []map[string]any{{"updateSpreadsheetProperties": map[string]any{"properties": map[string]any{"title": title}, "fields": "title"}}})
}

func (c *Client) DeleteTab(ctx context.Context, token, spreadsheetID string, tabID int64) error {
	return c.batchUpdate(ctx, token, spreadsheetID, []map[string]any{{"deleteSheet": map[string]any{"sheetId": tabID}}})
}

func (c *Client) ApplyLayout(ctx context.Context, token string, target googlesheet.Target, layout googlesheet.Layout) error {
	return c.relayout(ctx, token, target, layout, nil)
}

func (c *Client) relayout(ctx context.Context, token string, target googlesheet.Target, layout googlesheet.Layout, before func(tabState) ([]map[string]any, tabState)) error {
	sheets, err := c.sheets(ctx, token, target.SpreadsheetID, layoutStateFields)
	if err != nil {
		return err
	}
	for _, sheet := range sheets {
		if sheet.Properties.SheetID != target.TabID {
			continue
		}
		state := tabState{
			tabID: target.TabID, rows: sheet.Properties.GridProperties.RowCount, columns: sheet.Properties.GridProperties.ColumnCount,
			rules: sheet.ConditionalFormats, bandings: sheet.BandedRanges, protections: sheet.ProtectedRanges,
		}
		var requests []map[string]any
		if before != nil {
			requests, state = before(state)
		}
		return c.batchUpdate(ctx, token, target.SpreadsheetID, append(requests, layoutRequests(state, layout)...))
	}
	return tabMissing()
}

func (c *Client) Workbook(ctx context.Context, token, spreadsheetID string) (googlesheet.Workbook, error) {
	body, err := c.spreadsheet(ctx, token, spreadsheetID, "properties.locale,sheets.properties(sheetId,title,gridProperties)")
	if err != nil {
		return googlesheet.Workbook{}, err
	}
	wb := googlesheet.Workbook{Locale: body.Properties.Locale, Tabs: make([]googlesheet.TabInfo, 0, len(body.Sheets))}
	for _, sheet := range body.Sheets {
		p := sheet.Properties
		wb.Tabs = append(wb.Tabs, googlesheet.TabInfo{ID: p.SheetID, Title: p.Title, Rows: p.GridProperties.RowCount, Columns: p.GridProperties.ColumnCount})
	}
	return wb, nil
}

func (c *Client) sheets(ctx context.Context, token, spreadsheetID, fields string) ([]sheetBody, error) {
	body, err := c.spreadsheet(ctx, token, spreadsheetID, fields)
	return body.Sheets, err
}

func (c *Client) spreadsheet(ctx context.Context, token, spreadsheetID, fields string) (spreadsheetBody, error) {
	endpoint := c.endpoints.Sheets + "/" + url.PathEscape(spreadsheetID) + "?" + url.Values{"fields": {fields}}.Encode()
	var out spreadsheetBody
	if err := c.do(ctx, http.MethodGet, endpoint, token, nil, &out); err != nil {
		return spreadsheetBody{}, err
	}
	return out, nil
}

func tabMissing() error {
	return &APIError{Status: http.StatusNotFound, Reason: "tab_missing", Message: "a aba da planilha foi removida"}
}

func (c *Client) ReadRange(ctx context.Context, token string, target googlesheet.Target, a1 string) ([][]any, error) {
	read, err := c.ReadRanges(ctx, token, target, []string{a1})
	if err != nil {
		return nil, err
	}
	return read[0], nil
}

func (c *Client) batchUpdate(ctx context.Context, token, spreadsheetID string, requests []map[string]any) error {
	if len(requests) == 0 {
		return nil
	}
	endpoint := c.endpoints.Sheets + "/" + url.PathEscape(spreadsheetID) + ":batchUpdate"
	return c.do(ctx, http.MethodPost, endpoint, token, map[string]any{"requests": requests}, nil)
}

func quotedTab(name string) string {
	return "'" + strings.ReplaceAll(name, "'", "''") + "'"
}

func (c *Client) do(ctx context.Context, method, endpoint, token string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("googlesheets: encode request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return &TransportError{Err: err}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBody))
	if err != nil {
		return &TransportError{Err: err}
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return newAPIError(res, raw)
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("googlesheets: decode %s: %w", strings.SplitN(endpoint, "?", 2)[0], err)
	}
	return nil
}
