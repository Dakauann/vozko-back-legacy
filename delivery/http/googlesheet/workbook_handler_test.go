package googlesheet

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"vozko/domain/googlesheet"
	workspace_domain "vozko/domain/workspace"
	"vozko/pkg/codegolden"
	gsuc "vozko/usecases/googlesheet"
)

func sampleSheet() *googlesheet.Sheet {
	return &googlesheet.Sheet{
		ID: "s1", Title: "Pagamentos", SpreadsheetID: "sp-1", TabPeriod: googlesheet.PeriodMonth, TabPattern: "Pagamentos · {mes} {ano}",
		TabPeriodKey: "2026-11", Style: googlesheet.StyleGreen,
		Columns: []googlesheet.Column{
			{Key: "baixa", Header: "Baixa", Type: googlesheet.TypeChoice, Source: googlesheet.SourceInput, Options: []string{"SIM"}, Colors: map[string]googlesheet.OptionColor{"SIM": googlesheet.ColorGreen}},
			{Key: "negociacao", Header: "Negociação", Type: googlesheet.TypeLink, Source: googlesheet.SourceExportLink, LinkSheetID: "t1", LinkLabel: "Ver"},
		},
	}
}

func TestADefinitionRefusalNamesTheFieldAndTheColumn(t *testing.T) {
	err := &googlesheet.DefinitionError{Code: googlesheet.CodeOptionRepeated, Field: googlesheet.FieldColumns, Column: 2, Header: "Status", Option: "novo"}
	rec := serve(NewHandler(&stubSheets{err: err}, &stubRows{}), http.MethodPost, "/google-sheets", `{"title":"x"}`, nil)
	code, details := codeOf(t, rec)
	if rec.Code != http.StatusBadRequest || code != googlesheet.CodeOptionRepeated {
		t.Fatalf("status = %d code = %s", rec.Code, code)
	}
	if details["field"] != "columns" || details["column"] != 2.0 || details["header"] != "Status" || details["option"] != "novo" {
		t.Fatalf("details = %v", details)
	}
}

func TestAWorkbookRefusalSaysWhichSheet(t *testing.T) {
	inner := &googlesheet.DefinitionError{Code: googlesheet.CodeHeaderRequired, Field: googlesheet.FieldColumns, Column: 0}
	rec := serve(NewHandler(&stubSheets{err: &gsuc.WorkbookError{Sheet: 1, Err: inner}}, &stubRows{}), http.MethodPost, "/google-sheets/workbook", `{"sheets":[{"title":"a"},{"title":"b"}]}`, nil)
	code, details := codeOf(t, rec)
	if code != googlesheet.CodeHeaderRequired || details["sheet"] != 1.0 || details["column"] != 0.0 {
		t.Fatalf("code = %s details = %v", code, details)
	}
	rec = serve(NewHandler(&stubSheets{err: &gsuc.WorkbookError{Sheet: 1, Err: googlesheet.ErrTabNameTaken}}, &stubRows{}), http.MethodPost, "/google-sheets/workbook", `{"sheets":[{"title":"a"}]}`, nil)
	code, details = codeOf(t, rec)
	if rec.Code != http.StatusConflict || code != googlesheet.CodeTabNameTaken || details["sheet"] != 1.0 {
		t.Fatalf("status %d code = %s details = %v", rec.Code, code, details)
	}
}

