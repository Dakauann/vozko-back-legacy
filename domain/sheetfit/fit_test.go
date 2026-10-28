package sheetfit

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"vozko/domain/googlesheet"
)

const (
	modelFile   = "sp-model"
	centroFile  = "sp-centro"
	account     = "acct-1"
	modelNeg    = "model-neg"
	modelPag    = "model-pag"
	centroNeg   = "centro-neg"
	centroPag   = "centro-pag"
	elsewhereID = "other-neg"
)

var created = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func input(key, header string, t googlesheet.ColumnType) googlesheet.Column {
	return googlesheet.Column{Key: key, Header: header, Type: t, Source: googlesheet.SourceInput}
}

func required(col googlesheet.Column) googlesheet.Column {
	col.Required = true
	return col
}

func system(key, header string, source googlesheet.Source) googlesheet.Column {
	t, _ := source.ColumnType()
	return googlesheet.Column{Key: key, Header: header, Type: t, Source: source}
}

func choice(key, header string, options ...string) googlesheet.Column {
	return googlesheet.Column{Key: key, Header: header, Type: googlesheet.TypeChoice, Source: googlesheet.SourceInput, Options: options}
}

func link(key, header, target string) googlesheet.Column {
	return googlesheet.Column{Key: key, Header: header, Type: googlesheet.TypeLink, Source: googlesheet.SourceExportLink, LinkSheetID: target, LinkLabel: "Ver negociação"}
}

func tab(id, spreadsheet, title string, offset int, cols ...googlesheet.Column) *googlesheet.Sheet {
	return &googlesheet.Sheet{
		ID: id, WorkspaceID: "ws-1", AccountID: account, SpreadsheetID: spreadsheet, Title: title, TabName: title,
		TabPeriod: googlesheet.PeriodNone, Status: googlesheet.StatusActive, Columns: cols, CreatedAt: created.Add(time.Duration(offset) * time.Minute),
	}
}

func timeline(id, spreadsheet, title string, offset int) *googlesheet.Sheet {
	return tab(id, spreadsheet, title, offset,
		system("data", "Data", googlesheet.SourceMessageSentAt),
		system("cliente", "Cliente", googlesheet.SourceLeadName),
		system("mensagem", "Mensagem", googlesheet.SourceMessageText),
	)
}

func payments(id, spreadsheet, title, target string, offset int) *googlesheet.Sheet {
	return tab(id, spreadsheet, title, offset,
		required(input("data_pagamento", "Data do pagamento", googlesheet.TypeDate)),
		input("matricula", "Matrícula", googlesheet.TypeText),
		required(input("valor", "Valor", googlesheet.TypeMoney)),
		choice("baixa", "Baixa", "sim", "nao"),
		input("observacoes", "Observações", googlesheet.TypeLongText),
		link("negociacao", "Negociação", target),
		system("registrado_em", "Registrado em", googlesheet.SourceWrittenAt),
	)
}

func cobrancaModel() Model {
	return Model{
		Sheets: []*googlesheet.Sheet{timeline(modelNeg, modelFile, "Negociações", 0), payments(modelPag, modelFile, "Pagamentos", modelNeg, 1)},
		Uses: []Use{
			{ModelSheetID: modelPag, Kind: UseAddRow, Mapping: map[string]string{
				"data_pagamento": "{{data}}", "matricula": "{{campvars.matricula}}", "valor": "{{valor}}", "baixa": "nao",
			}},
			{ModelSheetID: modelNeg, Kind: UseExport},
		},
	}
}

func centro() googlesheet.Spreadsheet {
	return googlesheet.Spreadsheet{ID: centroFile, Sheets: []*googlesheet.Sheet{
		timeline(centroNeg, centroFile, "Negociações", 10),
		payments(centroPag, centroFile, "Pagamentos", centroNeg, 11),
	}}
}

func column(sheet *googlesheet.Sheet, key string) *googlesheet.Column {
	for i := range sheet.Columns {
		if sheet.Columns[i].Key == key {
			return &sheet.Columns[i]
		}
	}
	panic("no column " + key)
}

func misfitOf(t *testing.T, err error) *Misfit {
	t.Helper()
	var misfit *Misfit
	if !errors.As(err, &misfit) {
		t.Fatalf("err = %v, want a misfit", err)
	}
	return misfit
}

