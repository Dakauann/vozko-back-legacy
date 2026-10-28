package googlesheets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"vozko/domain/googlesheet"
)

func paymentsHeader() []any {
	return []any{googlesheet.RefHeader, "Nome", "Valor", "Status", "Aceitou"}
}

func paymentsTab(rows ...[]any) *fakeTab {
	return &fakeTab{id: 41, title: "Pagamentos · outubro 2026", rows: append([][]any{paymentsHeader()}, rows...), grid: googlesheet.NewTabRows}
}

func paymentsTarget() googlesheet.Target {
	return googlesheet.Target{SpreadsheetID: "sp-1", TabID: 41, TabName: "Pagamentos · outubro 2026"}
}

func snapshotOf(t *testing.T, client *Client, target googlesheet.Target) googlesheet.TabSnapshot {
	t.Helper()
	read, err := client.ReadRanges(context.Background(), "token", target, []string{googlesheet.HeaderRange, googlesheet.RefColumnRange})
	if err != nil {
		t.Fatalf("ReadRanges: %v", err)
	}
	return googlesheet.TabSnapshot{Header: read[0], Refs: read[1]}
}

func insertKinds(t *testing.T, g *googleGrid) []map[string]any {
	t.Helper()
	last := g.batches[len(g.batches)-1]
	var out []map[string]any
	for _, r := range last["requests"].([]any) {
		out = append(out, r.(map[string]any))
	}
	return out
}

func TestInsertRowsOnAFreshTabLandsInRowTwoUnderTheHeader(t *testing.T) {
	client, g := newGoogleGrid(t, paymentsTab())
	snapshot := snapshotOf(t, client, paymentsTarget())
	at := snapshot.NextFreeRow()
	if err := client.InsertRows(context.Background(), "token", paymentsTarget(), at, [][]any{{"r-1", "Ana", 10.0, "Pago", true}}); err != nil {
		t.Fatalf("InsertRows: %v", err)
	}
	if got := g.column("Pagamentos · outubro 2026"); len(got) != 2 || got[0] != googlesheet.RefHeader || got[1] != "r-1" {
		t.Fatalf("column A = %v, want the header first and the row under it", got)
	}
	for _, path := range g.paths {
		if strings.Contains(path, ":append") {
			t.Fatalf("rows must never go through the append table heuristic: %s", path)
		}
	}
}

func TestInsertRowsBelowTheLastRefPushesAHumanRowDownInsteadOfOverwritingIt(t *testing.T) {
	client, g := newGoogleGrid(t, paymentsTab([]any{"r-1", "Ana"}, []any{"", "linha digitada"}))
	at := snapshotOf(t, client, paymentsTarget()).NextFreeRow()
	if at != 3 {
		t.Fatalf("next free row = %d, want 3", at)
	}
	if err := client.InsertRows(context.Background(), "token", paymentsTarget(), at, [][]any{{"r-2", "Bia"}}); err != nil {
		t.Fatal(err)
	}
	rows := g.tabs["Pagamentos · outubro 2026"].rows
	if rows[2][0] != "r-2" || len(rows) != 4 || rows[3][1] != "linha digitada" {
		t.Fatalf("rows = %v, the human row must survive below", rows)
	}
}

func TestTwoBatchesThatPickTheSameRowBothSurvive(t *testing.T) {
	client, g := newGoogleGrid(t, paymentsTab())
	for _, ref := range []string{"r-1", "r-2"} {
		if err := client.InsertRows(context.Background(), "token", paymentsTarget(), 2, [][]any{{ref, ref}}); err != nil {
			t.Fatal(err)
		}
	}
	got := g.column("Pagamentos · outubro 2026")
	if len(got) != 3 || got[0] != googlesheet.RefHeader {
		t.Fatalf("column A = %v, an insert never overwrites another batch", got)
	}
}

