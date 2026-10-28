package campaignaudience

import (
	"errors"
	"reflect"
	"testing"

	"vozko/domain/automationneeds"
	"vozko/domain/campaign"
)

func optionalOf(name, fallback, owner string) automationneeds.Need {
	need := needOf(name, owner)
	need.Default = fallback
	return need
}

func TestAutomationSlotsPreselectTheColumnOfTheSameNameAndMarkOptionalVariables(t *testing.T) {
	needs := automationneeds.Needs{needOf("matricula", "Cobrança"), needOf("nome_cliente", "Cobrança")}
	optional := automationneeds.Needs{optionalOf("tom", "cordial", "Agente"), optionalOf("unidade", "Centro", "Agente")}
	suggest := NeedSuggest{
		Field: func(name string) campaign.BindingSource { return campaign.CustomBinding(name) },
		Column: func(name string) string {
			if name == "nome_cliente" || name == "unidade" {
				return "NOME_CLIENTE"
			}
			return ""
		},
	}
	got := AutomationSlots(needs, optional, nil, suggest)
	want := []Slot{
		{Key: "matricula", Name: "matricula", Kind: SlotAutomation, NeededBy: "Cobrança", Suggested: campaign.CustomBinding("matricula")},
		{Key: "nome_cliente", Name: "nome_cliente", Kind: SlotAutomation, NeededBy: "Cobrança", Suggested: campaign.CustomBinding("nome_cliente"), Column: "NOME_CLIENTE"},
		{Key: "tom", Name: "tom", Kind: SlotAutomation, NeededBy: "Agente", Optional: true, Default: "cordial"},
		{Key: "unidade", Name: "unidade", Kind: SlotAutomation, NeededBy: "Agente", Optional: true, Default: "Centro", Column: "NOME_CLIENTE"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("slots = %+v\nwant %+v", got, want)
	}
}

func TestBindPrefersTheSuggestedColumnUntilTheOperatorChoosesASource(t *testing.T) {
	slots := []Slot{
		{Key: "nome_cliente", Kind: SlotAutomation, Column: "NOME_CLIENTE", Suggested: campaign.CustomBinding("nome_cliente")},
		{Key: "matricula", Kind: SlotAutomation, Column: "MATRICULA"},
		{Key: "tom", Kind: SlotAutomation, Optional: true, Default: "cordial", Column: "TOM"},
	}
	bound, err := Bind(slots, []Variable{{Key: "matricula", Column: "VAR3"}, {Key: "tom", Source: SourceDefault}})
	if err != nil {
		t.Fatal(err)
	}
	if bound[0].Column != "NOME_CLIENTE" || bound[0].Source != "" || !bound[0].Suggested {
		t.Fatalf("nome_cliente = %+v, want the column suggested", bound[0])
	}
	if bound[1].Column != "VAR3" || bound[1].Suggested {
		t.Fatalf("matricula = %+v, want the chosen column", bound[1])
	}
	if bound[2].Column != "" || bound[2].Source != SourceDefault || !Unbound(bound[2]) || !bound[2].Optional || bound[2].Default != "cordial" {
		t.Fatalf("tom = %+v, want the default kept", bound[2])
	}
}

func TestBindRefusesAColumnWithAnotherSourceAndTheDefaultOfARequiredVariable(t *testing.T) {
	slots := []Slot{{Key: "matricula", Kind: SlotAutomation}}
	if _, err := Bind(slots, []Variable{{Key: "matricula", Column: "VAR3", Source: campaign.BindFirstName}}); !errors.Is(err, ErrColumnWithSource) {
		t.Fatalf("column and source = %v", err)
	}
	if _, err := Bind(slots, []Variable{{Key: "matricula", Source: SourceDefault}}); !errors.Is(err, ErrDefaultUnavailable) {
		t.Fatalf("default of a required variable = %v", err)
	}
	if _, err := Bind([]Slot{{Key: "1"}}, []Variable{{Key: "1", Source: SourceDefault}}); !errors.Is(err, ErrDefaultUnavailable) {
		t.Fatalf("default of a template field = %v", err)
	}
}

func TestALeadsAudienceCannotReadAListColumn(t *testing.T) {
	spec := Spec{Source: SourceLeads, Selection: filterSelection(), Variables: []Variable{{Key: "matricula", Column: "VAR3"}}}
	if err := spec.Validate(); !errors.Is(err, ErrColumnNeedsList) {
		t.Fatalf("validate = %v", err)
	}
	spec.Source, spec.Selection, spec.ListID = SourceList, nil, "list-1"
	if err := spec.Validate(); err != nil {
		t.Fatalf("a list may: %v", err)
	}
}

func listBound() []Bound {
	return []Bound{
		{Variable: Variable{Key: "1"}},
		{Variable: Variable{Key: "matricula", Column: "VAR3"}, Kind: SlotAutomation},
		{Variable: Variable{Key: "valor_pendencia", Column: "VALOR", Missing: MissingExclude}, Kind: SlotAutomation},
		{Variable: Variable{Key: "tom"}, Kind: SlotAutomation, Optional: true, Default: "cordial"},
		{Variable: Variable{Key: "unidade", Column: "UNIDADE"}, Kind: SlotAutomation, Optional: true, Default: "Centro"},
	}
}

func TestAListRowWithoutAnAutomationValueWaitsForTheMissingChoice(t *testing.T) {
	tally := NewTally(ChannelWhatsApp, SourceList, listBound())
	ready := tally.Add(Candidate{Line: 2, Contact: "a", HasContact: true, Values: []string{"Ana", "00123", "R$ 10", "", "Sul"}})
	waiting := tally.Add(Candidate{Line: 3, Contact: "b", HasContact: true, Values: []string{"Bia", "", "R$ 20", "", ""}})
	excluded := tally.Add(Candidate{Line: 4, Contact: "c", HasContact: true, Values: []string{"Caio", "00999", "", "", ""}})
	if ready.Outcome != OutcomeSend || waiting.Outcome != OutcomeUndecided || excluded.Outcome != OutcomeOut || excluded.Reason != ReasonMissingVariable {
		t.Fatalf("outcomes = %v %v %+v", ready.Outcome, waiting.Outcome, excluded)
	}
	if !reflect.DeepEqual(excluded.Missing, []campaign.MissingVariable{{Slot: 3}}) {
		t.Fatalf("excluded missing = %+v", excluded.Missing)
	}
	count := tally.Count()
	if !reflect.DeepEqual(count.Blockers, []Blocker{BlockerChoiceRequired}) || count.Sendable != nil {
		t.Fatalf("blockers = %v sendable %v", count.Blockers, count.Sendable)
	}
	matricula, tom, unidade := count.Variables[1], count.Variables[3], count.Variables[4]
	if matricula.Missing != 1 || !matricula.ChoiceRequired || matricula.Column != "VAR3" {
		t.Fatalf("matricula = %+v", matricula)
	}
	if tom.Missing != 0 || tom.ChoiceRequired || !tom.Optional || tom.Default != "cordial" {
		t.Fatalf("tom = %+v, an optional variable on its default never lacks", tom)
	}
	if unidade.Missing != 2 || unidade.ChoiceRequired {
		t.Fatalf("unidade = %+v, an empty optional cell falls back to the default", unidade)
	}
	if err := tally.Creatable(); !errors.Is(err, ErrChoiceRequired) {
		t.Fatalf("creatable = %v", err)
	}
}

func TestAListRowTakesTheDefaultTextOfAnAutomationVariable(t *testing.T) {
	bound := listBound()
	bound[1].Missing, bound[1].Text = MissingText, "sem matrícula"
	tally := NewTally(ChannelWhatsApp, SourceList, bound)
	d := tally.Add(Candidate{Line: 3, Contact: "b", HasContact: true, Values: []string{"Bia", "", "R$ 20", "", ""}})
	if d.Outcome != OutcomeSend || d.Values[1] != "sem matrícula" {
		t.Fatalf("decision = %+v", d)
	}
	if err := tally.Creatable(); err != nil {
		t.Fatalf("creatable = %v", err)
	}
}

func TestAnEmptyTemplateCellStillBlocksTheList(t *testing.T) {
	tally := NewTally(ChannelWhatsApp, SourceList, listBound())
	tally.Add(Candidate{Line: 2, Contact: "a", HasContact: true, Values: []string{"", "00123", "R$ 10", "", ""}})
	if got := tally.Count().Blockers; !reflect.DeepEqual(got, []Blocker{BlockerListVariablesMissing}) {
		t.Fatalf("blockers = %v", got)
	}
}

func TestAColumnTheListLacksBlocksBeforeAnyChoice(t *testing.T) {
	bound := MarkColumns(listBound(), campaign.ListLayout{Header: true, Headers: []string{"TELEFONE", "VAR3", "UNIDADE"}})
	if bound[1].ColumnMissing || !bound[2].ColumnMissing || bound[4].ColumnMissing {
		t.Fatalf("columns = %+v", bound)
	}
	tally := NewTally(ChannelWhatsApp, SourceList, bound)
	tally.Add(Candidate{Line: 2, Contact: "a", HasContact: true, Values: []string{"Ana", "00123", "", "", ""}})
	count := tally.Count()
	if count.Blockers[0] != BlockerListColumnMissing || !count.Variables[2].ColumnMissing {
		t.Fatalf("count = %+v", count)
	}
	if err := tally.Creatable(); !errors.Is(err, ErrListColumnUnknown) {
		t.Fatalf("creatable = %v", err)
	}
}

func TestEntryMetadataLetsTheBindingDecideEveryAutomationKey(t *testing.T) {
	tally := NewTally(ChannelWhatsApp, SourceList, listBound())
	base := map[string]any{"tom": "seco", "curso": "Direito", "matricula": float64(123)}
	template, metadata := tally.EntryMetadata(base, []string{"Ana", "00123", "R$ 10", "", "Sul"})
	want := map[string]any{"curso": "Direito", "matricula": "00123", "valor_pendencia": "R$ 10", "unidade": "Sul"}
	if !reflect.DeepEqual(template, []string{"Ana"}) || !reflect.DeepEqual(metadata, want) {
		t.Fatalf("template = %v metadata = %#v", template, metadata)
	}
	if base["tom"] != "seco" {
		t.Fatal("the row metadata is never changed in place")
	}
}

func TestListTargetsKeepTheBoundOrder(t *testing.T) {
	got := ListTargets(listBound())
	want := []campaign.ListTarget{{}, {Key: "matricula", Column: "VAR3"}, {Key: "valor_pendencia", Column: "VALOR"}, {Key: "tom"}, {Key: "unidade", Column: "UNIDADE"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %+v", got)
	}
}

func TestALeadsAudienceNeverWaitsOnAnOptionalVariable(t *testing.T) {
	bound := []Bound{
		{Variable: Variable{Key: "tom"}, Kind: SlotAutomation, Optional: true, Default: "cordial"},
		{Variable: Variable{Key: "unidade", Source: campaign.CustomBinding("unidade")}, Kind: SlotAutomation, Optional: true, Default: "Centro"},
	}
	tally := NewTally(ChannelWhatsApp, SourceLeads, bound)
	d := tally.Add(Candidate{LeadID: "a", HasContact: true, Values: []string{"", ""}})
	count := tally.Count()
	if d.Outcome != OutcomeSend || len(count.Blockers) != 0 || count.Variables[0].Missing != 0 || count.Variables[1].Missing != 1 || count.Variables[1].ChoiceRequired {
		t.Fatalf("decision = %+v count = %+v", d, count)
	}
	if _, metadata := tally.Split(d.Values); metadata != nil {
		t.Fatalf("metadata = %v, the agent keeps its defaults", metadata)
	}
}
