package sheetfit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"vozko/domain/googlesheet"
)

type choiceGolden struct {
	Value         string            `json:"value"`
	SpreadsheetID string            `json:"spreadsheetId"`
	Tabs          map[string]string `json:"tabs"`
}

func TestTheValueFormatIsSharedWithTheFront(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "choice_values.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []choiceGolden
	if err := json.Unmarshal(raw, &cases); err != nil || len(cases) == 0 {
		t.Fatalf("golden = %v", err)
	}
	for _, c := range cases {
		got, err := ParseChoice(c.Value)
		if err != nil || got.SpreadsheetID != c.SpreadsheetID || len(got.Tabs) != len(c.Tabs) {
			t.Fatalf("ParseChoice(%q) = %+v, %v", c.Value, got, err)
		}
		for model, sheet := range c.Tabs {
			if got.Tabs[model] != sheet {
				t.Fatalf("ParseChoice(%q) tabs = %+v", c.Value, got.Tabs)
			}
		}
		if got.Value() != c.Value {
			t.Fatalf("Value() = %q, want %q", got.Value(), c.Value)
		}
	}
}

func TestAPlainSpreadsheetValueKeepsMeaningTheAutomaticMatch(t *testing.T) {
	got, err := ParseChoice("  sp-centro ")
	if err != nil || got.SpreadsheetID != "sp-centro" || len(got.Tabs) != 0 {
		t.Fatalf("ParseChoice = %+v, %v", got, err)
	}
	empty, err := ParseChoice(`{"spreadsheet_id":"sp-centro","tabs":{}}`)
	if err != nil || empty.Value() != "sp-centro" {
		t.Fatalf("no tabs = %+v, %v", empty, err)
	}
}

func TestABrokenValueIsRefusedAsAFileThatIsNotThere(t *testing.T) {
	for name, value := range map[string]string{
		"blank":            " ",
		"not json":         "{spreadsheet",
		"no spreadsheet":   `{"tabs":{"model-pag":"norte-pag"}}`,
		"blank tab":        `{"spreadsheet_id":"sp","tabs":{"model-pag":" "}}`,
		"blank model tab":  `{"spreadsheet_id":"sp","tabs":{" ":"norte-pag"}}`,
		"an unknown field": `{"spreadsheet_id":"sp","sheet":"x"}`,
	} {
		_, err := ParseChoice(value)
		if got := misfitOf(t, err); got.Code != CodeFileNotFound {
			t.Fatalf("%s = %+v", name, got)
		}
	}
}

const (
	sharedFile = "sp-franquias"
	centroNegT = "f-centro-neg"
	centroPagT = "f-centro-pag"
	norteNegT  = "f-norte-neg"
	nortePagT  = "f-norte-pag"
)

func franquias() googlesheet.Spreadsheet {
	return googlesheet.Spreadsheet{ID: sharedFile, Sheets: []*googlesheet.Sheet{
		timeline(centroNegT, sharedFile, "Negociações Centro", 10),
		payments(centroPagT, sharedFile, "Pagamentos Centro", centroNegT, 11),
		timeline(norteNegT, sharedFile, "Negociações Norte", 12),
		payments(nortePagT, sharedFile, "Pagamentos Norte", norteNegT, 13),
	}}
}

func TestTwoCampaignsWriteToTwoTabsOfTheSameFile(t *testing.T) {
	if _, err := Fit(cobrancaModel(), franquias(), account); misfitOf(t, err).Code != CodeTabAmbiguous {
		t.Fatalf("without a choice the file is ambiguous = %v", err)
	}
	centro, err := FitPinned(cobrancaModel(), franquias(), account, map[string]string{modelPag: centroPagT})
	if err != nil || centro.Tabs[modelPag] != centroPagT || centro.Tabs[modelNeg] != centroNegT {
		t.Fatalf("centro = %+v, %v", centro, err)
	}
	norte, err := FitPinned(cobrancaModel(), franquias(), account, map[string]string{modelPag: nortePagT})
	if err != nil || norte.SpreadsheetID != sharedFile || norte.Tabs[modelPag] != nortePagT || norte.Tabs[modelNeg] != norteNegT {
		t.Fatalf("norte = %+v, %v", norte, err)
	}
}

func TestAChosenTabThatDoesNotFitIsRefused(t *testing.T) {
	file := franquias()
	column(file.Sheets[3], "valor").Type = googlesheet.TypeText
	cases := []struct {
		name    string
		file    googlesheet.Spreadsheet
		pins    map[string]string
		code    string
		problem *Problem
	}{
		{name: "a tab that is no longer in the file", file: franquias(), pins: map[string]string{modelPag: "gone"}, code: CodeTabNotFound},
		{name: "a tab whose columns do not fit", file: file, pins: map[string]string{modelPag: nortePagT}, code: CodeTabMissing,
			problem: &Problem{Code: CodeColumnType, SheetID: nortePagT, Tab: "Pagamentos Norte", Column: "Valor"}},
		{name: "a model tab that is not in the model", file: franquias(), pins: map[string]string{"model-gone": nortePagT}, code: CodeModelMissing},
		{name: "a timeline the payment tab does not link to", file: franquias(), pins: map[string]string{modelPag: centroPagT, modelNeg: norteNegT}, code: CodeTabMissing},
		{name: "a timeline on a payment tab", file: franquias(), pins: map[string]string{modelPag: centroPagT, modelNeg: centroPagT}, code: CodeTabMissing,
			problem: &Problem{Code: CodeRequiredUnmapped, SheetID: centroPagT, Tab: "Pagamentos Centro", Column: "Data do pagamento"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := FitPinned(cobrancaModel(), c.file, account, c.pins)
			got := misfitOf(t, err)
			if got.Code != c.code {
				t.Fatalf("misfit = %+v, want %s", got, c.code)
			}
			if c.problem != nil && (len(got.Problems) != 1 || got.Problems[0] != *c.problem) {
				t.Fatalf("problems = %+v", got.Problems)
			}
		})
	}
}

