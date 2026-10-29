package googlesheet

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"vozko/domain/googleaccount"
	"vozko/domain/googlesheet"
	"vozko/domain/sheetfit"
)

func franchisePayments(target string) []googlesheet.ColumnInput {
	return []googlesheet.ColumnInput{
		{Header: "Data do pagamento", Type: googlesheet.TypeDate, Required: true},
		{Header: "Matrícula", Type: googlesheet.TypeText},
		{Header: "Valor", Type: googlesheet.TypeMoney, Required: true},
		{Header: "Baixa", Type: googlesheet.TypeChoice, Options: []string{"sim", "nao"}, Colors: map[string]googlesheet.OptionColor{"sim": googlesheet.ColorGreen}},
		{Header: "Antigo", Type: googlesheet.TypeText},
		{Header: "Negociação", Type: googlesheet.TypeLink, Source: googlesheet.SourceExportLink, LinkSheetID: target, LinkLabel: "Ver negociação"},
	}
}

type franchiseModel struct {
	timeline *googlesheet.Sheet
	payments *googlesheet.Sheet
}

func (w *world) storeModel(t *testing.T) franchiseModel {
	t.Helper()
	timeline := w.storeSheet(t, googlesheet.NewSheetInput{Title: "Negociações", Columns: timelineColumns()}, "sp-model", 0)
	payments := w.storeSheet(t, googlesheet.NewSheetInput{
		Title: "Pagamentos", TabPeriod: googlesheet.PeriodMonth, TabPattern: "Pagamentos · {mes} {ano}", Style: googlesheet.StyleGreen,
		Description: "Baixas do mês", TimeZone: "America/Manaus", Columns: franchisePayments(timeline.ID),
		Unique: googlesheet.UniqueInput{Mode: googlesheet.UniqueColumns, Columns: []int{1}},
	}, "sp-model", 7)
	payments.Columns[2].Header = "Valor pago"
	payments.Columns[4].Archived = true
	w.sheets.sheets[payments.ID] = copySheet(payments)
	if err := w.sheets.SaveFileTitle(context.Background(), testWorkspace, "sp-model", "Modelo Altiva"); err != nil {
		t.Fatal(err)
	}
	return franchiseModel{timeline: timeline, payments: payments}
}

func (m franchiseModel) uses() []sheetfit.Use {
	return []sheetfit.Use{
		{ModelSheetID: m.payments.ID, Kind: sheetfit.UseAddRow, Mapping: map[string]string{"data_do_pagamento": "{{data}}", "valor": "{{valor}}", "baixa": "nao"}},
		{ModelSheetID: m.timeline.ID, Kind: sheetfit.UseExport},
	}
}

func (w *world) storeFranchise(t *testing.T, spreadsheetID, account string) (*googlesheet.Sheet, *googlesheet.Sheet) {
	t.Helper()
	timeline := w.storeSheet(t, googlesheet.NewSheetInput{Title: "Negociações", Columns: timelineColumns()}, spreadsheetID, 0)
	columns := franchisePayments(timeline.ID)
	columns[2].Header = "Valor pago"
	columns[2].PreferredKey = "valor"
	payments := w.storeSheet(t, googlesheet.NewSheetInput{Title: "Pagamentos", Columns: columns}, spreadsheetID, 1)
	for _, sheet := range []*googlesheet.Sheet{timeline, payments} {
		sheet.AccountID = account
		w.sheets.sheets[sheet.ID] = copySheet(sheet)
	}
	return timeline, payments
}

