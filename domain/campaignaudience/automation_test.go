package campaignaudience

import (
	"reflect"
	"testing"

	"vozko/domain/automationneeds"
	"vozko/domain/campaign"
	"vozko/domain/conversation"
)

func needOf(name, owner string) automationneeds.Need {
	return automationneeds.Need{Name: name, Via: []automationneeds.Link{{Automation: conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "wf"}, Name: owner}}}
}

func TestAutomationSlotsListWhatTheAutomationNeedsBesideTheTemplate(t *testing.T) {
	needs := automationneeds.Needs{needOf("1", "Cobrança"), needOf("nome_cliente", "Cobrança"), needOf("cpf", "Cobrança")}
	suggest := func(name string) campaign.BindingSource {
		if name == "cpf" {
			return campaign.CustomBinding("cpf")
		}
		return ""
	}
	got := AutomationSlots(needs, []Slot{{Key: "1"}}, suggest)
	want := []Slot{
		{Key: "nome_cliente", Name: "nome_cliente", Kind: SlotAutomation, NeededBy: "Cobrança"},
		{Key: "cpf", Name: "cpf", Kind: SlotAutomation, NeededBy: "Cobrança", Suggested: campaign.CustomBinding("cpf")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("slots = %+v\nwant %+v", got, want)
	}
}

func TestBindCarriesTheKindAndWhoNeedsTheVariable(t *testing.T) {
	bound, err := Bind([]Slot{{Key: "1"}, {Key: "turma", Kind: SlotAutomation, NeededBy: "Escola"}}, []Variable{{Key: "turma", Source: campaign.BindLiteral, Value: "A"}})
	if err != nil {
		t.Fatal(err)
	}
	if bound[1].Kind != SlotAutomation || bound[1].NeededBy != "Escola" || bound[1].Value != "A" {
		t.Fatalf("bound = %+v", bound[1])
	}
}

func TestTheTallySplitsTemplateValuesFromAutomationMetadata(t *testing.T) {
	tally := NewTally(ChannelWhatsApp, SourceLeads, []Bound{{Variable: Variable{Key: "1"}}, {Variable: Variable{Key: "turma"}, Kind: SlotAutomation}, {Variable: Variable{Key: "2"}}})
	template, metadata := tally.Split([]string{"Ana", "B", "1º"})
	if !reflect.DeepEqual(template, []string{"Ana", "1º"}) || !reflect.DeepEqual(metadata, map[string]any{"turma": "B"}) {
		t.Fatalf("template = %v metadata = %v", template, metadata)
	}
	count := tally.Count()
	if count.Variables[0].Kind != SlotTemplate || count.Variables[1].Kind != SlotAutomation || count.RequiredVariables != 2 {
		t.Fatalf("count = %+v", count)
	}
}

func TestKnownVariablesDropsAutomationChoicesTheListDoesNotUse(t *testing.T) {
	got := KnownVariables([]Variable{{Key: "1"}, {Key: "turma"}, {Key: "outra"}}, []Slot{{Key: "1"}}, automationneeds.Needs{needOf("turma", "Escola")})
	if !reflect.DeepEqual(got, []Variable{{Key: "1"}, {Key: "outra"}}) {
		t.Fatalf("variables = %+v", got)
	}
}