func TestCreateWorkbookTurnsSiblingRefsIntoLinkRefs(t *testing.T) {
	stub := &stubSheets{sheet: sampleSheet()}
	body := `{"spreadsheetId":"sp-1","sheets":[
		{"ref":"neg","title":"Negociações","columns":[{"header":"Texto","type":"long_text","source":"message_text"}]},
		{"ref":"pag","title":"Pagamentos","tabPeriod":"month","style":"green","columns":[
			{"header":"Baixa","type":"choice","options":["SIM"],"colors":{"SIM":"green"}},
			{"header":"Negociação","type":"link","source":"export_link","linkSheetRef":"neg","linkLabel":"Ver"}]}]}`
	rec := serve(NewHandler(stub, &stubRows{}), http.MethodPost, "/google-sheets/workbook", body, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	in := stub.workbook
	if in.SpreadsheetID != "sp-1" || len(in.Sheets) != 2 || in.Sheets[1].Ref != "pag" || in.Sheets[1].LinkRefs[1] != "neg" {
		t.Fatalf("input = %+v", in)
	}
	pay := in.Sheets[1].Input
	if pay.TabPeriod != googlesheet.PeriodMonth || pay.Style != googlesheet.StyleGreen || pay.Columns[0].Colors["SIM"] != googlesheet.ColorGreen || pay.Columns[1].LinkLabel != "Ver" {
		t.Fatalf("payments = %+v", pay)
	}
}

func TestSheetResponsesCarryTheWorkbookFields(t *testing.T) {
	rec := serve(NewHandler(&stubSheets{sheet: sampleSheet()}, &stubRows{}), http.MethodGet, "/google-sheets/s1", "", nil)
	var body struct {
		Sheet SheetResponse `json:"sheet"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	s := body.Sheet
	if s.SpreadsheetID != "sp-1" || s.TabPeriod != "month" || s.Style != "green" || s.TabPattern == "" || !s.TakesInput || s.TabBaseName != "Pagamentos" {
		t.Fatalf("sheet = %+v", s)
	}
	if s.Columns[0].Colors["SIM"] != "green" || s.Columns[1].LinkSheetID != "t1" || s.Columns[1].LinkLabel != "Ver" {
		t.Fatalf("columns = %+v", s.Columns)
	}
}

func TestListCanAskOnlyForSheetsThatTakeInput(t *testing.T) {
	stub := &stubSheets{sheet: sampleSheet()}
	serve(NewHandler(stub, &stubRows{}), http.MethodGet, "/google-sheets?takesInput=true", "", nil)
	if !stub.list.TakesInput {
		t.Fatal("takesInput was not passed on")
	}
}

func TestUpdateLayoutPassesStyleAndPattern(t *testing.T) {
	stub := &stubSheets{sheet: sampleSheet()}
	rec := serve(NewHandler(stub, &stubRows{}), http.MethodPatch, "/google-sheets/s1/layout", `{"version":3,"style":"blue","tabPattern":"P {mm}-{ano}"}`, nil)
	if rec.Code != http.StatusOK || stub.layout.Version != 3 || *stub.layout.Style != googlesheet.StyleBlue || *stub.layout.TabPattern != "P {mm}-{ano}" || stub.layout.ID != "s1" {
		t.Fatalf("status %d layout %+v", rec.Code, stub.layout)
	}
}

func TestSpreadsheetsListTheFilesWithTheirSheets(t *testing.T) {
	stub := &stubSheets{books: []gsuc.WorkbookView{{SpreadsheetID: "sp-1", Title: "Cobrança", Usable: true, Sheets: []gsuc.WorkbookTab{
		{Sheet: sampleSheet(), Tabs: []googlesheet.Tab{{Period: "2026-11", ID: 9, Title: "Pagamentos · novembro 2026"}}},
	}}}}
	rec := serve(NewHandler(stub, &stubRows{}), http.MethodGet, "/google-sheets/spreadsheets", "", nil)
	var body SpreadsheetListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || !body.Items[0].Usable || body.Items[0].Sheets[0].Tabs[0].Title != "Pagamentos · novembro 2026" || body.Items[0].Sheets[0].ID != "s1" {
		t.Fatalf("body = %+v", body)
	}
}

func TestTabNamePreviewRendersOrRefuses(t *testing.T) {
	h := NewHandler(&stubSheets{}, &stubRows{})
	rec := serve(h, http.MethodPost, "/google-sheets/tab-name-preview", `{"pattern":"Pagamentos · {mes} {ano}"}`, nil)
	var body TabNamePreviewResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || body.Title != "Pagamentos · novembro 2026" || body.Period != "2026-11" {
		t.Fatalf("status %d body %+v", rec.Code, body)
	}
	rec = serve(h, http.MethodPost, "/google-sheets/tab-name-preview", `{"pattern":"Pagamentos"}`, nil)
	if code, details := codeOf(t, rec); rec.Code != http.StatusBadRequest || code != googlesheet.CodeTabPatternInvalid || details["field"] != "tabPattern" {
		t.Fatalf("status %d code %s details %v", rec.Code, code, details)
	}
}

func TestPresetsAreServedWithTheirStructure(t *testing.T) {
	rec := serve(NewHandler(&stubSheets{}, &stubRows{}), http.MethodGet, "/google-sheets/presets", "", nil)
	var body PresetListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var cobranca *PresetResponse
	for i := range body.Items {
		if body.Items[i].Key == "cobranca" {
			cobranca = &body.Items[i]
		}
	}
	if cobranca == nil || len(cobranca.Sheets) != 2 {
		t.Fatalf("presets = %+v", body.Items)
	}
	pay := cobranca.Sheets[1]
	found := false
	for _, col := range pay.Columns {
		if col.Source == "export_link" && col.LinkRef == "negociacoes" {
			found = true
		}
	}
	if !found || pay.TabPeriod != "month" || pay.Style == "" {
		t.Fatalf("payments = %+v", pay)
	}
}

func TestTheCatalogCarriesStylesColoursAndPeriods(t *testing.T) {
	rec := serve(NewHandler(&stubSheets{}, &stubRows{}), http.MethodGet, "/google-sheets/catalog", "", nil)
	var body CatalogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Styles) != len(googlesheet.StylePresets()) || len(body.OptionColors) != len(googlesheet.OptionColors()) || body.DefaultStyle != string(googlesheet.DefaultStyle) {
		t.Fatalf("catalog = %+v", body)
	}
	if len(body.TabPeriods) != 2 || len(body.TabTokens) != 3 || body.MaxLinkLabel != googlesheet.MaxLinkLabelRunes || body.DefaultLinkLabel == "" {
		t.Fatalf("catalog = %+v", body)
	}
	for _, typ := range body.Types {
		if typ == "link" {
			t.Fatal("link is not a type a person picks for an input column")
		}
	}
	linkSource := false
	for _, source := range body.Sources {
		linkSource = linkSource || (source.Source == "export_link" && source.Type == "link")
	}
	if !linkSource {
		t.Fatal("the link source is missing from the catalog")
	}
	if !strings.HasPrefix(body.Styles[1].Header, "#") {
		t.Fatalf("styles = %+v", body.Styles)
	}
}

func TestPresetCopyKeysMatchTheGoldenSoTheFrontTranslatesThem(t *testing.T) {
	codegolden.Compare(t, filepath.Join("testdata", "preset_keys.golden"), googlesheet.PresetCopyKeys(),
		"update testdata/preset_keys.golden, and also translate googleSheets.presets.<key> in the vozko-front pt, en, es and de catalogs")
}

func TestFilesListEachGoogleFileOnceWithItsTabs(t *testing.T) {
	written := time.Date(2026, 10, 27, 9, 30, 0, 0, time.UTC)
	negotiations := &googlesheet.Sheet{ID: "neg", SpreadsheetID: "sp-1", Title: "Altiva: negociações", TabName: "Negociações", Status: googlesheet.StatusActive, LastWrittenAt: &written, CreatedAt: written.Add(-time.Hour)}
	payments := &googlesheet.Sheet{ID: "pag", SpreadsheetID: "sp-1", Title: "Altiva: pagamentos", TabName: "Pagamentos · outubro 2026", TabID: 7, Status: googlesheet.StatusNeedsAttention, CreatedAt: written}
	stub := &stubSheets{files: []googlesheet.Spreadsheet{{ID: "sp-1", Sheets: []*googlesheet.Sheet{negotiations, payments}}}}
	rec := serve(NewHandler(stub, &stubRows{}), http.MethodGet, "/google-sheets/files?search=%20altiva%20&page=2&pageSize=10", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if stub.list.Search != "altiva" || stub.list.Pagination.Page != 2 || stub.list.Pagination.PageSize != 10 || stub.list.TakesInput {
		t.Fatalf("input = %+v", stub.list)
	}
	var body SheetFileListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Page != 2 || body.TotalItems != 11 || body.TotalPages != 2 || len(body.Items) != 1 {
		t.Fatalf("page = %+v", body)
	}
	file := body.Items[0]
	if file.SpreadsheetID != "sp-1" || file.Title != "Altiva: negociações" || file.URL != "https://docs.google.com/spreadsheets/d/sp-1/edit" {
		t.Fatalf("file = %+v", file)
	}
	if file.Status != "needs_attention" || file.LastWrittenAt == nil || !file.LastWrittenAt.Equal(written) {
		t.Fatalf("file situation = %q, last written %v", file.Status, file.LastWrittenAt)
	}
	if len(file.Sheets) != 2 || file.Sheets[0].ID != "neg" || file.Sheets[1].ID != "pag" || file.Sheets[1].Status != "needs_attention" || file.Sheets[1].URL != "https://docs.google.com/spreadsheets/d/sp-1/edit#gid=7" {
		t.Fatalf("tabs = %+v", file.Sheets)
	}
}

func TestFilesAnswersAnEmptyPageWithAnEmptyList(t *testing.T) {
	rec := serve(NewHandler(&stubSheets{}, &stubRows{}), http.MethodGet, "/google-sheets/files", "", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestTheFilesRouteIsNotTakenForASheetID(t *testing.T) {
	stub := &stubSheets{}
	router := mux.NewRouter()
	pass := func(_ workspace_domain.Resource, _ workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		return next
	}
	RegisterProtectedRoutes(router, NewHandler(stub, &stubRows{}), pass)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/google-sheets/files", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestFilesCanBeNarrowedToOneFileOrToFilesWithInputTabs(t *testing.T) {
	stub := &stubSheets{}
	serve(NewHandler(stub, &stubRows{}), http.MethodGet, "/google-sheets/files?spreadsheetId=%20sp-1%20&takesInput=true", "", nil)
	if stub.list.SpreadsheetID != "sp-1" || !stub.list.TakesInput {
		t.Fatalf("input = %+v", stub.list)
	}
	serve(NewHandler(stub, &stubRows{}), http.MethodGet, "/google-sheets/files?takesMessages=true", "", nil)
	if !stub.list.TakesMessages || stub.list.TakesInput {
		t.Fatalf("input = %+v", stub.list)
	}
}

func TestCreatePassesTheNameOfTheNewFile(t *testing.T) {
	stub := &stubSheets{sheet: sampleSheet()}
	serve(NewHandler(stub, &stubRows{}), http.MethodPost, "/google-sheets", `{"fileTitle":"CDT Jabaquara","title":"Leads","columns":[{"header":"Nome","type":"text"}]}`, nil)
	if stub.created.FileTitle != "CDT Jabaquara" || stub.created.Title != "Leads" {
		t.Fatalf("input = %+v", stub.created)
	}
	serve(NewHandler(stub, &stubRows{}), http.MethodPost, "/google-sheets/workbook", `{"fileTitle":"Altiva","sheets":[{"title":"Negociações"}]}`, nil)
	if stub.workbook.FileTitle != "Altiva" {
		t.Fatalf("workbook = %+v", stub.workbook)
	}
}

func TestRenameFileAnswersTheFileWithItsNewName(t *testing.T) {
	sheet := sampleSheet()
	stub := &stubSheets{renamed: &googlesheet.Spreadsheet{ID: "sp-1", Name: "Altiva", Sheets: []*googlesheet.Sheet{sheet}}}
	rec := serve(NewHandler(stub, &stubRows{}), http.MethodPatch, "/google-sheets/files/sp-1", `{"title":"Altiva"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if stub.rename.SpreadsheetID != "sp-1" || stub.rename.Title != "Altiva" {
		t.Fatalf("input = %+v", stub.rename)
	}
	var body SheetFileEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.File.SpreadsheetID != "sp-1" || body.File.Title != "Altiva" || len(body.File.Sheets) != 1 {
		t.Fatalf("body = %+v", body)
	}
}

func TestRenameFileRefusalsAreCoded(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"blank", &googlesheet.DefinitionError{Code: googlesheet.CodeFileTitleRequired, Field: googlesheet.FieldFileTitle, Column: -1}, http.StatusBadRequest, googlesheet.CodeFileTitleRequired},
		{"unknown file", googlesheet.ErrSpreadsheetNotFound, 0, googlesheet.ErrorCode(googlesheet.ErrSpreadsheetNotFound)},
		{"another account", googlesheet.ErrAccountChanged, 0, googlesheet.ErrorCode(googlesheet.ErrAccountChanged)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := serve(NewHandler(&stubSheets{err: c.err}, &stubRows{}), http.MethodPatch, "/google-sheets/files/sp-1", `{"title":"x"}`, nil)
			code, details := codeOf(t, rec)
			if code != c.code || (c.status != 0 && rec.Code != c.status) || rec.Code < 400 {
				t.Fatalf("status %d code %s details %v", rec.Code, code, details)
			}
			if c.name == "blank" && details["field"] != "fileTitle" {
				t.Fatalf("details = %v", details)
			}
		})
	}
}