func TestFitFilesTellsWhichWorkspaceFilesTheModelUsesCanWriteInto(t *testing.T) {
	w := newWorld(t)
	model := w.storeModel(t)
	timeline, payments := w.storeFranchise(t, "sp-centro", "acct-1")
	w.storeFranchise(t, "sp-norte", "acct-2")
	fits, err := w.manager.FitFiles(context.Background(), FitFilesInput{WorkspaceID: testWorkspace, ModelSpreadsheetID: "sp-model", Uses: model.uses()})
	if err != nil {
		t.Fatalf("FitFiles: %v", err)
	}
	codes := map[string]string{}
	for _, fit := range fits {
		code := ""
		if fit.Misfit != nil {
			code = fit.Misfit.Code
		}
		codes[fit.File.ID] = code
	}
	want := map[string]string{"sp-centro": "", "sp-1": sheetfit.CodeTabMissing, "sp-model": "", "sp-norte": sheetfit.CodeFileOtherAccount}
	if !reflect.DeepEqual(codes, want) {
		t.Fatalf("codes = %v", codes)
	}
	first := fitOf(t, fits, "sp-centro")
	if first.Misfit != nil || first.Assignment.Tabs[model.timeline.ID] != timeline.ID || first.Assignment.Tabs[model.payments.ID] != payments.ID {
		t.Fatalf("first = %+v", first)
	}
	for _, fit := range fits {
		if fit.File.ID == "sp-model" && fit.File.Title() != "Modelo Altiva" {
			t.Fatalf("model file title = %q", fit.File.Title())
		}
	}
}

