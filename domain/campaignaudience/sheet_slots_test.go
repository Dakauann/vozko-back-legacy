package campaignaudience

import (
	"errors"
	"reflect"
	"testing"

	"vozko/domain/automationneeds"
	"vozko/domain/campaign"
	"vozko/domain/sheetfit"
	"vozko/domain/variabledecl"
)

func sheetNeed(fallback string) automationneeds.Need {
	need := needOf("planilha", "Franquias")
	need.Label = "Planilha de gravação"
	need.Type = variabledecl.Type{Kind: variabledecl.KindSheet, Settings: map[string]string{variabledecl.SettingModel: "sp-model"}}
	if fallback != "" {
		need.Type.Settings[variabledecl.SettingDefault] = fallback
	}
	need.Shape = variabledecl.ShapeSheet
	need.SheetUses = []sheetfit.Use{{ModelSheetID: "m-pag", Kind: sheetfit.UseAddRow}}
	return need
}

func sheetSlot() Slot {
	return Slot{Key: "planilha", Name: "Planilha de gravação", Kind: SlotAutomation, Sheet: true}
}

func variableKey(t *testing.T, err error) string {
	t.Helper()
	key, ok := VariableOf(err)
	if !ok {
		t.Fatalf("err = %v names no variable", err)
	}
	return key
}

func TestAPlanilhaSlotTakesNoColumnNorLeadFieldAndStartsFromTheDeclaredDefault(t *testing.T) {
	suggest := NeedSuggest{
		Field:  func(string) campaign.BindingSource { return campaign.CustomBinding("planilha") },
		Column: func(string) string { return "planilha_de_gravacao" },
	}
	got := AutomationSlots(automationneeds.Needs{sheetNeed("sp-centro")}, nil, nil, suggest)
	if len(got) != 1 {
		t.Fatalf("slots = %+v", got)
	}
	slot := got[0]
	if !slot.Sheet || slot.Suggested != "" || slot.Column != "" || slot.Literal != "sp-centro" || slot.Shape != variabledecl.ShapeSheet || slot.Type.Kind != variabledecl.KindSheet {
		t.Fatalf("slot = %+v", slot)
	}
	text := AutomationSlots(automationneeds.Needs{needOf("nome", "Franquias")}, nil, nil, suggest)[0]
	if text.Sheet || text.Literal != "" || text.Column != "planilha_de_gravacao" {
		t.Fatalf("a text slot = %+v", text)
	}
}

