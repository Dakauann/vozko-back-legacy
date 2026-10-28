package googlesheets

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"vozko/domain/googlesheet"
)

type fakeTab struct {
	id    int64
	title string
	rows  [][]any
	grid  int
}

type googleGrid struct {
	mu       sync.Mutex
	tabs     map[string]*fakeTab
	batches  []map[string]any
	paths    []string
	refusals int
}

func newGoogleGrid(t *testing.T, tabs ...*fakeTab) (*Client, *googleGrid) {
	t.Helper()
	g := &googleGrid{tabs: map[string]*fakeTab{}}
	for _, tab := range tabs {
		g.tabs[tab.title] = tab
	}
	server := httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(server.Close)
	return New(Endpoints{Sheets: server.URL + "/v4/spreadsheets", Drive: server.URL + "/drive/v3/files"}), g
}

func (g *googleGrid) tab(id int64) *fakeTab {
	for _, tab := range g.tabs {
		if tab.id == id {
			return tab
		}
	}
	return nil
}

func (g *googleGrid) serve(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	path, _ := url.PathUnescape(r.URL.EscapedPath())
	g.paths = append(g.paths, r.Method+" "+path)
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/values:batchGet"):
		var ranges []map[string]any
		for _, a1 := range r.URL.Query()["ranges"] {
			values, ok := g.read(a1)
			if !ok {
				g.refuse(w, "Unable to parse range: "+a1)
				return
			}
			ranges = append(ranges, map[string]any{"range": a1, "values": values})
		}
		g.reply(w, map[string]any{"valueRanges": ranges})
	case r.Method == http.MethodGet && strings.Contains(path, "/values/"):
		a1 := path[strings.Index(path, "/values/")+len("/values/"):]
		values, ok := g.read(a1)
		if !ok {
			g.refuse(w, "Unable to parse range: "+a1)
			return
		}
		g.reply(w, map[string]any{"values": values})
	case r.Method == http.MethodGet:
		var sheets []map[string]any
		for _, tab := range g.tabs {
			sheets = append(sheets, map[string]any{"properties": map[string]any{
				"sheetId": tab.id, "title": tab.title, "gridProperties": map[string]any{"rowCount": tab.grid, "columnCount": 12},
			}})
		}
		g.reply(w, map[string]any{"sheets": sheets})
	case r.Method == http.MethodPost && strings.HasSuffix(path, ":batchUpdate"):
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Requests []map[string]any `json:"requests"`
		}
		_ = json.Unmarshal(raw, &body)
		var whole map[string]any
		_ = json.Unmarshal(raw, &whole)
		g.batches = append(g.batches, whole)
		if err := g.apply(body.Requests); err != nil {
			g.refuse(w, err.Error())
			return
		}
		g.reply(w, map[string]any{})
	default:
		g.refuse(w, "the fake does not serve "+r.Method+" "+path)
	}
}

func (g *googleGrid) reply(w http.ResponseWriter, body any) {
	raw, _ := json.Marshal(body)
	_, _ = w.Write(raw)
}

func (g *googleGrid) refuse(w http.ResponseWriter, message string) {
	g.refusals++
	w.WriteHeader(http.StatusBadRequest)
	raw, _ := json.Marshal(map[string]any{"error": map[string]any{"code": 400, "message": message, "status": "INVALID_ARGUMENT"}})
	_, _ = w.Write(raw)
}

var (
	rowRangePattern = regexp.MustCompile(`^(\d+):(\d+)$`)
	quotedTabName   = regexp.MustCompile(`^'((?:[^']|'')*)'!(.+)$`)
)

func (g *googleGrid) read(a1 string) ([][]any, bool) {
	match := quotedTabName.FindStringSubmatch(a1)
	if match == nil {
		return nil, false
	}
	tab, ok := g.tabs[strings.ReplaceAll(match[1], "''", "'")]
	if !ok {
		return nil, false
	}
	if match[2] == "A:A" {
		column := make([][]any, len(tab.rows))
		for i, row := range tab.rows {
			column[i] = []any{}
			if len(row) > 0 && row[0] != nil && row[0] != "" {
				column[i] = []any{row[0]}
			}
		}
		return trimmed(column), true
	}
	rows := rowRangePattern.FindStringSubmatch(match[2])
	if rows == nil {
		return nil, false
	}
	from, _ := strconv.Atoi(rows[1])
	to, _ := strconv.Atoi(rows[2])
	var out [][]any
	for i := from; i <= to && i <= len(tab.rows); i++ {
		out = append(out, trimmedCells(tab.rows[i-1]))
	}
	return trimmed(out), true
}

func trimmedCells(row []any) []any {
	end := len(row)
	for end > 0 && (row[end-1] == nil || row[end-1] == "") {
		end--
	}
	return append([]any{}, row[:end]...)
}