func TestAChoiceForATabTheFlowDoesNotUseIsIgnored(t *testing.T) {
	model := cobrancaModel()
	model.Uses = []Use{{ModelSheetID: modelNeg, Kind: UseExport}}
	file := centro()
	got, err := FitPinned(model, file, account, map[string]string{modelPag: "gone"})
	if err != nil || got.Tabs[modelNeg] != centroNeg || len(got.Tabs) != 1 {
		t.Fatalf("FitPinned = %+v, %v", got, err)
	}
}

func TestCandidatesOfferEveryTabWithTheBackendVerdict(t *testing.T) {
	model := cobrancaModel()
	broken := franquias()
	broken.ID = "sp-broken"
	broken.Sheets = broken.Sheets[:2]
	column(broken.Sheets[1], "valor").Type = googlesheet.TypeText
	ws := Workspace{Account: account, Files: []googlesheet.Spreadsheet{{ID: modelFile, Sheets: model.Sheets}, franquias(), broken}}
	got, err := Candidates(ws, model, Choice{})
	if err != nil || len(got) != 3 {
		t.Fatalf("Candidates = %+v, %v", got, err)
	}
	byID := map[string]Candidate{}
	for _, candidate := range got {
		byID[candidate.File.ID] = candidate
	}
	shared := byID[sharedFile]
	if shared.Misfit == nil || shared.Misfit.Code != CodeTabAmbiguous || !shared.Choosable() {
		t.Fatalf("an ambiguous file can be chosen tab by tab = %+v", shared)
	}
	if len(shared.Tabs) != 2 || shared.Tabs[1].ModelSheetID != modelPag || shared.Tabs[1].ModelTab != "Pagamentos" || len(shared.Tabs[1].Options) != 4 {
		t.Fatalf("tabs = %+v", shared.Tabs)
	}
	options := shared.Tabs[1].Options
	if options[1].SheetID != centroPagT || options[1].Problem != nil || options[3].Problem != nil || options[0].Problem == nil || options[0].Problem.SheetID != centroNegT {
		t.Fatalf("options = %+v", options)
	}
	if byID[sharedFile].Assignment.Tabs != nil {
		t.Fatalf("an ambiguous file has no assignment = %+v", byID[sharedFile].Assignment)
	}
	if b := byID["sp-broken"]; b.Choosable() || b.Misfit == nil {
		t.Fatalf("a file without a fitting tab cannot be chosen = %+v", b)
	}
	if shared.Chosen || shared.Choice.SpreadsheetID != sharedFile || len(shared.Choice.Tabs) != 0 {
		t.Fatalf("a file not being chosen starts from the automatic match = %+v", shared.Choice)
	}
	if m := byID[modelFile]; !m.Choosable() || m.Misfit != nil || m.Assignment.Tabs[modelNeg] != modelNeg || m.Assignment.Tabs[modelPag] != modelPag {
		t.Fatalf("the model file is offered and writes into its own tabs = %+v", m)
	}

	chosen, err := Candidates(ws, model, Choice{SpreadsheetID: sharedFile, Tabs: map[string]string{modelPag: nortePagT}})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range chosen {
		if candidate.File.ID != sharedFile {
			continue
		}
		if candidate.Misfit != nil || candidate.Assignment.Tabs[modelPag] != nortePagT || candidate.Assignment.Tabs[modelNeg] != norteNegT {
			t.Fatalf("the chosen file follows the choice = %+v", candidate)
		}
		if !candidate.Chosen || candidate.Choice.Tabs[modelPag] != nortePagT {
			t.Fatalf("the chosen file keeps the choice = %+v", candidate)
		}
	}
	if chosen[0].File.ID != sharedFile {
		t.Fatalf("a chosen file that fits comes first = %+v", chosen[0].File.ID)
	}
}

func TestChoosingATabKeepsTheOtherChoices(t *testing.T) {
	start := Choice{SpreadsheetID: sharedFile, Tabs: map[string]string{modelNeg: norteNegT}}
	got := start.With(modelPag, nortePagT)
	if got.SpreadsheetID != sharedFile || got.Tabs[modelNeg] != norteNegT || got.Tabs[modelPag] != nortePagT {
		t.Fatalf("With = %+v", got)
	}
	if len(start.Tabs) != 1 {
		t.Fatalf("the original choice changed = %+v", start)
	}
	if plain := (Choice{SpreadsheetID: sharedFile}).With(modelPag, centroPagT); plain.Tabs[modelPag] != centroPagT || len(plain.Tabs) != 1 {
		t.Fatalf("With on a plain value = %+v", plain)
	}
}