func TestInsertRowsInheritsTheBodyFormatNeverTheHeader(t *testing.T) {
	client, g := newGoogleGrid(t, paymentsTab([]any{"r-1"}))
	cases := []struct {
		at     int
		before bool
	}{{2, false}, {3, true}}
	for _, c := range cases {
		if err := client.InsertRows(context.Background(), "token", paymentsTarget(), c.at, [][]any{{fmt.Sprint("x", c.at)}}); err != nil {
			t.Fatal(err)
		}
		insert := insertKinds(t, g)[0]["insertDimension"].(map[string]any)
		if insert["inheritFromBefore"] != c.before {
			t.Fatalf("row %d inheritFromBefore = %v, want %v", c.at, insert["inheritFromBefore"], c.before)
		}
	}
}

func TestInsertRowsRefusesToWriteOverTheHeader(t *testing.T) {
	client, g := newGoogleGrid(t, paymentsTab())
	if err := client.InsertRows(context.Background(), "token", paymentsTarget(), 1, [][]any{{"r-1"}}); err == nil {
		t.Fatal("a write at row 1 would push the header down")
	}
	if len(g.batches) != 0 {
		t.Fatalf("batches = %v", g.batches)
	}
}

func TestInsertRowsIsOneAtomicBatchAndKeepsTextLiteralSoOnlyTheDomainFormulaIsAFormula(t *testing.T) {
	client, g := newGoogleGrid(t, paymentsTab())
	formula := googlesheet.Formula(`=HYPERLINK("#gid=1&range=B"&MATCH("x",'T'!$A:$A,0),"Ver")`)
	row := []any{"r1", "=1+1", "+5511999", "-2", "@x", "'já", "", 12.5, true, formula}
	if err := client.InsertRows(context.Background(), "token", paymentsTarget(), 2, [][]any{row}); err != nil {
		t.Fatal(err)
	}
	if len(g.batches) != 1 {
		t.Fatalf("batches = %d, the insert and the values must land together", len(g.batches))
	}
	requests := insertKinds(t, g)
	if len(requests) != 2 || requests[0]["insertDimension"] == nil || requests[1]["updateCells"] == nil {
		t.Fatalf("requests = %v", requests)
	}
	raw, _ := json.Marshal(requests[1])
	text := string(raw)
	for _, want := range []string{
		`{"stringValue":"r1"}`, `{"stringValue":"=1+1"}`, `{"stringValue":"+5511999"}`, `{"stringValue":"-2"}`, `{"stringValue":"@x"}`,
		`{"stringValue":"'já"}`, `{"numberValue":12.5}`, `{"boolValue":true}`, `"formulaValue":"=HYPERLINK(`, `"fields":"userEnteredValue"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("updateCells misses %s: %s", want, text)
		}
	}
	if strings.Count(text, "formulaValue") != 1 {
		t.Fatalf("only the domain formula may be a formula: %s", text)
	}
	if got := g.tabs["Pagamentos · outubro 2026"].rows[1]; got[1] != "=1+1" || got[9] != formula || got[6] != "" {
		t.Fatalf("stored row = %#v", got)
	}
}

func TestInsertRowsRefusesACellOfAnUnknownType(t *testing.T) {
	client, g := newGoogleGrid(t, paymentsTab())
	if err := client.InsertRows(context.Background(), "token", paymentsTarget(), 2, [][]any{{"r1", struct{}{}}}); err == nil {
		t.Fatal("an unknown cell type must not be guessed")
	}
	if len(g.batches) != 0 {
		t.Fatal("nothing may be sent for a row that cannot be encoded")
	}
}

func TestReadRangesReadsTheHeaderAndTheRefColumnInOneCall(t *testing.T) {
	client, g := newGoogleGrid(t, paymentsTab([]any{"r-1", "Ana"}))
	snapshot := snapshotOf(t, client, paymentsTarget())
	if len(g.paths) != 1 || !strings.HasSuffix(g.paths[0], "/values:batchGet") {
		t.Fatalf("paths = %v", g.paths)
	}
	if len(snapshot.Header) != 1 || snapshot.Header[0][1] != "Nome" || len(snapshot.Refs) != 2 || snapshot.Refs[1][0] != "r-1" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestReadRangeOfAMissingTabIsNotFound(t *testing.T) {
	client, _ := newGoogleGrid(t, paymentsTab())
	_, err := client.ReadRange(context.Background(), "token", googlesheet.Target{SpreadsheetID: "sp-1", TabID: 9, TabName: "Sumiu"}, googlesheet.HeaderRange)
	if got := (Classifier{}).Classify(err); got.Kind != googlesheet.FailureNotFound {
		t.Fatalf("missing tab classified as %s (%v)", got.Kind, err)
	}
}

func damagedNegotiations() *fakeTab {
	var rows [][]any
	for i := 17; i >= 1; i-- {
		rows = append(rows, []any{fmt.Sprint("m-", i), "", "", "", "", "", "", "", "", "ABC7366477"})
	}
	header := []any{googlesheet.RefHeader, "Data", "Cliente", "Telefone", "Quem enviou", "Sentido", "Mensagem", "Anexo", "Conversa", "Matrícula"}
	return &fakeTab{id: 0, title: "Negociações", rows: append(rows, header), grid: googlesheet.NewTabRows + 17}
}

func negotiationsLayout() googlesheet.Layout {
	var cols []googlesheet.Column
	for _, h := range []string{"Data", "Cliente", "Telefone", "Quem enviou", "Sentido", "Mensagem", "Anexo", "Conversa", "Matrícula"} {
		cols = append(cols, googlesheet.Column{Key: strings.ToLower(h), Header: h, Type: googlesheet.TypeText, Source: googlesheet.SourceInput})
	}
	cols[0].Type = googlesheet.TypeDateTime
	return googlesheet.Layout{Columns: cols, Style: googlesheet.StylePurple}
}

func TestRepairHeaderMovesTheHeaderBackAboveTheRowsWrittenOverItKeepingTheirOrder(t *testing.T) {
	tab := damagedNegotiations()
	client, g := newGoogleGrid(t, tab)
	target := googlesheet.Target{SpreadsheetID: "sp-1", TabID: 0, TabName: "Negociações"}
	if err := client.RepairHeader(context.Background(), "token", target, googlesheet.HeaderRepair{MoveFrom: 18}, negotiationsLayout()); err != nil {
		t.Fatalf("RepairHeader: %v", err)
	}
	column := g.column("Negociações")
	if len(column) != 18 || column[0] != googlesheet.RefHeader || column[1] != "m-17" || column[17] != "m-1" {
		t.Fatalf("column A = %v", column)
	}
	if got := tab.rows[15][9]; got != "ABC7366477" {
		t.Fatalf("moved row lost its value: %v", tab.rows[15])
	}
	if len(g.batches) != 1 {
		t.Fatalf("batches = %d, the move and the layout must land together", len(g.batches))
	}
	raw, _ := json.Marshal(g.batches[0])
	text := string(raw)
	for _, want := range []string{
		`"moveDimension":{"destinationIndex":0,"source":{"dimension":"ROWS","endIndex":18,"sheetId":0,"startIndex":17}}`,
		`"userEnteredFormat.textFormat"`, `"endRowIndex":18`, `"addBanding"`, `"DATE_TIME"`, `"addProtectedRange"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("repair misses %s: %s", want, text)
		}
	}
	if strings.Contains(text, "deleteDimension") || strings.Contains(text, "deleteRange") {
		t.Fatalf("a repair never deletes rows: %s", text)
	}
	snapshot := snapshotOf(t, client, target)
	if len(snapshot.Header) != 1 || snapshot.Header[0][9] != "Matrícula" {
		t.Fatalf("header after repair = %v", snapshot.Header)
	}
}

func TestRepairHeaderInsertsAFreshHeaderAboveARowThatIsNotAHeader(t *testing.T) {
	tab := &fakeTab{id: 41, title: "Pagamentos · outubro 2026", rows: [][]any{{"r-1", "Ana"}}, grid: googlesheet.NewTabRows}
	client, g := newGoogleGrid(t, tab)
	if err := client.RepairHeader(context.Background(), "token", paymentsTarget(), googlesheet.HeaderRepair{Insert: true}, styledLayout(googlesheet.StyleGreen)); err != nil {
		t.Fatalf("RepairHeader: %v", err)
	}
	column := g.column("Pagamentos · outubro 2026")
	if len(column) != 2 || column[0] != googlesheet.RefHeader || column[1] != "r-1" {
		t.Fatalf("column A = %v", column)
	}
}