func TestAFranchiseFileMadeFromTheModelFitsEveryModelTab(t *testing.T) {
	got, err := Fit(cobrancaModel(), centro(), account)
	if err != nil {
		t.Fatalf("Fit: %v", err)
	}
	if got.SpreadsheetID != centroFile || got.Tabs[modelNeg] != centroNeg || got.Tabs[modelPag] != centroPag || len(got.Tabs) != 2 {
		t.Fatalf("assignment = %+v", got)
	}
}

func TestAFileTheFlowCannotUseIsRefusedWithACode(t *testing.T) {
	cases := []struct {
		name  string
		model func(*Model)
		file  func(*googlesheet.Spreadsheet)
		acct  string
		code  string
	}{
		{name: "no tab in this workspace", file: func(f *googlesheet.Spreadsheet) { f.Sheets = nil }, code: CodeFileNotFound},
		{name: "made by another account", file: func(f *googlesheet.Spreadsheet) {
			for _, s := range f.Sheets {
				s.AccountID = "acct-2"
			}
		}, code: CodeFileOtherAccount},
		{name: "no account connected", acct: "-", code: CodeFileOtherAccount},
		{name: "the model file itself", file: func(f *googlesheet.Spreadsheet) {
			*f = googlesheet.Spreadsheet{ID: modelFile, Sheets: cobrancaModel().Sheets}
		}, code: CodeModelFile},
		{name: "no uses", model: func(m *Model) { m.Uses = nil }, code: CodeUsesRequired},
		{name: "unknown use kind", model: func(m *Model) { m.Uses[0].Kind = "delete_row" }, code: CodeUseInvalid},
		{name: "use without a model tab", model: func(m *Model) { m.Uses[0].ModelSheetID = " " }, code: CodeUseInvalid},
		{name: "model tab removed", model: func(m *Model) { m.Uses[0].ModelSheetID = "gone" }, code: CodeModelMissing},
		{name: "model link target removed", model: func(m *Model) { m.Sheets = m.Sheets[1:]; m.Uses = m.Uses[:1] }, code: CodeModelMissing},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			model, file, acct := cobrancaModel(), centro(), account
			if c.model != nil {
				c.model(&model)
			}
			if c.file != nil {
				c.file(&file)
			}
			if c.acct == "-" {
				acct = ""
			}
			_, err := Fit(model, file, acct)
			if got := misfitOf(t, err); got.Code != c.code {
				t.Fatalf("code = %s, want %s (%v)", got.Code, c.code, err)
			}
		})
	}
}

