package googlesheet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"vozko/domain/auth"
	"vozko/domain/googleaccount"
	"vozko/domain/googlesheet"
	"vozko/domain/sheetfit"
	"vozko/infra/http/middleware"
	"vozko/pkg/codegolden"
	gsuc "vozko/usecases/googlesheet"
)

type stubModelFits struct {
	fitIn  gsuc.FitFilesInput
	copyIn gsuc.CopyModelInput
	fits   []gsuc.FileFit
	copied *gsuc.ModelCopy
	err    error
}

func (s *stubModelFits) FitFiles(_ context.Context, in gsuc.FitFilesInput) ([]gsuc.FileFit, error) {
	s.fitIn = in
	return s.fits, s.err
}

func (s *stubModelFits) CopyModel(_ context.Context, in gsuc.CopyModelInput) (*gsuc.ModelCopy, error) {
	s.copyIn = in
	return s.copied, s.err
}

func serveModelFits(h *ModelFitHandler, method, path, body string) *httptest.ResponseRecorder {
	router := mux.NewRouter()
	router.HandleFunc("/google-sheets/files/{spreadsheetId}/fit", h.Fit).Methods(http.MethodPost)
	router.HandleFunc("/google-sheets/files/{spreadsheetId}/copies", h.Copy).Methods(http.MethodPost)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, "ws-1")
	ctx = context.WithValue(ctx, middleware.ClaimsContextKey, &auth.Claims{UserID: "user-1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

func fitTab(id, spreadsheet, title string) *googlesheet.Sheet {
	return &googlesheet.Sheet{ID: id, SpreadsheetID: spreadsheet, Title: title, TabName: title, Status: googlesheet.StatusActive}
}

func TestFitListsEveryFileWithItsTabsOrTheReasonItDoesNotFit(t *testing.T) {
	centro := googlesheet.Spreadsheet{ID: "sp-centro", Name: "Franquia Centro", Sheets: []*googlesheet.Sheet{fitTab("c-neg", "sp-centro", "Negociações"), fitTab("c-pag", "sp-centro", "Pagamentos")}}
	norte := googlesheet.Spreadsheet{ID: "sp-norte", Name: "Franquia Norte", Sheets: []*googlesheet.Sheet{fitTab("n-pag", "sp-norte", "Pagamentos")}}
	norte.Sheets[0].NeedsAttention(googlesheet.ReasonAccessLost, "")
	stub := &stubModelFits{fits: []gsuc.FileFit{
		{File: centro, Assignment: sheetfit.Assignment{SpreadsheetID: "sp-centro", Tabs: map[string]string{"m-pag": "c-pag", "m-neg": "c-neg"}}},
		{File: norte, Misfit: &sheetfit.Misfit{Code: sheetfit.CodeTabMissing, ModelSheetID: "m-pag", ModelTab: "Pagamentos", Problems: []sheetfit.Problem{
			{Code: sheetfit.CodeColumnOptions, SheetID: "n-pag", Tab: "Pagamentos", Column: "Baixa", Option: "nao"},
		}}},
	}}
	body := `{"uses":[{"modelSheetId":"m-pag","kind":"add_row","mapping":{"valor":"{{valor}}"}},{"modelSheetId":"m-neg","kind":"export"}]}`
	rec := serveModelFits(NewModelFitHandler(stub), http.MethodPost, "/google-sheets/files/sp-model/fit", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	in := stub.fitIn
	if in.WorkspaceID != "ws-1" || in.ModelSpreadsheetID != "sp-model" || len(in.Uses) != 2 || in.Uses[0].Kind != sheetfit.UseAddRow || in.Uses[0].Mapping["valor"] != "{{valor}}" || in.Uses[1].ModelSheetID != "m-neg" {
		t.Fatalf("input = %+v", in)
	}
	var out FileFitListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 2 {
		t.Fatalf("items = %+v", out.Items)
	}
	first := out.Items[0]
	if !first.Fits || first.Title != "Franquia Centro" || first.Misfit != nil || first.Status != "active" || len(first.Tabs) != 2 {
		t.Fatalf("first = %+v", first)
	}
	if first.Tabs[0] != (FitTabResponse{ModelSheetID: "m-neg", SheetID: "c-neg", Title: "Negociações"}) {
		t.Fatalf("tabs = %+v", first.Tabs)
	}
	second := out.Items[1]
	if second.Fits || second.Status != "needs_attention" || second.Misfit == nil || second.Misfit.Code != sheetfit.CodeTabMissing || second.Misfit.ModelTab != "Pagamentos" {
		t.Fatalf("second = %+v", second)
	}
	if p := second.Misfit.Problems[0]; p.Code != sheetfit.CodeColumnOptions || p.Column != "Baixa" || p.Option != "nao" || p.SheetID != "n-pag" {
		t.Fatalf("problem = %+v", p)
	}
}

func TestFitRefusalsCarryTheirCodes(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		body   string
		status int
		code   string
	}{
		{"invalid uses", &sheetfit.Misfit{Code: sheetfit.CodeModelMissing, ModelSheetID: "gone"}, `{"uses":[{"modelSheetId":"gone","kind":"add_row"}]}`, http.StatusUnprocessableEntity, sheetfit.CodeModelMissing},
		{"unknown model file", googlesheet.ErrSpreadsheetNotFound, `{"uses":[]}`, http.StatusNotFound, googlesheet.CodeSpreadsheetNotFound},
		{"google not connected", googleaccount.ErrNotConnected, `{"uses":[]}`, http.StatusPreconditionFailed, googleaccount.CodeNotConnected},
		{"broken body", nil, `{`, http.StatusBadRequest, "invalid_body"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := serveModelFits(NewModelFitHandler(&stubModelFits{err: c.err}), http.MethodPost, "/google-sheets/files/sp-model/fit", c.body)
			code, details := codeOf(t, rec)
			if rec.Code != c.status || code != c.code {
				t.Fatalf("status = %d code = %s", rec.Code, code)
			}
			if c.code == sheetfit.CodeModelMissing && details["modelSheetId"] != "gone" {
				t.Fatalf("details = %v", details)
			}
		})
	}
}

func TestCopyCreatesTheFileAndSaysWhichTabCameFromWhichModelTab(t *testing.T) {
	file := googlesheet.Spreadsheet{ID: "sp-copy", Name: "Franquia Sul", Sheets: []*googlesheet.Sheet{fitTab("s-neg", "sp-copy", "Negociações"), fitTab("s-pag", "sp-copy", "Pagamentos")}}
	stub := &stubModelFits{copied: &gsuc.ModelCopy{File: file, Tabs: map[string]string{"m-neg": "s-neg", "m-pag": "s-pag"}}}
	rec := serveModelFits(NewModelFitHandler(stub), http.MethodPost, "/google-sheets/files/sp-model/copies", `{"fileTitle":"Franquia Sul"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if stub.copyIn != (gsuc.CopyModelInput{WorkspaceID: "ws-1", UserID: "user-1", ModelSpreadsheetID: "sp-model", FileTitle: "Franquia Sul"}) {
		t.Fatalf("input = %+v", stub.copyIn)
	}
	var out ModelCopyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.File.SpreadsheetID != "sp-copy" || out.File.Title != "Franquia Sul" || len(out.File.Sheets) != 2 || len(out.Tabs) != 2 {
		t.Fatalf("out = %+v", out)
	}
	if out.Tabs[1] != (FitTabResponse{ModelSheetID: "m-pag", SheetID: "s-pag", Title: "Pagamentos"}) {
		t.Fatalf("tabs = %+v", out.Tabs)
	}
}

func TestACopyRefusalUsesTheSheetCodes(t *testing.T) {
	stub := &stubModelFits{err: &gsuc.WorkbookError{Sheet: 1, Err: googlesheet.ErrTabNameTaken}}
	rec := serveModelFits(NewModelFitHandler(stub), http.MethodPost, "/google-sheets/files/sp-model/copies", `{"fileTitle":"X"}`)
	code, details := codeOf(t, rec)
	if rec.Code != http.StatusConflict || code != googlesheet.CodeTabNameTaken || details["sheet"] != 1.0 {
		t.Fatalf("status = %d code = %s details = %v", rec.Code, code, details)
	}
}

func TestSheetFitCodesMatchTheGoldenSoTheFrontTranslatesThem(t *testing.T) {
	codegolden.Compare(t, filepath.Join("testdata", "sheet_fit_codes.golden"), sheetfit.Codes(),
		"update testdata/sheet_fit_codes.golden, and also translate sheetFit.errors.<code> in the vozko-front pt, en, es and de catalogs")
}