func trimmed(rows [][]any) [][]any {
	end := len(rows)
	for end > 0 && len(trimmedCells(rows[end-1])) == 0 {
		end--
	}
	return rows[:end]
}

func number(v any) int {
	f, _ := v.(float64)
	return int(f)
}

func (g *googleGrid) apply(requests []map[string]any) error {
	type change func()
	var changes []change
	staged := map[int64][][]any{}
	grids := map[int64]int{}
	rowsOf := func(id int64) ([][]any, error) {
		if rows, ok := staged[id]; ok {
			return rows, nil
		}
		tab := g.tab(id)
		if tab == nil {
			return nil, fmt.Errorf("No grid with id: %d", id)
		}
		staged[id] = append([][]any{}, tab.rows...)
		grids[id] = tab.grid
		return staged[id], nil
	}
	for _, request := range requests {
		for kind, raw := range request {
			body, _ := raw.(map[string]any)
			switch kind {
			case "insertDimension":
				r := body["range"].(map[string]any)
				id := int64(number(r["sheetId"]))
				rows, err := rowsOf(id)
				if err != nil {
					return err
				}
				start, end := number(r["startIndex"]), number(r["endIndex"])
				if r["dimension"] != "ROWS" || start < 0 || start > grids[id] || end <= start {
					return fmt.Errorf("Invalid insertDimension %v", r)
				}
				if before, _ := body["inheritFromBefore"].(bool); before && start == 0 {
					return fmt.Errorf("inheritFromBefore needs a row before index 0")
				}
				for len(rows) < start {
					rows = append(rows, nil)
				}
				blank := make([][]any, end-start)
				rows = append(rows[:start], append(blank, rows[start:]...)...)
				staged[id], grids[id] = rows, grids[id]+end-start
			case "deleteDimension":
				r := body["range"].(map[string]any)
				id := int64(number(r["sheetId"]))
				rows, err := rowsOf(id)
				if err != nil {
					return err
				}
				start, end := number(r["startIndex"]), number(r["endIndex"])
				if r["dimension"] != "ROWS" || start < 0 || end <= start || end > grids[id] {
					return fmt.Errorf("Invalid deleteDimension %v", r)
				}
				if start < len(rows) {
					rows = append(append([][]any{}, rows[:start]...), rows[min(end, len(rows)):]...)
				}
				staged[id], grids[id] = rows, grids[id]-(end-start)
			case "moveDimension":
				src := body["source"].(map[string]any)
				id := int64(number(src["sheetId"]))
				rows, err := rowsOf(id)
				if err != nil {
					return err
				}
				start, end, dest := number(src["startIndex"]), number(src["endIndex"]), number(body["destinationIndex"])
				if src["dimension"] != "ROWS" || end > len(rows) || start >= end || dest > start {
					return fmt.Errorf("Invalid moveDimension %v", body)
				}
				moved := append([][]any{}, rows[start:end]...)
				rest := append(append([][]any{}, rows[:start]...), rows[end:]...)
				staged[id] = append(append(append([][]any{}, rest[:dest]...), moved...), rest[dest:]...)
			case "updateCells":
				start, ok := body["start"].(map[string]any)
				at := number(start["rowIndex"])
				if !ok {
					start, _ = body["range"].(map[string]any)
					at = number(start["startRowIndex"])
				}
				id := int64(number(start["sheetId"]))
				rows, err := rowsOf(id)
				if err != nil {
					return err
				}
				written, _ := body["rows"].([]any)
				if at+len(written) > grids[id] {
					return fmt.Errorf("Range exceeds grid limits. Max rows: %d", grids[id])
				}
				for len(rows) < at+len(written) {
					rows = append(rows, nil)
				}
				for i, raw := range written {
					values, _ := raw.(map[string]any)["values"].([]any)
					cells := make([]any, len(values))
					for j, cell := range values {
						cells[j] = enteredCell(cell)
					}
					rows[at+i] = cells
				}
				staged[id] = rows
			}
		}
	}
	for id, rows := range staged {
		tab := g.tab(id)
		rows, grid := rows, grids[id]
		changes = append(changes, func() { tab.rows, tab.grid = rows, grid })
	}
	for _, apply := range changes {
		apply()
	}
	return nil
}

func enteredCell(raw any) any {
	cell, _ := raw.(map[string]any)
	value, _ := cell["userEnteredValue"].(map[string]any)
	for kind, v := range value {
		if kind == "formulaValue" {
			return googlesheet.Formula(v.(string))
		}
		return v
	}
	return ""
}

func (g *googleGrid) column(title string) []any {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []any
	for _, row := range g.tabs[title].rows {
		if len(row) == 0 {
			out = append(out, "")
			continue
		}
		out = append(out, row[0])
	}
	return out
}
