package campaignaudience_usecase

import (
	"errors"
	"reflect"
	"testing"
	"vozko/domain/automationneeds"

	"vozko/domain/campaign"
	"vozko/domain/campaignaudience"
	uw "vozko/domain/unofficial_whatsapp"
	uwc "vozko/domain/unofficial_whatsapp_campaign"
	wc "vozko/domain/whatsapp_campaign"
)

func classNeed() automationneeds.Needs {
	return automationneeds.Needs{{Name: "turma", Via: []automationneeds.Link{{Name: "Escola"}}}}
}

func TestLeadsFeedTheAutomationVariablesLikeTemplateFields(t *testing.T) {
	w := newWorld(t)
	w.selections.ids = []string{"a", "b"}
	w.channel.slots = []campaignaudience.Slot{{Key: "1", Suggested: campaign.BindFirstName}}
	w.channel.needs = classNeed()
	w.channel.judge = fakeJudge{leads: map[string]campaignaudience.Candidate{
		"a": {HasContact: true, Values: []string{"Ana", "B"}},
		"b": {HasContact: true, Values: []string{"Bia", ""}},
	}}
	undecided := campaignaudience.Spec{Source: campaignaudience.SourceLeads, Selection: confirmed(everyoneSelection(), 2)}
	count, err := w.service.Count(ctx, testActor, campaignaudience.ChannelWhatsApp, Params{}, undecided)
	if err != nil {
		t.Fatal(err)
	}
	if len(count.Variables) != 2 || count.Variables[1].Key != "turma" || count.Variables[1].Kind != campaignaudience.SlotAutomation || count.Variables[1].NeededBy != "Escola" {
		t.Fatalf("variables = %+v", count.Variables)
	}
	if !reflect.DeepEqual(count.Blockers, []campaignaudience.Blocker{campaignaudience.BlockerSourceRequired}) {
		t.Fatalf("blockers = %v, the automation variable needs a source like a template field", count.Blockers)
	}
	chosen := undecided
	chosen.Variables = []campaignaudience.Variable{{Key: "turma", Source: campaign.CustomBinding("turma"), Missing: campaignaudience.MissingText, Text: "A"}}
	recipients, err := w.service.Recipients(ctx, testActor, campaignaudience.ChannelWhatsApp, Params{}, chosen)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipients) != 2 || !reflect.DeepEqual(recipients[0].Variables, []string{"Ana"}) || recipients[0].Metadata["turma"] != "B" || recipients[1].Metadata["turma"] != "A" {
		t.Fatalf("recipients = %+v", recipients)
	}
}

func TestOfficialCreateHandsItsAutomationToTheAudience(t *testing.T) {
	w := newWorld(t)
	w.channel.slots = []campaignaudience.Slot{{Key: "1"}}
	w.channel.judge = fakeJudge{}
	w.channel.needs = classNeed()
	preview := w.upload(t, campaignaudience.ChannelWhatsApp, "telefone;nome;var1;Turma\n5511900010142;Maria;M;A\n5511900023301;João;J;\n", View{})
	inner := &recordingOfficial{}
	input := &wc.Campaign{WorkspaceID: "ws-1", TemplateID: "tpl", WorkflowID: "wf-1", EnableWorkflow: true, Recipients: &campaignaudience.Request{Spec: campaignaudience.Spec{Source: campaignaudience.SourceList, ListID: preview.ID}, UserID: "user-1"}}
	if _, err := OfficialCreate(inner, w.service).Execute(ctx, input); !errors.Is(err, campaignaudience.ErrChoiceRequired) {
		t.Fatalf("create = %v", err)
	}
	if got := w.channel.params[len(w.channel.params)-1].Automation; got != input.Automation() || len(inner.got) != 0 {
		t.Fatalf("automation = %+v, inner %d", got, len(inner.got))
	}
}

func TestUnofficialCreateHandsItsAutomationToTheAudience(t *testing.T) {
	w := newWorld(t)
	unofficial := &fakeChannel{name: campaignaudience.ChannelUnofficial, slots: []campaignaudience.Slot{{Key: "1"}}, judge: fakeJudge{}, needs: classNeed()}
	w.service.AddChannel(unofficial)
	preview := w.upload(t, campaignaudience.ChannelUnofficial, "numero;nome;variavel1;Turma\n5511900010142;Maria;Maria;\n", View{})
	inner := &recordingUnofficial{}
	in := &uwc.Campaign{WorkspaceID: "ws-1", InstanceID: "inst-1", AgentID: "agent-1", Message: uwc.MessageSpec{Kind: uwc.KindText, Bodies: []string{"Oi {{1}}"}}, Recipients: &campaignaudience.Request{Spec: campaignaudience.Spec{Source: campaignaudience.SourceList, ListID: preview.ID}, UserID: "user-1"}}
	if _, err := UnofficialCreate(inner, w.service).Execute(ctx, in, uw.DepartmentScope{}); !errors.Is(err, campaignaudience.ErrChoiceRequired) {
		t.Fatalf("create = %v", err)
	}
	if got := unofficial.params[len(unofficial.params)-1].Automation; got != in.Automation() || len(inner.got) != 0 {
		t.Fatalf("automation = %+v, inner %d", got, len(inner.got))
	}
}
