package googlesheets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"vozko/domain/googlesheet"
)

const (
	brokenRow  = "b8dabd6c-409a-437d-90f4-c0d8193e5cc3"
	writtenRow = "0f1e2d3c-4b5a-4968-8776-655443322110"
)

func TestWorkbookReadsTheLocaleOfTheFileInTheSameCall(t *testing.T) {
	client, calls := fakeGoogle(t, func(*http.Request) (int, string) {
		return 200, `{"properties":{"locale":"pt_BR"},"sheets":[{"properties":{"sheetId":0,"title":"Negociações","gridProperties":{"rowCount":500,"columnCount":9}}}]}`
	})
	wb, err := client.Workbook(context.Background(), "token", "sp-1")
	if err != nil || wb.Locale != "pt_BR" || len(wb.Tabs) != 1 {
		t.Fatalf("Workbook = %+v, %v", wb, err)
	}
	if len(*calls) != 1 {
		t.Fatalf("calls = %d, the locale must ride on the workbook read", len(*calls))
	}
	query, _ := url.ParseQuery((*calls)[0].query)
	if fields := query.Get("fields"); !strings.Contains(fields, "properties.locale") || !strings.Contains(fields, "sheets.properties(") {
		t.Fatalf("fields = %q", fields)
	}
}

func TestTheFormulaSentFollowsTheLocaleTheWorkbookRead(t *testing.T) {
	client, g := newGoogleGrid(t, paymentsTab())
	formula, err := googlesheet.LinkFormula(googlesheet.TabRef{ID: 0, Title: "Negociações"}, brokenRow, "Ver negociação", googlesheet.SeparatorSemicolon)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.InsertRows(context.Background(), "token", paymentsTarget(), 2, [][]any{{writtenRow, formula}}); err != nil {
		t.Fatal(err)
	}
	update := insertKinds(t, g)[1]["updateCells"].(map[string]any)
	values := update["rows"].([]any)[0].(map[string]any)["values"].([]any)
	sent := values[1].(map[string]any)["userEnteredValue"].(map[string]any)["formulaValue"]
	want := `=HYPERLINK("#gid=0&range=B"&MATCH("` + brokenRow + `";'Negociações'!$A:$A;0);"Ver negociação")`
	if sent != want {
		t.Fatalf("formulaValue = %v\nwant %s", sent, want)
	}
}

const linkGrid = `{"sheets":[{"data":[
	{"startRow":1,"rowData":[
		{"values":[{"userEnteredValue":{"stringValue":"` + writtenRow + `"}}]},
		{},
		{"values":[{"userEnteredValue":{"stringValue":"` + brokenRow + `"}}]}
	]},
	{"startRow":1,"startColumn":3,"rowData":[
		{"values":[{"userEnteredValue":{"formulaValue":"=HYPERLINK(\"#gid=0&range=B\"&MATCH(\"x\",'T'!$A:$A,0),\"Ver\")"},"effectiveValue":{"errorValue":{"type":"ERROR"}}}]},
		{"values":[{"userEnteredValue":{"stringValue":"texto"}}]},
		{"values":[{"userEnteredValue":{"formulaValue":"=HYPERLINK(\"#gid=0&range=B\"&MATCH(\"y\";'T'!$A:$A;0);\"Ver\")"},"effectiveValue":{"stringValue":"Ver"}}]}
	]}
]}]}`

