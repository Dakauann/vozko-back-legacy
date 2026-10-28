package sheetvariable

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/googlesheet"
	"vozko/domain/sheetfit"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

type staticFiles struct {
	ws  sheetfit.Workspace
	err error
}

func (s staticFiles) FitWorkspace(context.Context, string) (sheetfit.Workspace, error) {
	return s.ws, s.err
}

var created = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func tab(id, spreadsheet, title string, cols ...googlesheet.Column) *googlesheet.Sheet {
	return &googlesheet.Sheet{ID: id, SpreadsheetID: spreadsheet, Title: title, AccountID: "acct-1", Status: googlesheet.StatusActive, Columns: cols, CreatedAt: created}
}

func valor() googlesheet.Column {
	return googlesheet.Column{Key: "valor", Header: "Valor", Type: googlesheet.TypeMoney, Source: googlesheet.SourceInput}
}

func obs() googlesheet.Column {
	return googlesheet.Column{Key: "obs", Header: "Obs", Type: googlesheet.TypeText, Source: googlesheet.SourceInput}
}

func workspace() sheetfit.Workspace {
	return sheetfit.Workspace{Account: "acct-1", Files: []googlesheet.Spreadsheet{
		{ID: "sp-model", Name: "Modelo Altiva", Sheets: []*googlesheet.Sheet{tab("m-pag", "sp-model", "Pagamentos", valor(), obs())}},
		{ID: "sp-centro", Name: "Centro", Sheets: []*googlesheet.Sheet{tab("c-pag", "sp-centro", "Pagamentos", valor(), obs())}},
		{ID: "sp-norte", Name: "Norte", Sheets: []*googlesheet.Sheet{tab("n-pag", "sp-norte", "Pagamentos", valor())}},
	}}
}

func sheetType(settings map[string]string) variabledecl.Type {
	return variabledecl.Type{Kind: variabledecl.KindSheet, Settings: settings}
}

