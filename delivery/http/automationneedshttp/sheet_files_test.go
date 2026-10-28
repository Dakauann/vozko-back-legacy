package automationneedshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/googlesheet"
	"vozko/domain/sheetfit"
	"vozko/domain/variabledecl"
	automationneeds_usecase "vozko/usecases/automationneeds"
)

func (f *fakeQueries) SheetCandidates(_ context.Context, _ string, target *conversation.Automation, name string) (automationneeds_usecase.SheetCandidates, error) {
	f.formOf = append(f.formOf, target)
	if f.err != nil {
		return automationneeds_usecase.SheetCandidates{}, f.err
	}
	return automationneeds_usecase.SheetCandidates{
		Variable: variabledecl.Declaration{Name: name, Label: "Planilha da franquia", Type: variabledecl.Type{Kind: variabledecl.KindSheet, Settings: map[string]string{
			variabledecl.SettingModel: "sp-model", variabledecl.SettingModelTitle: "Modelo Altiva", variabledecl.SettingDefault: "sp-centro",
		}}},
		Exact: target != nil,
		Candidates: []sheetfit.Candidate{
			{File: googlesheet.Spreadsheet{ID: "sp-centro", Name: "Centro", Sheets: []*googlesheet.Sheet{{ID: "c-pag", Title: "Pagamentos", Status: googlesheet.StatusActive}}}, Assignment: sheetfit.Assignment{Tabs: map[string]string{"m-pag": "c-pag"}}},
			{File: googlesheet.Spreadsheet{ID: "sp-norte", Name: "Norte"}, Misfit: &sheetfit.Misfit{Code: sheetfit.CodeTabMissing, ModelTab: "Pagamentos"}},
		},
	}, nil
}

func TestThePlanilhaPickerListsFilesWithTheirFitForTheAutomation(t *testing.T) {
	q := &fakeQueries{}
	rec := serve(t, q, http.MethodGet, "/automation-needs/sheet-files?variable=planilha&kind=workflow&id=wf-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if len(q.formOf) != 1 || q.formOf[0] == nil || q.formOf[0].ID != "wf-1" || q.formOf[0].Kind != conversation.AutomationWorkflow {
		t.Fatalf("target = %+v", q.formOf)
	}
	var out SheetFilesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Exact || out.Variable.Label != "Planilha da franquia" || out.Variable.Model.SpreadsheetID != "sp-model" || out.Variable.Model.Title != "Modelo Altiva" || out.Variable.DefaultSpreadsheetID != "sp-centro" {
		t.Fatalf("variable = %+v exact %v", out.Variable, out.Exact)
	}
	if len(out.Items) != 2 || !out.Items[0].Fits || out.Items[0].Title != "Centro" || out.Items[1].Misfit == nil || out.Items[1].Misfit.Code != sheetfit.CodeTabMissing {
		t.Fatalf("items = %+v", out.Items)
	}
}

func TestThePlanilhaPickerWithoutAnAutomationAndItsRefusals(t *testing.T) {
	q := &fakeQueries{}
	if rec := serve(t, q, http.MethodGet, "/automation-needs/sheet-files?variable=planilha", ""); rec.Code != http.StatusOK || q.formOf[0] != nil {
		t.Fatalf("status = %d target %+v", rec.Code, q.formOf)
	}
	if rec := serve(t, &fakeQueries{}, http.MethodGet, "/automation-needs/sheet-files?variable=planilha&kind=workflow", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("kind without id = %d", rec.Code)
	}
	notSheet := serve(t, &fakeQueries{err: automationneeds_usecase.ErrNotASheet}, http.MethodGet, "/automation-needs/sheet-files?variable=matricula", "")
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(notSheet.Body.Bytes(), &body)
	if notSheet.Code != http.StatusBadRequest || body.Code != "conversation_variable_not_sheet" {
		t.Fatalf("not a Planilha = %d %s", notSheet.Code, body.Code)
	}
	if rec := serve(t, &fakeQueries{err: errors.New("boom")}, http.MethodGet, "/automation-needs/sheet-files?variable=planilha", ""); rec.Code != http.StatusInternalServerError {
		t.Fatalf("failure = %d", rec.Code)
	}
}