func TestLinkCellsReadsTheRefAndEveryLinkFormulaWithItsErrorInOneCall(t *testing.T) {
	client, calls := fakeGoogle(t, func(*http.Request) (int, string) { return 200, linkGrid })
	target := googlesheet.Target{SpreadsheetID: "sp-1", TabID: 41, TabName: "Pagamentos d'Ana"}
	cells, err := client.LinkCells(context.Background(), "token", target, []int{3})
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0].method != http.MethodGet || !strings.HasSuffix((*calls)[0].path, "/sp-1") {
		t.Fatalf("calls = %+v", *calls)
	}
	query, _ := url.ParseQuery((*calls)[0].query)
	if got := query["ranges"]; len(got) != 2 || got[0] != "'Pagamentos d''Ana'!A2:A" || got[1] != "'Pagamentos d''Ana'!D2:D" {
		t.Fatalf("ranges = %v", got)
	}
	if fields := query.Get("fields"); !strings.Contains(fields, "userEnteredValue") || !strings.Contains(fields, "errorValue") {
		t.Fatalf("fields = %q", fields)
	}
	want := []googlesheet.LinkCell{
		{Row: 2, Column: 3, RowRef: writtenRow, Formula: `=HYPERLINK("#gid=0&range=B"&MATCH("x",'T'!$A:$A,0),"Ver")`, Failed: true},
		{Row: 4, Column: 3, RowRef: brokenRow, Formula: `=HYPERLINK("#gid=0&range=B"&MATCH("y";'T'!$A:$A;0);"Ver")`},
	}
	if len(cells) != len(want) {
		t.Fatalf("cells = %+v", cells)
	}
	for i := range want {
		if cells[i] != want[i] {
			t.Fatalf("cell %d = %+v, want %+v", i, cells[i], want[i])
		}
	}
}

func TestLinkCellsOfASheetWithoutLinksReadsNothing(t *testing.T) {
	client, calls := fakeGoogle(t, func(*http.Request) (int, string) { return 200, `{}` })
	cells, err := client.LinkCells(context.Background(), "token", paymentsTarget(), nil)
	if err != nil || len(cells) != 0 || len(*calls) != 0 {
		t.Fatalf("cells = %+v err %v calls %d", cells, err, len(*calls))
	}
}

func TestLinkCellsRefusesAnAnswerThatDoesNotMatchTheRanges(t *testing.T) {
	client, _ := fakeGoogle(t, func(*http.Request) (int, string) {
		return 200, `{"sheets":[{"data":[{"startRow":1,"rowData":[]}]}]}`
	})
	if _, err := client.LinkCells(context.Background(), "token", paymentsTarget(), []int{3}); err == nil {
		t.Fatal("a short answer was trusted")
	}
}

func TestWriteFormulasRewritesOnlyTheGivenCellsInOneBatch(t *testing.T) {
	client, calls := fakeGoogle(t, func(*http.Request) (int, string) { return 200, `{}` })
	cells := []googlesheet.FormulaCell{
		{Row: 2, Column: 3, Formula: `=HYPERLINK("#gid=0&range=B"&MATCH("x";'T'!$A:$A;0);"Ver")`},
		{Row: 9, Column: 5, Formula: `=HYPERLINK("#gid=0&range=B"&MATCH("y";'T'!$A:$A;0);"Ver")`},
	}
	if err := client.WriteFormulas(context.Background(), "token", paymentsTarget(), cells); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatalf("calls = %d", len(*calls))
	}
	requests, _ := (*calls)[0].body["requests"].([]any)
	if len(requests) != 2 {
		t.Fatalf("requests = %v", requests)
	}
	raw, _ := json.Marshal(requests[1])
	text := string(raw)
	for _, want := range []string{`"rowIndex":8`, `"columnIndex":5`, `"sheetId":41`, `"fields":"userEnteredValue"`, `MATCH(\"y\";`} {
		if !strings.Contains(text, want) {
			t.Fatalf("request misses %s: %s", want, text)
		}
	}
}

func TestWriteFormulasNeverTouchesTheHeaderAndSendsNothingForNoCells(t *testing.T) {
	client, calls := fakeGoogle(t, func(*http.Request) (int, string) { return 200, `{}` })
	if err := client.WriteFormulas(context.Background(), "token", paymentsTarget(), nil); err != nil || len(*calls) != 0 {
		t.Fatalf("empty write = %v, calls %d", err, len(*calls))
	}
	header := []googlesheet.FormulaCell{{Row: 1, Column: 3, Formula: "=1"}}
	if err := client.WriteFormulas(context.Background(), "token", paymentsTarget(), header); err == nil || len(*calls) != 0 {
		t.Fatalf("header write = %v, calls %d", err, len(*calls))
	}
}