func service(t *testing.T, files Files) *Service {
	t.Helper()
	s, err := NewService(files)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestThePlanilhaTypeIsDeclaredWithAModelFile(t *testing.T) {
	s := service(t, staticFiles{ws: workspace()})
	if s.Kind() != variabledecl.KindSheet || s.Shape() != variabledecl.ShapeSheet {
		t.Fatal("kind or shape")
	}
	if err := s.CheckType(sheetType(map[string]string{variabledecl.SettingModel: "sp-model", variabledecl.SettingModelTitle: "Modelo Altiva"})); err != nil {
		t.Fatalf("a model = %v", err)
	}
	for name, typ := range map[string]variabledecl.Type{
		"no model":        sheetType(nil),
		"options":         {Kind: variabledecl.KindSheet, Options: []string{"a"}, Settings: map[string]string{variabledecl.SettingModel: "sp-model"}},
		"unknown setting": sheetType(map[string]string{variabledecl.SettingModel: "sp-model", "cor": "verde"}),
	} {
		if err := s.CheckType(typ); !errors.Is(err, variabledecl.ErrTypeInvalid) {
			t.Fatalf("%s = %v", name, err)
		}
	}
	if s.Sample(sheetType(map[string]string{variabledecl.SettingModelTitle: "Modelo Altiva"})) != "Modelo Altiva" || s.Sample(sheetType(nil)) == "" {
		t.Fatal("sample")
	}
}

func TestPreparingReadsTheModelTitleAndChecksTheDefault(t *testing.T) {
	s := service(t, staticFiles{ws: workspace()})
	got, err := s.Prepare(context.Background(), "ws", sheetType(map[string]string{variabledecl.SettingModel: "sp-model", variabledecl.SettingDefault: "sp-centro"}))
	if err != nil || got.Settings[variabledecl.SettingModelTitle] != "Modelo Altiva" || got.Settings[variabledecl.SettingDefault] != "sp-centro" {
		t.Fatalf("Prepare = %+v, %v", got, err)
	}
	for name, settings := range map[string]map[string]string{
		"model not in the workspace": {variabledecl.SettingModel: "sp-gone"},
		"default that does not fit":  {variabledecl.SettingModel: "sp-model", variabledecl.SettingDefault: "sp-norte"},
		"the model as default":       {variabledecl.SettingModel: "sp-model", variabledecl.SettingDefault: "sp-model"},
	} {
		if _, err := s.Prepare(context.Background(), "ws", sheetType(settings)); !errors.Is(err, variabledecl.ErrTypeInvalid) {
			t.Fatalf("%s = %v", name, err)
		}
	}
	broken := service(t, staticFiles{err: errors.New("db down")})
	if _, err := broken.Prepare(context.Background(), "ws", sheetType(map[string]string{variabledecl.SettingModel: "sp-model"})); !errors.Is(err, variabledecl.ErrUnreadable) {
		t.Fatalf("unreadable = %v", err)
	}
}

func TestAValueIsAFileThatFitsTheWholeModel(t *testing.T) {
	s := service(t, staticFiles{ws: workspace()})
	typ := sheetType(map[string]string{variabledecl.SettingModel: "sp-model"})
	check := func(value, workspaceID string) error {
		return s.Accepts(context.Background(), variabledecl.Check{WorkspaceID: workspaceID, Name: "planilha", Type: typ, Value: value})
	}
	if err := check("sp-centro", "ws"); err != nil {
		t.Fatalf("a fitting file = %v", err)
	}
	for name, value := range map[string]string{"missing column": "sp-norte", "unknown file": "sp-gone", "the model": "sp-model", "blank": " "} {
		if err := check(value, "ws"); !errors.Is(err, variabledecl.ErrValueInvalid) {
			t.Fatalf("%s = %v", name, err)
		}
	}
	if err := check("sp-centro", ""); !errors.Is(err, variabledecl.ErrValueInvalid) {
		t.Fatalf("no workspace = %v", err)
	}
	broken := service(t, staticFiles{err: errors.New("db down")})
	err := broken.Accepts(context.Background(), variabledecl.Check{WorkspaceID: "ws", Type: typ, Value: "sp-centro"})
	if err == nil || errors.Is(err, variabledecl.ErrValueInvalid) {
		t.Fatalf("a port failure must not decide = %v", err)
	}
}

func TestResolvingFitsTheExactUsesAndReturnsTheTabMap(t *testing.T) {
	s := service(t, staticFiles{ws: workspace()})
	typ := sheetType(map[string]string{variabledecl.SettingModel: "sp-model"})
	uses := []sheetfit.Use{{ModelSheetID: "m-pag", Kind: sheetfit.UseAddRow, Mapping: map[string]string{"valor": "1"}}}
	file, err := s.Resolve(context.Background(), "ws", typ, uses, "sp-norte")
	if err != nil || file.SpreadsheetID != "sp-norte" || file.Tabs["m-pag"] != "n-pag" {
		t.Fatalf("Resolve = %+v, %v", file, err)
	}
	var misfit *sheetfit.Misfit
	if _, err := s.Resolve(context.Background(), "ws", typ, uses, "sp-gone"); !errors.As(err, &misfit) || misfit.Code != sheetfit.CodeFileNotFound {
		t.Fatalf("unknown file = %v", err)
	}
	if _, err := s.Resolve(context.Background(), "ws", sheetType(map[string]string{variabledecl.SettingModel: "sp-gone"}), uses, "sp-norte"); !errors.As(err, &misfit) || misfit.Code != sheetfit.CodeModelMissing {
		t.Fatalf("unknown model = %v", err)
	}
	candidates, err := s.Candidates(context.Background(), "ws", typ, uses)
	if err != nil || len(candidates) != 3 || candidates[0].Misfit != nil || candidates[2].Misfit == nil {
		t.Fatalf("Candidates = %+v, %v", candidates, err)
	}
}

func TestWithoutUsesTheCandidatesFitTheWholeModel(t *testing.T) {
	s := service(t, staticFiles{ws: workspace()})
	got, err := s.Candidates(context.Background(), "ws", sheetType(map[string]string{variabledecl.SettingModel: "sp-model"}), nil)
	if err != nil || len(got) != 3 || got[0].File.ID != "sp-centro" || got[1].Misfit == nil {
		t.Fatalf("Candidates = %+v, %v", got, err)
	}
}

func TestAttentionLooksOnlyAtTheTabsTheRunWritesTo(t *testing.T) {
	ws := workspace()
	ws.Files[1].Sheets = append(ws.Files[1].Sheets, tab("c-old", "sp-centro", "Antiga", valor()))
	ws.Files[1].Sheets[1].NeedsAttention(googlesheet.ReasonAccessLost, "")
	s := service(t, staticFiles{ws: ws})
	calm, err := s.Attention(context.Background(), "ws", workflowFile("sp-centro", "c-pag"))
	if err != nil || calm {
		t.Fatalf("an unused tab in attention = %v, %v", calm, err)
	}
	ws.Files[1].Sheets[0].NeedsAttention(googlesheet.ReasonHeaderMismatch, "")
	if busy, err := service(t, staticFiles{ws: ws}).Attention(context.Background(), "ws", workflowFile("sp-centro", "c-pag")); err != nil || !busy {
		t.Fatalf("a used tab in attention = %v, %v", busy, err)
	}
	if _, err := service(t, staticFiles{err: errors.New("down")}).Attention(context.Background(), "ws", workflowFile("sp-centro", "c-pag")); err == nil {
		t.Fatal("an unreadable workspace must not look calm")
	}
}

func workflowFile(spreadsheetID, tab string) workflow.SheetFile {
	return workflow.SheetFile{SpreadsheetID: spreadsheetID, Tabs: map[string]string{"m-pag": tab}}
}
