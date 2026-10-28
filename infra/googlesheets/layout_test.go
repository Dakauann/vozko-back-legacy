package googlesheets

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"vozko/domain/googlesheet"
)

func styledLayout(style googlesheet.StylePreset) googlesheet.Layout {
	cols := columns()
	cols[2].Colors = map[string]googlesheet.OptionColor{"Pago": googlesheet.ColorGreen}
	cols = append(cols,
		googlesheet.Column{Key: "obs", Header: "Obs", Type: googlesheet.TypeLongText, Source: googlesheet.SourceInput},
		googlesheet.Column{Key: "link", Header: "Negociação", Type: googlesheet.TypeLink, Source: googlesheet.SourceExportLink, LinkLabel: "Ver"},
	)
	return googlesheet.Layout{Columns: cols, Style: style}
}

func batchText(t *testing.T, call recorded) string {
	t.Helper()
	raw, _ := json.Marshal(call.body)
	return string(raw)
}

func requestKinds(call recorded) []string {
	requests, _ := call.body["requests"].([]any)
	kinds := make([]string, 0, len(requests))
	for _, r := range requests {
		for kind := range r.(map[string]any) {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

func countKind(kinds []string, want string) int {
	n := 0
	for _, k := range kinds {
		if k == want {
			n++
		}
	}
	return n
}

func TestCreateSpreadsheetSizesTheTabAndAppliesTheStyle(t *testing.T) {
	client, calls := fakeGoogle(t, func(r *http.Request) (int, string) {
		if strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			return 200, `{}`
		}
		return 200, `{"spreadsheetId":"sp-1","sheets":[{"properties":{"sheetId":77,"title":"Pedidos"}}]}`
	})
	created, err := client.CreateSpreadsheet(context.Background(), "token", googlesheet.SpreadsheetSpec{
		Title: "Pedidos", TabName: "Pedidos", Locale: "pt_BR", TimeZone: "America/Sao_Paulo", Layout: styledLayout(googlesheet.StyleGreen),
	})
	if err != nil || created.SpreadsheetID != "sp-1" || created.TabID != 77 {
		t.Fatalf("CreateSpreadsheet = %+v, %v", created, err)
	}
	create := batchText(t, (*calls)[0])
	if !strings.Contains(create, `"rowCount":100`) || !strings.Contains(create, `"frozenRowCount":1`) {
		t.Fatalf("new tab not sized to need: %s", create)
	}
	batch := batchText(t, (*calls)[1])
	for _, want := range []string{
		`"stringValue":"ref"`, `"stringValue":"Nome"`, `"hiddenByUser":true`, `"CURRENCY"`, `"ONE_OF_LIST"`, `"BOOLEAN"`,
		`"warningOnly":true`, `"addBanding"`, `"headerColorStyle"`, `"addConditionalFormatRule"`, `"TEXT_EQ"`,
		`"userEnteredValue":"Pago"`, `"WRAP"`, `"pixelSize"`, `"frozenRowCount":1`,
	} {
		if !strings.Contains(batch, want) {
			t.Fatalf("layout misses %s: %s", want, batch)
		}
	}
	if strings.Contains(batch, `"endRowIndex"`) && strings.Contains(batch, `"addBanding":{"bandedRange":{"range":{"endRowIndex"`) {
		t.Fatalf("banding must not stop at a row: %s", batch)
	}
}

func TestPlainStyleHasNoBandingNorFill(t *testing.T) {
	client, calls := fakeGoogle(t, func(r *http.Request) (int, string) {
		if strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			return 200, `{}`
		}
		return 200, `{"spreadsheetId":"sp-1","sheets":[{"properties":{"sheetId":77,"title":"Pedidos"}}]}`
	})
	layout := styledLayout(googlesheet.StylePlain)
	if _, err := client.CreateSpreadsheet(context.Background(), "token", googlesheet.SpreadsheetSpec{Title: "P", TabName: "P", Layout: layout}); err != nil {
		t.Fatal(err)
	}
	batch := batchText(t, (*calls)[1])
	if strings.Contains(batch, `"addBanding"`) || strings.Contains(batch, `"headerColorStyle"`) {
		t.Fatalf("plain style painted the table: %s", batch)
	}
	if !strings.Contains(batch, `"addConditionalFormatRule"`) {
		t.Fatalf("option colours are independent of the style: %s", batch)
	}
}

func TestAddTabCreatesAndStylesTheTabInOneBatch(t *testing.T) {
	client, calls := fakeGoogle(t, func(*http.Request) (int, string) { return 200, `{}` })
	err := client.AddTab(context.Background(), "token", "sp-1", googlesheet.TabSpec{ID: 4242, Title: "Pagamentos · novembro 2026", Layout: styledLayout(googlesheet.StyleBlue)})
	if err != nil {
		t.Fatalf("AddTab: %v", err)
	}
	if len(*calls) != 1 || !strings.HasSuffix((*calls)[0].path, ":batchUpdate") {
		t.Fatalf("calls = %+v, want one batch update", *calls)
	}
	kinds := requestKinds((*calls)[0])
	if kinds[0] != "addSheet" {
		t.Fatalf("first request = %s, the tab must exist before it is styled", kinds[0])
	}
	text := batchText(t, (*calls)[0])
	for _, want := range []string{`"sheetId":4242`, `"title":"Pagamentos · novembro 2026"`, `"rowCount":100`, `"columnCount":7`, `"addProtectedRange"`, `"addBanding"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("AddTab misses %s: %s", want, text)
		}
	}
}

func TestDeleteTabRemovesOnlyThatTab(t *testing.T) {
	client, calls := fakeGoogle(t, func(*http.Request) (int, string) { return 200, `{}` })
	if err := client.DeleteTab(context.Background(), "token", "sp-1", 4242); err != nil {
		t.Fatal(err)
	}
	text := batchText(t, (*calls)[0])
	if !strings.Contains(text, `"deleteSheet":{"sheetId":4242}`) {
		t.Fatalf("delete = %s", text)
	}
}

func TestWorkbookListsEveryTabWithItsGrid(t *testing.T) {
	client, calls := fakeGoogle(t, func(*http.Request) (int, string) {
		return 200, `{"sheets":[{"properties":{"sheetId":0,"title":"Negociações","gridProperties":{"rowCount":500,"columnCount":9}}},{"properties":{"sheetId":77,"title":"Pagamentos","gridProperties":{"rowCount":120,"columnCount":11}}}]}`
	})
	wb, err := client.Workbook(context.Background(), "token", "sp-1")
	if err != nil || len(wb.Tabs) != 2 {
		t.Fatalf("Workbook = %+v, %v", wb, err)
	}
	if tab, ok := wb.Tab(77); !ok || tab.Title != "Pagamentos" || tab.Rows != 120 || tab.Columns != 11 {
		t.Fatalf("tab 77 = %+v", tab)
	}
	if wb.Cells() != 500*9+120*11 {
		t.Fatalf("cells = %d", wb.Cells())
	}
	if !strings.Contains((*calls)[0].query, "fields=") {
		t.Fatalf("query = %s", (*calls)[0].query)
	}
}

const styledTabState = `{"sheets":[
	{"properties":{"sheetId":1,"title":"Outra","gridProperties":{"rowCount":10,"columnCount":3}},
	 "conditionalFormats":[{"ranges":[{"sheetId":1,"startColumnIndex":3,"endColumnIndex":4}],"booleanRule":{"condition":{"type":"TEXT_EQ","values":[{"userEnteredValue":"x"}]}}}]},
	{"properties":{"sheetId":77,"title":"Pedidos","gridProperties":{"rowCount":300,"columnCount":5}},
	 "conditionalFormats":[
	   {"ranges":[{"sheetId":77,"startColumnIndex":3,"endColumnIndex":4}],"booleanRule":{"condition":{"type":"TEXT_EQ","values":[{"userEnteredValue":"Novo"}]}}},
	   {"ranges":[{"sheetId":77,"startRowIndex":4,"endRowIndex":9,"startColumnIndex":1,"endColumnIndex":3}],"booleanRule":{"condition":{"type":"NUMBER_GREATER","values":[{"userEnteredValue":"10"}]}}},
	   {"ranges":[{"sheetId":77,"endRowIndex":300,"startColumnIndex":3,"endColumnIndex":4}],"booleanRule":{"condition":{"type":"TEXT_EQ","values":[{"userEnteredValue":"Pago"}]}}}
	 ],
	 "bandedRanges":[
	   {"bandedRangeId":11,"range":{"sheetId":77,"endRowIndex":300,"endColumnIndex":5}},
	   {"bandedRangeId":12,"range":{"sheetId":77,"startRowIndex":10,"endRowIndex":20,"startColumnIndex":6,"endColumnIndex":8}}
	 ]}
]}`

func TestApplyLayoutReplacesOnlyWhatVozkoOwns(t *testing.T) {
	client, calls := fakeGoogle(t, func(r *http.Request) (int, string) {
		if r.Method == http.MethodGet {
			return 200, styledTabState
		}
		return 200, `{}`
	})
	layout := styledLayout(googlesheet.StyleGraphite)
	layout.SizeFrom = 4
	err := client.ApplyLayout(context.Background(), "token", googlesheet.Target{SpreadsheetID: "sp-1", TabID: 77, TabName: "Pedidos"}, layout)
	if err != nil {
		t.Fatalf("ApplyLayout: %v", err)
	}
	if !strings.Contains((*calls)[0].query, "conditionalFormats") || !strings.Contains((*calls)[0].query, "bandedRanges") {
		t.Fatalf("state query = %s", (*calls)[0].query)
	}
	batch := (*calls)[1]
	text := batchText(t, batch)
	kinds := requestKinds(batch)
	if countKind(kinds, "deleteConditionalFormatRule") != 2 || countKind(kinds, "deleteBanding") != 1 {
		t.Fatalf("deleted %d rules and %d bandings, want 2 and 1: %s", countKind(kinds, "deleteConditionalFormatRule"), countKind(kinds, "deleteBanding"), text)
	}
	first := strings.Index(text, `"deleteConditionalFormatRule":{"index":2`)
	second := strings.Index(text, `"deleteConditionalFormatRule":{"index":0`)
	if first < 0 || second < 0 || first > second {
		t.Fatalf("rules must be deleted from the highest index down: %s", text)
	}
	if !strings.Contains(text, `"bandedRangeId":11`) || strings.Contains(text, `"bandedRangeId":12`) {
		t.Fatalf("wrong banding deleted: %s", text)
	}
	if !strings.Contains(text, `"appendDimension"`) || !strings.Contains(text, `"length":2`) {
		t.Fatalf("grid not grown for the new columns: %s", text)
	}
	if countKind(kinds, "updateDimensionProperties") != 1+2 {
		t.Fatalf("widths must be set only for columns from SizeFrom on, got kinds %v", kinds)
	}
}

func TestApplyLayoutOnAMissingTabIsNotFound(t *testing.T) {
	client, _ := fakeGoogle(t, func(*http.Request) (int, string) { return 200, styledTabState })
	err := client.ApplyLayout(context.Background(), "token", googlesheet.Target{SpreadsheetID: "sp-1", TabID: 99}, styledLayout(googlesheet.StylePlain))
	if got := (Classifier{}).Classify(err); got.Kind != googlesheet.FailureNotFound {
		t.Fatalf("missing tab classified as %s (%v)", got.Kind, err)
	}
}

func TestRenameSpreadsheetChangesOnlyTheFileTitle(t *testing.T) {
	client, calls := fakeGoogle(t, func(*http.Request) (int, string) { return 200, `{}` })
	if err := client.RenameSpreadsheet(context.Background(), "token", "sp-1", "CDT Jabaquara"); err != nil {
		t.Fatal(err)
	}
	text := batchText(t, (*calls)[0])
	if !strings.Contains(text, `"updateSpreadsheetProperties":{"fields":"title","properties":{"title":"CDT Jabaquara"}}`) {
		t.Fatalf("rename = %s", text)
	}
}

const shiftedTabState = `{"sheets":[
	{"properties":{"sheetId":0,"title":"Negociações","gridProperties":{"rowCount":117,"columnCount":10}},
	 "conditionalFormats":[{"ranges":[{"sheetId":0,"startRowIndex":17,"endRowIndex":117,"startColumnIndex":5,"endColumnIndex":6}],"booleanRule":{"condition":{"type":"TEXT_EQ","values":[{"userEnteredValue":"Entrada"}]}}}],
	 "bandedRanges":[
	   {"bandedRangeId":21,"range":{"sheetId":0,"startRowIndex":17,"endRowIndex":117,"startColumnIndex":0,"endColumnIndex":10}},
	   {"bandedRangeId":22,"range":{"sheetId":0,"startRowIndex":2,"endRowIndex":5,"startColumnIndex":11,"endColumnIndex":12}}
	 ],
	 "protectedRanges":[
	   {"protectedRangeId":31,"description":"Cabeçalho usado pelas gravações automáticas","range":{"sheetId":0,"startRowIndex":17,"endRowIndex":18}},
	   {"protectedRangeId":32,"description":"Fechamento do mês","range":{"sheetId":0,"startRowIndex":40,"endRowIndex":60}}
	 ]}
]}`

func TestApplyLayoutReplacesVozkoBandingRulesAndProtectionEvenAfterRowsPushedThemDown(t *testing.T) {
	client, calls := fakeGoogle(t, func(r *http.Request) (int, string) {
		if r.Method == http.MethodGet {
			return 200, shiftedTabState
		}
		return 200, `{}`
	})
	err := client.ApplyLayout(context.Background(), "token", googlesheet.Target{SpreadsheetID: "sp-1", TabID: 0, TabName: "Negociações"}, styledLayout(googlesheet.StylePurple))
	if err != nil {
		t.Fatalf("ApplyLayout: %v", err)
	}
	if !strings.Contains((*calls)[0].query, "protectedRanges") {
		t.Fatalf("state query = %s", (*calls)[0].query)
	}
	text := batchText(t, (*calls)[1])
	for _, want := range []string{`"bandedRangeId":21`, `"protectedRangeId":31`, `"deleteConditionalFormatRule":{"index":0`, `"addProtectedRange"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("layout misses %s: %s", want, text)
		}
	}
	for _, kept := range []string{`"bandedRangeId":22`, `"protectedRangeId":32`} {
		if strings.Contains(text, kept) {
			t.Fatalf("layout touched what the owner made (%s): %s", kept, text)
		}
	}
}