func TestATabThatDoesNotMatchTheModelNamesTheFirstProblem(t *testing.T) {
	cases := []struct {
		name     string
		change   func(neg, pag *googlesheet.Sheet)
		modelTab string
		problem  Problem
	}{
		{name: "mapped column missing", change: func(_, pag *googlesheet.Sheet) {
			pag.Columns = slices.DeleteFunc(pag.Columns, func(c googlesheet.Column) bool { return c.Key == "matricula" })
		}, modelTab: "Pagamentos", problem: Problem{Code: CodeColumnMissing, SheetID: centroPag, Tab: "Pagamentos", Column: "Matrícula"}},
		{name: "mapped column archived", change: func(_, pag *googlesheet.Sheet) { column(pag, "matricula").Archived = true },
			modelTab: "Pagamentos", problem: Problem{Code: CodeColumnArchived, SheetID: centroPag, Tab: "Pagamentos", Column: "Matrícula"}},
		{name: "mapped column filled by the system", change: func(_, pag *googlesheet.Sheet) {
			*column(pag, "matricula") = system("matricula", "Matrícula", googlesheet.SourceLeadName)
		}, modelTab: "Pagamentos", problem: Problem{Code: CodeColumnSystem, SheetID: centroPag, Tab: "Pagamentos", Column: "Matrícula"}},
		{name: "a required column the flow does not fill", change: func(_, pag *googlesheet.Sheet) {
			pag.Columns = append(pag.Columns, required(input("cpf", "CPF", googlesheet.TypeText)))
		}, modelTab: "Pagamentos", problem: Problem{Code: CodeRequiredUnmapped, SheetID: centroPag, Tab: "Pagamentos", Column: "CPF"}},
		{name: "a column of another type", change: func(_, pag *googlesheet.Sheet) { column(pag, "valor").Type = googlesheet.TypeNumber },
			modelTab: "Pagamentos", problem: Problem{Code: CodeColumnType, SheetID: centroPag, Tab: "Pagamentos", Column: "Valor"}},
		{name: "a choice without a model option", change: func(_, pag *googlesheet.Sheet) { column(pag, "baixa").Options = []string{"sim"} },
			modelTab: "Pagamentos", problem: Problem{Code: CodeColumnOptions, SheetID: centroPag, Tab: "Pagamentos", Column: "Baixa", Option: "nao"}},
		{name: "an export tab without message columns", change: func(neg, _ *googlesheet.Sheet) {
			neg.Columns = []googlesheet.Column{input("nota", "Nota", googlesheet.TypeText)}
		}, modelTab: "Negociações", problem: Problem{Code: CodeNoMessageColumns, SheetID: centroNeg, Tab: "Negociações"}},
		{name: "link column missing", change: func(_, pag *googlesheet.Sheet) {
			pag.Columns = slices.DeleteFunc(pag.Columns, func(c googlesheet.Column) bool { return c.Key == "negociacao" })
		}, modelTab: "Pagamentos", problem: Problem{Code: CodeLinkMismatch, SheetID: centroPag, Tab: "Pagamentos", Column: "Negociação"}},
		{name: "link column archived", change: func(_, pag *googlesheet.Sheet) { column(pag, "negociacao").Archived = true },
			modelTab: "Pagamentos", problem: Problem{Code: CodeLinkMismatch, SheetID: centroPag, Tab: "Pagamentos", Column: "Negociação"}},
		{name: "an extra link column", change: func(_, pag *googlesheet.Sheet) {
			pag.Columns = append(pag.Columns, link("outra", "Outra conversa", centroNeg))
		}, modelTab: "Pagamentos", problem: Problem{Code: CodeLinkMismatch, SheetID: centroPag, Tab: "Pagamentos", Column: "Outra conversa"}},
		{name: "a link to another file", change: func(_, pag *googlesheet.Sheet) { column(pag, "negociacao").LinkSheetID = elsewhereID },
			modelTab: "Pagamentos", problem: Problem{Code: CodeLinkMismatch, SheetID: centroPag, Tab: "Pagamentos", Column: "Negociação"}},
		{name: "a link to a tab that is not the timeline", change: func(_, pag *googlesheet.Sheet) { column(pag, "negociacao").LinkSheetID = centroPag },
			modelTab: "Pagamentos", problem: Problem{Code: CodeLinkMismatch, SheetID: centroPag, Tab: "Pagamentos", Column: "Negociação"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := centro()
			c.change(file.Sheets[0], file.Sheets[1])
			_, err := Fit(cobrancaModel(), file, account)
			got := misfitOf(t, err)
			if got.Code != CodeTabMissing || got.ModelTab != c.modelTab {
				t.Fatalf("misfit = %+v", got)
			}
			if len(got.Problems) == 0 || got.Problems[0] != c.problem {
				t.Fatalf("problems = %+v, want first %+v", got.Problems, c.problem)
			}
		})
	}
}

func TestDifferencesThatDoNotBreakTheFlowStillFit(t *testing.T) {
	cases := []struct {
		name   string
		change func(neg, pag *googlesheet.Sheet)
	}{
		{name: "more choice options", change: func(_, pag *googlesheet.Sheet) { column(pag, "baixa").Options = []string{"sim", "nao", "parcial"} }},
		{name: "options in another case", change: func(_, pag *googlesheet.Sheet) { column(pag, "baixa").Options = []string{"SIM", "NAO"} }},
		{name: "renamed headers keep their keys", change: func(_, pag *googlesheet.Sheet) { column(pag, "valor").Header = "Valor pago" }},
		{name: "monthly tabs", change: func(_, pag *googlesheet.Sheet) {
			pag.TabPeriod, pag.TabPattern = googlesheet.PeriodMonth, "Pagamentos · {mes} {ano}"
		}},
		{name: "an extra optional column", change: func(_, pag *googlesheet.Sheet) {
			pag.Columns = append(pag.Columns, input("unidade", "Unidade", googlesheet.TypeText))
		}},
		{name: "an unmapped model column missing", change: func(_, pag *googlesheet.Sheet) {
			pag.Columns = slices.DeleteFunc(pag.Columns, func(c googlesheet.Column) bool { return c.Key == "observacoes" })
		}},
		{name: "a tab that needs attention", change: func(_, pag *googlesheet.Sheet) {
			pag.NeedsAttention(googlesheet.ReasonHeaderMismatch, "coluna B")
		}},
		{name: "tab names chosen by the franchise", change: func(neg, pag *googlesheet.Sheet) { neg.Title, pag.Title = "Conversas", "Baixas" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := centro()
			c.change(file.Sheets[0], file.Sheets[1])
			got, err := Fit(cobrancaModel(), file, account)
			if err != nil || got.Tabs[modelPag] != centroPag || got.Tabs[modelNeg] != centroNeg {
				t.Fatalf("Fit = %+v, %v", got, err)
			}
		})
	}
}