func TestFitFilesRefusesWhatItCannotCheck(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*world, franchiseModel) FitFilesInput
		check func(error) bool
	}{
		{"unknown model file", func(_ *world, m franchiseModel) FitFilesInput {
			return FitFilesInput{WorkspaceID: testWorkspace, ModelSpreadsheetID: "sp-gone", Uses: m.uses()}
		}, func(err error) bool { return errors.Is(err, googlesheet.ErrSpreadsheetNotFound) }},
		{"model of another workspace", func(_ *world, m franchiseModel) FitFilesInput {
			return FitFilesInput{WorkspaceID: "ws-other", ModelSpreadsheetID: "sp-model", Uses: m.uses()}
		}, func(err error) bool { return errors.Is(err, googlesheet.ErrSpreadsheetNotFound) }},
		{"no uses", func(_ *world, _ franchiseModel) FitFilesInput {
			return FitFilesInput{WorkspaceID: testWorkspace, ModelSpreadsheetID: "sp-model"}
		}, func(err error) bool {
			var misfit *sheetfit.Misfit
			return errors.As(err, &misfit) && misfit.Code == sheetfit.CodeUsesRequired
		}},
		{"a use outside the model file", func(w *world, m franchiseModel) FitFilesInput {
			uses := m.uses()
			uses[0].ModelSheetID = w.sheet.ID
			return FitFilesInput{WorkspaceID: testWorkspace, ModelSpreadsheetID: "sp-model", Uses: uses}
		}, func(err error) bool {
			var misfit *sheetfit.Misfit
			return errors.As(err, &misfit) && misfit.Code == sheetfit.CodeModelMissing
		}},
		{"google not connected", func(w *world, m franchiseModel) FitFilesInput {
			w.access.err = googleaccount.ErrNotConnected
			return FitFilesInput{WorkspaceID: testWorkspace, ModelSpreadsheetID: "sp-model", Uses: m.uses()}
		}, func(err error) bool { return errors.Is(err, googleaccount.ErrNotConnected) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld(t)
			model := w.storeModel(t)
			_, err := w.manager.FitFiles(context.Background(), c.setup(w, model))
			if !c.check(err) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestCopyModelCreatesAFileThatFitsTheModel(t *testing.T) {
	w := newWorld(t)
	model := w.storeModel(t)
	w.api.created = googlesheet.CreatedSpreadsheet{SpreadsheetID: "sp-copy", TabID: 0}
	w.api.workbook = googlesheet.Workbook{Tabs: []googlesheet.TabInfo{{ID: 0, Title: "Negociações", Rows: 100, Columns: 4}}}
	copied, err := w.manager.CopyModel(context.Background(), CopyModelInput{WorkspaceID: testWorkspace, UserID: "user-1", ModelSpreadsheetID: "sp-model", FileTitle: " Franquia Centro "})
	if err != nil {
		t.Fatalf("CopyModel: %v", err)
	}
	if copied.File.ID != "sp-copy" || copied.File.Title() != "Franquia Centro" || len(copied.File.Sheets) != 2 {
		t.Fatalf("copied = %+v", copied.File)
	}
	timeline := copied.File.Sheets[0]
	payments := copied.File.Sheets[1]
	if copied.Tabs[model.timeline.ID] != timeline.ID || copied.Tabs[model.payments.ID] != payments.ID {
		t.Fatalf("tabs = %v", copied.Tabs)
	}
	if len(w.api.specs) != 1 || w.api.specs[0].Title != "Franquia Centro" {
		t.Fatalf("specs = %+v", w.api.specs)
	}
	keys := []string{}
	for _, col := range payments.Columns {
		keys = append(keys, col.Key)
	}
	if !reflect.DeepEqual(keys, []string{"data_do_pagamento", "matricula", "valor", "baixa", "negociacao"}) {
		t.Fatalf("keys = %v", keys)
	}
	link, _ := payments.Column("negociacao")
	if link.LinkSheetID != timeline.ID || link.LinkLabel != "Ver negociação" {
		t.Fatalf("link = %+v", link)
	}
	baixa, _ := payments.Column("baixa")
	if !reflect.DeepEqual(baixa.Options, []string{"sim", "nao"}) || baixa.Colors["sim"] != googlesheet.ColorGreen {
		t.Fatalf("baixa = %+v", baixa)
	}
	if payments.TabPeriod != googlesheet.PeriodMonth || payments.TabPattern != "Pagamentos · {mes} {ano}" || payments.Style != googlesheet.StyleGreen ||
		payments.TimeZone != "America/Manaus" || payments.Description != "Baixas do mês" || payments.CreatedBy != "user-1" {
		t.Fatalf("payments = %+v", payments)
	}
	if payments.Unique.Mode != googlesheet.UniqueColumns || !reflect.DeepEqual(payments.Unique.Columns, []string{"matricula"}) {
		t.Fatalf("unique = %+v", payments.Unique)
	}
	fits, err := w.manager.FitFiles(context.Background(), FitFilesInput{WorkspaceID: testWorkspace, ModelSpreadsheetID: "sp-model", Uses: model.uses()})
	if err != nil || fitOf(t, fits, "sp-copy").Misfit != nil || !reflect.DeepEqual(fitOf(t, fits, "sp-copy").Assignment.Tabs, copied.Tabs) {
		t.Fatalf("the copy does not fit: %+v, %v", fits, err)
	}
}

func TestCopyModelRefusesWhatItCannotCopy(t *testing.T) {
	w := newWorld(t)
	w.storeModel(t)
	if _, err := w.manager.CopyModel(context.Background(), CopyModelInput{WorkspaceID: testWorkspace, ModelSpreadsheetID: "sp-gone", FileTitle: "X"}); !errors.Is(err, googlesheet.ErrSpreadsheetNotFound) {
		t.Fatalf("unknown model = %v", err)
	}
	var def *googlesheet.DefinitionError
	if _, err := w.manager.CopyModel(context.Background(), CopyModelInput{WorkspaceID: testWorkspace, ModelSpreadsheetID: "sp-model", FileTitle: "  "}); !errors.As(err, &def) || def.Code != googlesheet.CodeFileTitleRequired {
		t.Fatalf("blank title = %v", err)
	}
	if len(w.api.specs) != 0 {
		t.Fatal("a refused copy reached Google")
	}
}

func TestFitWorkspaceListsNamedFilesWithTheConnectedAccount(t *testing.T) {
	w := newWorld(t)
	w.storeModel(t)
	ws, err := w.manager.FitWorkspace(context.Background(), testWorkspace)
	if err != nil || ws.Account != "acct-1" || len(ws.Files) != 2 {
		t.Fatalf("FitWorkspace = %+v, %v", ws, err)
	}
	if model, ok := ws.File("sp-model"); !ok || model.Title() != "Modelo Altiva" {
		t.Fatalf("model = %+v %v", model, ok)
	}
	w.access.err = googleaccount.ErrNotConnected
	if ws, err := w.manager.FitWorkspace(context.Background(), testWorkspace); err != nil || ws.Account != "" || len(ws.Files) != 2 {
		t.Fatalf("not connected = %+v, %v", ws, err)
	}
	w.access.err = errors.New("token store down")
	if _, err := w.manager.FitWorkspace(context.Background(), testWorkspace); err == nil {
		t.Fatal("an unreadable account must not look like a disconnected one")
	}
}

func fitOf(t *testing.T, fits []FileFit, spreadsheetID string) FileFit {
	t.Helper()
	for _, fit := range fits {
		if fit.File.ID == spreadsheetID {
			return fit
		}
	}
	t.Fatalf("no fit for %s in %+v", spreadsheetID, fits)
	return FileFit{}
}