func TestBindStartsAPlanilhaFromItsDefaultUntilTheOperatorPicksOne(t *testing.T) {
	slot := sheetSlot()
	slot.Literal = "sp-centro"
	bound, err := Bind([]Slot{slot}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b := bound[0]; b.Source != campaign.BindLiteral || b.Value != "sp-centro" || !b.Suggested || !b.Sheet {
		t.Fatalf("bound = %+v", b)
	}
	bound, err = Bind([]Slot{slot}, []Variable{{Key: "planilha", Source: campaign.BindLiteral, Value: "sp-norte"}})
	if err != nil {
		t.Fatal(err)
	}
	if b := bound[0]; b.Value != "sp-norte" || b.Suggested {
		t.Fatalf("chosen = %+v", b)
	}
}

func TestBindRefusesAPerRecipientSourceForAPlanilhaNamingTheVariable(t *testing.T) {
	refused := []Variable{
		{Key: "planilha", Column: "planilha_de_gravacao"},
		{Key: "planilha", Source: campaign.BindFirstName},
		{Key: "planilha", Source: campaign.CustomBinding("planilha")},
		{Key: "planilha", Source: campaign.BindLiteral, Value: "sp-norte", Missing: MissingText, Text: "x"},
		{Key: "planilha", Source: campaign.BindLiteral, Value: "sp-norte", Missing: MissingExclude},
	}
	for _, v := range refused {
		_, err := Bind([]Slot{sheetSlot()}, []Variable{v})
		if !errors.Is(err, ErrSheetLiteralOnly) || variableKey(t, err) != "planilha" {
			t.Fatalf("%+v = %v", v, err)
		}
	}
}

func TestAnUnchosenPlanilhaBlocksTheCampaignWithoutCountingRows(t *testing.T) {
	bound, err := Bind([]Slot{{Key: "1"}, sheetSlot()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	tally := NewTally(ChannelWhatsApp, SourceList, bound).Checking(func(string, string) bool { calls++; return true })
	tally.Add(Candidate{Line: 1, Contact: "5511900000001", HasContact: true, Values: []string{"Ana", ""}})
	count := tally.Count()
	if !reflect.DeepEqual(count.Blockers, []Blocker{BlockerSheetRequired}) {
		t.Fatalf("blockers = %v", count.Blockers)
	}
	planilha := count.Variables[1]
	if planilha.Missing != 0 || planilha.ChoiceRequired || !planilha.Sheet {
		t.Fatalf("planilha = %+v", planilha)
	}
	err = tally.Creatable()
	if !errors.Is(err, ErrSheetRequired) || variableKey(t, err) != "planilha" {
		t.Fatalf("creatable = %v", err)
	}
	if calls != 0 {
		t.Fatalf("the row check ran %d times for a campaign-wide Planilha", calls)
	}
}

func TestAPlanilhaThatDoesNotFitBlocksTheCampaignWithTheReason(t *testing.T) {
	bound, err := Bind([]Slot{sheetSlot()}, []Variable{{Key: "planilha", Source: campaign.BindLiteral, Value: "sp-norte"}})
	if err != nil {
		t.Fatal(err)
	}
	bound[0].SheetUnfit = string(sheetfit.CodeFileNotFound)
	tally := NewTally(ChannelWhatsApp, SourceLeads, bound)
	tally.Add(Candidate{LeadID: "l1", HasContact: true, Values: []string{"sp-norte"}})
	count := tally.Count()
	if !reflect.DeepEqual(count.Blockers, []Blocker{BlockerSheetUnfit}) || count.Variables[0].SheetUnfit != string(sheetfit.CodeFileNotFound) {
		t.Fatalf("count = %+v", count)
	}
	if err := tally.Creatable(); !errors.Is(err, ErrSheetUnfit) || variableKey(t, err) != "planilha" {
		t.Fatalf("creatable = %v", err)
	}
}

func TestAChosenPlanilhaGoesToEveryEntryAndTheCountCarriesItsType(t *testing.T) {
	slot := sheetSlot()
	slot.Type, slot.Shape = variabledecl.Type{Kind: variabledecl.KindSheet}, variabledecl.ShapeSheet
	choice := Slot{Key: "turno", Kind: SlotAutomation, Type: variabledecl.Type{Kind: variabledecl.KindChoice, Options: []string{"Manhã"}}, Shape: variabledecl.ShapeChoice}
	bound, err := Bind([]Slot{slot, choice}, []Variable{{Key: "planilha", Source: campaign.BindLiteral, Value: "sp-norte"}, {Key: "turno", Source: campaign.BindLiteral, Value: "Manhã"}})
	if err != nil {
		t.Fatal(err)
	}
	tally := NewTally(ChannelWhatsApp, SourceLeads, bound)
	d := tally.Add(Candidate{LeadID: "l1", HasContact: true, Values: []string{"sp-norte", "Manhã"}})
	if d.Outcome != OutcomeSend {
		t.Fatalf("decision = %+v", d)
	}
	count := tally.Count()
	want := &VariableType{Kind: variabledecl.KindSheet, Shape: variabledecl.ShapeSheet, Options: []string{}}
	if len(count.Blockers) != 0 || !reflect.DeepEqual(count.Variables[0].Type, want) {
		t.Fatalf("count = %+v type = %+v", count, count.Variables[0].Type)
	}
	if got := count.Variables[1].Type; got == nil || got.Shape != variabledecl.ShapeChoice || !reflect.DeepEqual(got.Options, []string{"Manhã"}) {
		t.Fatalf("turno type = %+v", got)
	}
}

func TestAPlanilhaIsNeverReadFromAListColumn(t *testing.T) {
	bound := []Bound{{Variable: Variable{Key: "1"}}, {Variable: Variable{Key: "nome", Column: "NOME"}, Kind: SlotAutomation}, {Variable: Variable{Key: "planilha", Source: campaign.BindLiteral, Value: "sp"}, Kind: SlotAutomation, Sheet: true}}
	want := []campaign.ListTarget{{}, {Key: "nome", Column: "NOME"}, {Key: "planilha", Fixed: true}}
	if got := ListTargets(bound); !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %+v", got)
	}
}

func TestVariableErrorsNameTheVariable(t *testing.T) {
	err := ValidateVariables([]Variable{{Key: "nome", Missing: MissingText}})
	if !errors.Is(err, ErrTextRequired) || variableKey(t, err) != "nome" {
		t.Fatalf("text = %v", err)
	}
	if _, err := Bind([]Slot{{Key: "1"}}, []Variable{{Key: "2"}}); !errors.Is(err, ErrVariableUnknown) || variableKey(t, err) != "2" {
		t.Fatalf("unknown = %v", err)
	}
	if _, err := Bind([]Slot{{Key: "nome", Kind: SlotAutomation}}, []Variable{{Key: "nome", Source: SourceDefault}}); !errors.Is(err, ErrDefaultUnavailable) || variableKey(t, err) != "nome" {
		t.Fatalf("default = %v", err)
	}
	if _, ok := VariableOf(ErrTemplateRequired); ok {
		t.Fatal("a campaign-wide error names no variable")
	}
}