func TestTheLinkDecidesWhichTimelineAPaymentTabBelongsTo(t *testing.T) {
	file := centro()
	file.Sheets[0].Title = "Conversas"
	file.Sheets = append(file.Sheets, timeline("centro-neg-2", centroFile, "Histórico", 12))
	got, err := Fit(cobrancaModel(), file, account)
	if err != nil || got.Tabs[modelNeg] != centroNeg || got.Tabs[modelPag] != centroPag {
		t.Fatalf("Fit = %+v, %v", got, err)
	}
}

func TestALinkTargetIsAssignedEvenWhenNoStepExportsToIt(t *testing.T) {
	model := cobrancaModel()
	model.Uses = model.Uses[:1]
	got, err := Fit(model, centro(), account)
	if err != nil || got.Tabs[modelNeg] != centroNeg || got.Tabs[modelPag] != centroPag {
		t.Fatalf("Fit = %+v, %v", got, err)
	}
}

func TestTwoTabsThatBothFitAreSettledByTheModelTabName(t *testing.T) {
	file := centro()
	file.Sheets = append(file.Sheets, payments("centro-pag-old", centroFile, "Pagamentos antigos", centroNeg, 12))
	got, err := Fit(cobrancaModel(), file, account)
	if err != nil || got.Tabs[modelPag] != centroPag {
		t.Fatalf("Fit = %+v, %v", got, err)
	}
	file.Sheets[1].Title = "Pagamentos novos"
	_, err = Fit(cobrancaModel(), file, account)
	got2 := misfitOf(t, err)
	if got2.Code != CodeTabAmbiguous || got2.ModelTab != "Pagamentos" || len(got2.Candidates) != 2 {
		t.Fatalf("misfit = %+v", got2)
	}
	if got2.Candidates[0] != (Tab{SheetID: centroPag, Title: "Pagamentos novos"}) {
		t.Fatalf("candidates = %+v", got2.Candidates)
	}
}

func TestTwoModelTabsCannotWriteIntoOneFileTab(t *testing.T) {
	model := Model{
		Sheets: []*googlesheet.Sheet{
			tab("model-a", modelFile, "Centro", 0, input("nome", "Nome", googlesheet.TypeText)),
			tab("model-b", modelFile, "Norte", 1, input("nome", "Nome", googlesheet.TypeText)),
		},
		Uses: []Use{
			{ModelSheetID: "model-a", Kind: UseAddRow, Mapping: map[string]string{"nome": "x"}},
			{ModelSheetID: "model-b", Kind: UseAddRow, Mapping: map[string]string{"nome": "y"}},
		},
	}
	file := googlesheet.Spreadsheet{ID: centroFile, Sheets: []*googlesheet.Sheet{tab("only", centroFile, "Registros", 0, input("nome", "Nome", googlesheet.TypeText))}}
	_, err := Fit(model, file, account)
	got := misfitOf(t, err)
	want := Problem{Code: CodeTabShared, SheetID: "only", Tab: "Registros", ModelTab: "Centro"}
	if got.Code != CodeTabShared || got.ModelTab != "Norte" || len(got.Problems) != 1 || got.Problems[0] != want {
		t.Fatalf("misfit = %+v", got)
	}
}

