package googlesheet

import (
	"encoding/json"
	"net/http"
	"testing"

	"vozko/domain/googlesheet"
)

func TestCreateAndWorkbookCarryTheUniqueRuleByColumnPosition(t *testing.T) {
	stub := &stubSheets{sheet: sampleSheet()}
	body := `{"title":"Pagamentos","columns":[{"header":"Matrícula","type":"text"}],"unique":{"mode":"columns","columnIndexes":[0],"onRepeat":"skip"}}`
	if rec := serve(NewHandler(stub, &stubRows{}), http.MethodPost, "/google-sheets", body, nil); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	got := stub.created.Unique
	if got.Mode != googlesheet.UniqueColumns || len(got.Columns) != 1 || got.Columns[0] != 0 || got.OnRepeat != googlesheet.RepeatSkip {
		t.Fatalf("create unique = %+v", got)
	}
	workbook := `{"fileTitle":"Cobrança","sheets":[{"title":"Pagamentos","columns":[{"header":"Valor","type":"money"}],"unique":{"mode":"conversation"}}]}`
	if rec := serve(NewHandler(stub, &stubRows{}), http.MethodPost, "/google-sheets/workbook", workbook, nil); rec.Code != http.StatusCreated {
		t.Fatalf("workbook = %d %s", rec.Code, rec.Body.String())
	}
	if mode := stub.workbook.Sheets[0].Input.Unique.Mode; mode != googlesheet.UniqueConversation {
		t.Fatalf("workbook unique = %s", mode)
	}
	if rec := serve(NewHandler(stub, &stubRows{}), http.MethodPost, "/google-sheets", `{"title":"Sem regra","columns":[]}`, nil); rec.Code != http.StatusCreated || stub.created.Unique.Mode != "" {
		t.Fatalf("an omitted rule must stay unset for the domain to default: %+v", stub.created.Unique)
	}
}

func TestUpdateDetailsCarriesTheRuleByColumnKey(t *testing.T) {
	stub := &stubSheets{sheet: sampleSheet()}
	rec := serve(NewHandler(stub, &stubRows{}), http.MethodPatch, "/google-sheets/s1", `{"version":3,"unique":{"mode":"columns","columns":["baixa"]}}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	in := stub.details
	if in.Version != 3 || in.Unique == nil || in.Unique.Mode != googlesheet.UniqueColumns || in.Unique.Columns[0] != "baixa" || in.Title != nil {
		t.Fatalf("details = %+v", in)
	}
	serve(NewHandler(stub, &stubRows{}), http.MethodPatch, "/google-sheets/s1", `{"version":3,"title":"Novo"}`, nil)
	if stub.details.Unique != nil {
		t.Fatal("a rename must not touch the rule")
	}
}

func TestTheSheetShowsItsUniqueRule(t *testing.T) {
	sheet := sampleSheet()
	sheet.Unique = googlesheet.UniqueRule{Mode: googlesheet.UniqueColumns, Columns: []string{"baixa"}, OnRepeat: googlesheet.RepeatSkip}
	rec := serve(NewHandler(&stubSheets{sheet: sheet}, &stubRows{}), http.MethodGet, "/google-sheets/s1", "", nil)
	var body struct {
		Sheet struct {
			Unique struct {
				Mode     string   `json:"mode"`
				Columns  []string `json:"columns"`
				OnRepeat string   `json:"onRepeat"`
			} `json:"unique"`
		} `json:"sheet"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Sheet.Unique.Mode != "columns" || len(body.Sheet.Unique.Columns) != 1 || body.Sheet.Unique.OnRepeat != "skip" {
		t.Fatalf("unique = %+v", body.Sheet.Unique)
	}
	rec = serve(NewHandler(&stubSheets{sheet: sampleSheet()}, &stubRows{}), http.MethodGet, "/google-sheets/s1", "", nil)
	var none map[string]map[string]map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &none)
	unique := none["sheet"]["unique"]
	if unique["mode"] != "none" || unique["onRepeat"] != "skip" || unique["columns"] == nil {
		t.Fatalf("a tab without a rule = %+v", unique)
	}
}

func TestAManualRepeatIsRefusedWithItsCodeTheHeadersAndTheRow(t *testing.T) {
	rows := &stubRows{repeat: &googlesheet.RepeatError{Code: googlesheet.CodeAlreadyRecordedColumnsMonth, RowID: "row-first", Headers: []string{"Matrícula", "Forma"}}}
	rec := serve(NewHandler(&stubSheets{}, rows), http.MethodPost, "/google-sheets/s1/rows", `{"values":{"matricula":"1"}}`, map[string]string{"Idempotency-Key": "k1"})
	code, details := codeOf(t, rec)
	if rec.Code != http.StatusConflict || code != googlesheet.CodeAlreadyRecordedColumnsMonth || details["header"] != "Matrícula, Forma" || details["rowId"] != "row-first" {
		t.Fatalf("status = %d code = %s details = %v", rec.Code, code, details)
	}
}

func TestAWriteWithoutAConversationOnAConversationTabIsRefused(t *testing.T) {
	rec := serve(NewHandler(&stubSheets{}, &stubRows{err: googlesheet.ErrUniqueNeedsConversation}), http.MethodPost, "/google-sheets/s1/rows", `{"values":{}}`, map[string]string{"Idempotency-Key": "k1"})
	if code, _ := codeOf(t, rec); rec.Code != http.StatusUnprocessableEntity || code != googlesheet.CodeUniqueNeedsConversation {
		t.Fatalf("status = %d code = %s", rec.Code, code)
	}
}

func TestTheCatalogAndPresetsDescribeTheUniqueRule(t *testing.T) {
	rec := serve(NewHandler(&stubSheets{}, &stubRows{}), http.MethodGet, "/google-sheets/catalog", "", nil)
	var catalog struct {
		UniqueModes   []string `json:"uniqueModes"`
		RepeatActions []string `json:"repeatActions"`
		UniqueSources []string `json:"uniqueSources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.UniqueModes) != 3 || len(catalog.RepeatActions) != 1 || catalog.RepeatActions[0] != "skip" || catalog.UniqueSources[0] != "input" {
		t.Fatalf("catalog = %+v", catalog)
	}
	for _, source := range catalog.UniqueSources {
		if source == "written_at" || source == "export_link" || source == "message_text" {
			t.Fatalf("%s offered as a unique key", source)
		}
	}
	rec = serve(NewHandler(&stubSheets{}, &stubRows{}), http.MethodGet, "/google-sheets/presets", "", nil)
	var presets PresetListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &presets); err != nil {
		t.Fatal(err)
	}
	for _, preset := range presets.Items {
		for _, sheet := range preset.Sheets {
			want := "none"
			if preset.Key == "cobranca" && sheet.Ref == "pagamentos" {
				want = "conversation"
			}
			if sheet.Unique.Mode != want || sheet.Unique.Columns == nil {
				t.Fatalf("%s/%s unique = %+v", preset.Key, sheet.Ref, sheet.Unique)
			}
		}
	}
}
