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
	} {
		if _, err := s.Prepare(context.Background(), "ws", sheetType(settings)); !errors.Is(err, variabledecl.ErrTypeInvalid) {
			t.Fatalf("%s = %v", name, err)
		}
	}
	if _, err := s.Prepare(context.Background(), "ws", sheetType(map[string]string{variabledecl.SettingModel: "sp-model", variabledecl.SettingDefault: "sp-model"})); err != nil {
		t.Fatalf("the model as default = %v", err)
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
	for _, value := range []string{"sp-centro", "sp-model"} {
		if err := check(value, "ws"); err != nil {
			t.Fatalf("%s fits = %v", value, err)
		}
	}
	for name, value := range map[string]string{"missing column": "sp-norte", "unknown file": "sp-gone", "blank": " "} {
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
	candidates, err := s.Candidates(context.Background(), "ws", typ, uses, "")
	if err != nil || len(candidates) != 3 || candidates[0].Misfit != nil || candidates[1].Misfit != nil || candidates[2].Misfit != nil {
		t.Fatalf("Candidates = %+v, %v", candidates, err)
	}
}

func TestWithoutUsesTheCandidatesFitTheWholeModel(t *testing.T) {
	s := service(t, staticFiles{ws: workspace()})
	got, err := s.Candidates(context.Background(), "ws", sheetType(map[string]string{variabledecl.SettingModel: "sp-model"}), nil, "")
	if err != nil || len(got) != 3 || got[0].Misfit != nil || got[1].Misfit != nil || got[2].File.ID != "sp-norte" || got[2].Misfit == nil {
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

func sharedWorkspace() sheetfit.Workspace {
	ws := workspace()
	ws.Files = append(ws.Files, googlesheet.Spreadsheet{ID: "sp-franquias", Name: "Franquias", Sheets: []*googlesheet.Sheet{
		tab("f-centro", "sp-franquias", "Centro", valor(), obs()),
		tab("f-norte", "sp-franquias", "Norte", valor(), obs()),
		tab("f-velha", "sp-franquias", "Velha", valor()),
	}})
	return ws
}

func pinned(tab string) string {
	return sheetfit.Choice{SpreadsheetID: "sp-franquias", Tabs: map[string]string{"m-pag": tab}}.Value()
}

func TestAValueCanChooseTheTabOfASharedFile(t *testing.T) {
	s := service(t, staticFiles{ws: sharedWorkspace()})
	typ := sheetType(map[string]string{variabledecl.SettingModel: "sp-model"})
	check := func(value string) error {
		return s.Accepts(context.Background(), variabledecl.Check{WorkspaceID: "ws", Name: "planilha", Type: typ, Value: value})
	}
	if err := check("sp-franquias"); !errors.Is(err, variabledecl.ErrValueInvalid) {
		t.Fatalf("two tabs fit, the plain file is ambiguous = %v", err)
	}
	if err := check(pinned("f-norte")); err != nil {
		t.Fatalf("a chosen tab = %v", err)
	}
	for name, value := range map[string]string{
		"a tab that is gone":           pinned("f-gone"),
		"a tab missing a model column": pinned("f-velha"),
		"a tab of another file":        pinned("c-pag"),
		"a broken value":               `{"spreadsheet_id":`,
	} {
		if err := check(value); !errors.Is(err, variabledecl.ErrValueInvalid) {
			t.Fatalf("%s = %v", name, err)
		}
	}
	uses := []sheetfit.Use{{ModelSheetID: "m-pag", Kind: sheetfit.UseAddRow, Mapping: map[string]string{"valor": "1"}}}
	norte, err := s.Resolve(context.Background(), "ws", typ, uses, pinned("f-norte"))
	if err != nil || norte.SpreadsheetID != "sp-franquias" || norte.Tabs["m-pag"] != "f-norte" {
		t.Fatalf("Resolve = %+v, %v", norte, err)
	}
	var misfit *sheetfit.Misfit
	if _, err := s.Resolve(context.Background(), "ws", typ, uses, pinned("f-gone")); !errors.As(err, &misfit) || misfit.Code != sheetfit.CodeTabNotFound {
		t.Fatalf("a tab removed after the choice = %v", err)
	}
}

func TestADefaultCanChooseTheTabToo(t *testing.T) {
	s := service(t, staticFiles{ws: sharedWorkspace()})
	got, err := s.Prepare(context.Background(), "ws", sheetType(map[string]string{variabledecl.SettingModel: "sp-model", variabledecl.SettingDefault: pinned("f-centro")}))
	if err != nil || got.Settings[variabledecl.SettingDefault] != pinned("f-centro") {
		t.Fatalf("Prepare = %+v, %v", got, err)
	}
	if _, err := s.Prepare(context.Background(), "ws", sheetType(map[string]string{variabledecl.SettingModel: "sp-model", variabledecl.SettingDefault: pinned("f-velha")})); !errors.Is(err, variabledecl.ErrTypeInvalid) {
		t.Fatalf("a default tab that does not fit = %v", err)
	}
}

func TestTheCandidatesFollowTheValueBeingChosen(t *testing.T) {
	s := service(t, staticFiles{ws: sharedWorkspace()})
	typ := sheetType(map[string]string{variabledecl.SettingModel: "sp-model"})
	got, err := s.Candidates(context.Background(), "ws", typ, nil, pinned("f-norte"))
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range got {
		if candidate.File.ID == "sp-franquias" && (candidate.Misfit != nil || candidate.Assignment.Tabs["m-pag"] != "f-norte" || len(candidate.Tabs) != 1 || len(candidate.Tabs[0].Options) != 3) {
			t.Fatalf("the chosen file = %+v", candidate)
		}
	}
	plain, err := s.Candidates(context.Background(), "ws", typ, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range plain {
		if candidate.File.ID == "sp-franquias" && (candidate.Misfit == nil || !candidate.Choosable()) {
			t.Fatalf("an ambiguous file is offered tab by tab = %+v", candidate)
		}
	}
}