func TestTheTabNamedLikeTheModelIsReportedFirst(t *testing.T) {
	file := centro()
	file.Sheets = []*googlesheet.Sheet{
		tab("notes", centroFile, "Anotações", 9, input("nota", "Nota", googlesheet.TypeText)),
		file.Sheets[0],
		file.Sheets[1],
	}
	column(file.Sheets[2], "valor").Type = googlesheet.TypeText
	_, err := Fit(cobrancaModel(), file, account)
	got := misfitOf(t, err)
	if got.ModelTab != "Pagamentos" || len(got.Problems) < 2 || got.Problems[0].SheetID != centroPag || got.Problems[0].Code != CodeColumnType {
		t.Fatalf("misfit = %+v", got)
	}
}

func TestEveryCodeIsListedOnce(t *testing.T) {
	codes := Codes()
	seen := map[string]bool{}
	for _, code := range codes {
		if seen[code] || code == "" {
			t.Fatalf("code %q repeated or blank", code)
		}
		seen[code] = true
		if (&Misfit{Code: code}).Error() == "" {
			t.Fatalf("code %q has no message", code)
		}
	}
}

func TestAnAgentRowNeedsEveryInputColumnOfTheModel(t *testing.T) {
	model := cobrancaModel()
	model.Uses = []Use{{ModelSheetID: modelPag, Kind: UseAgentRow}}
	if _, err := Fit(model, centro(), account); err != nil {
		t.Fatalf("a full copy = %v", err)
	}
	file := centro()
	file.Sheets[1].Columns = slices.DeleteFunc(file.Sheets[1].Columns, func(c googlesheet.Column) bool { return c.Key == "observacoes" })
	_, err := Fit(model, file, account)
	if got := misfitOf(t, err); got.Code != CodeTabMissing || got.Problems[0].Code != CodeColumnMissing || got.Problems[0].Column != "Observações" {
		t.Fatalf("misfit = %+v", got)
	}
}

func TestTheWholeModelUsesEveryTabTheWayItCanBeWritten(t *testing.T) {
	notes := tab("model-notes", modelFile, "Notas", 2, system("cliente", "Cliente", googlesheet.SourceLeadName))
	sheets := append(cobrancaModel().Sheets, notes)
	got := WholeModel(sheets)
	want := []Use{
		{ModelSheetID: modelNeg, Kind: UseExport},
		{ModelSheetID: modelPag, Kind: UseAgentRow},
		{ModelSheetID: "model-notes", Kind: UseAddRow},
	}
	if len(got.Sheets) != 3 || !reflect.DeepEqual(got.Uses, want) {
		t.Fatalf("uses = %+v", got.Uses)
	}
	if _, err := Fit(got, centro(), account); misfitOf(t, err).ModelTab != "Notas" {
		t.Fatalf("a file without the notes tab = %v", err)
	}
}

func TestTheModelFileIsNeverAValue(t *testing.T) {
	model := cobrancaModel()
	if !IsModelFile(model, googlesheet.Spreadsheet{ID: modelFile}) || IsModelFile(model, centro()) {
		t.Fatal("the model file rule is wrong")
	}
}

func TestAWorkspaceFindsItsFiles(t *testing.T) {
	ws := Workspace{Files: []googlesheet.Spreadsheet{centro()}, Account: account}
	if file, ok := ws.File(centroFile); !ok || file.ID != centroFile {
		t.Fatalf("File = %+v %v", file, ok)
	}
	if _, ok := ws.File("gone"); ok {
		t.Fatal("found a file that is not there")
	}
	if model, ok := ws.Model(centroFile); !ok || len(model.Sheets) != 2 {
		t.Fatalf("Model = %+v %v", model, ok)
	}
}

func TestCandidatesPutTheFilesThatFitFirst(t *testing.T) {
	model := cobrancaModel()
	ws := Workspace{Account: account, Files: []googlesheet.Spreadsheet{{ID: modelFile, Sheets: model.Sheets}, centro()}}
	got, err := Candidates(ws, model)
	if err != nil || len(got) != 2 || got[0].File.ID != centroFile || got[0].Misfit != nil || got[1].Misfit.Code != CodeModelFile {
		t.Fatalf("Candidates = %+v, %v", got, err)
	}
	if _, err := Candidates(ws, Model{Sheets: model.Sheets}); err == nil {
		t.Fatal("a model without uses must be refused, not listed")
	}
}
