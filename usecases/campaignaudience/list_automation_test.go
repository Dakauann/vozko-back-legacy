package campaignaudience_usecase

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/campaign"
	"vozko/domain/campaignaudience"
	wc "vozko/domain/whatsapp_campaign"
	"vozko/usecases/campaignautomation"
)

type agentBook map[string]*agent.Agent

func (b agentBook) Execute(id string) (*agent.Agent, error) {
	if a, ok := b[id]; ok {
		return a, nil
	}
	return nil, agent.ErrAgentNotFound
}

const ownerList = "NOME;TELEFONE;VAR1;VAR2;VAR3;NOME_CLIENTE;PENDENCIA\n" +
	"Maria;5511900010142;Maria;R$ 10,00;00123;Maria Souza;R$ 10,00\n" +
	"João;5511900020202;João;R$ 20,00;00456;João Lima;\n"

var collector = &agent.Agent{ID: "agent-1", WorkspaceID: "ws-1", Name: "Cobrança IA", MessagingPrompt: "Cobre {{matricula}}", Variables: []agent.AgentVariable{
	{Name: "matricula"}, {Name: "valor_pendencia"}, {Name: "tom", DefaultValue: "cordial"},
}}

type ownerWorld struct {
	*world
	rules  *campaignautomation.Step
	params Params
}

func newOwnerWorld(t *testing.T) ownerWorld {
	t.Helper()
	w := newWorld(t)
	rules, err := campaignautomation.NewStep(fakeWorkflowBook{}, agentBook{"agent-1": collector})
	if err != nil {
		t.Fatal(err)
	}
	ww := newWhatsAppWorld(t)
	ww.deps.Templates = fakeTemplates{"tpl-3": approvedTemplate("Olá {{1}}, valor {{2}}, matrícula {{3}}", "Maria", "R$ 1", "1")}
	ww.deps.Automation = rules
	official, err := NewOfficialChannel(ww.deps)
	if err != nil {
		t.Fatal(err)
	}
	w.service.AddChannel(official)
	return ownerWorld{world: w, rules: rules, params: Params{TemplateID: "tpl-3", Automation: campaign.Automation{AgentID: "agent-1"}}}
}

func (o ownerWorld) count(t *testing.T, spec campaignaudience.Spec) campaignaudience.Count {
	t.Helper()
	count, err := o.service.Count(ctx, testActor, campaignaudience.ChannelWhatsApp, o.params, spec)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func variableOf(t *testing.T, count campaignaudience.Count, key string) campaignaudience.VariableCount {
	t.Helper()
	for _, v := range count.Variables {
		if v.Key == key {
			return v
		}
	}
	t.Fatalf("no variable %q in %+v", key, count.Variables)
	return campaignaudience.VariableCount{}
}

func listSpec(id string, vars ...campaignaudience.Variable) campaignaudience.Spec {
	return campaignaudience.Spec{Source: campaignaudience.SourceList, ListID: id, Variables: vars}
}

func TestAListColumnFeedsATemplatePositionAndTheAgentVariableOfTheSameRow(t *testing.T) {
	o := newOwnerWorld(t)
	preview := o.upload(t, campaignaudience.ChannelWhatsApp, ownerList, View{Params: o.params})
	if !reflect.DeepEqual(preview.Columns.Headers, []string{"NOME", "TELEFONE", "VAR1", "VAR2", "VAR3", "NOME_CLIENTE", "PENDENCIA"}) {
		t.Fatalf("headers = %v", preview.Columns.Headers)
	}
	untouched := o.count(t, listSpec(preview.ID))
	matricula, tom := variableOf(t, untouched, "matricula"), variableOf(t, untouched, "tom")
	if matricula.Kind != campaignaudience.SlotAutomation || matricula.NeededBy != "Cobrança IA" || matricula.Column != "" || matricula.Missing != 2 || !matricula.ChoiceRequired {
		t.Fatalf("matricula = %+v, the list has no column with its name", matricula)
	}
	if !tom.Optional || tom.Default != "cordial" || tom.Missing != 0 || tom.ChoiceRequired {
		t.Fatalf("tom = %+v, an agent variable with a default is optional", tom)
	}
	if !reflect.DeepEqual(untouched.Blockers, []campaignaudience.Blocker{campaignaudience.BlockerChoiceRequired}) {
		t.Fatalf("blockers = %v", untouched.Blockers)
	}
	chosen := listSpec(preview.ID,
		campaignaudience.Variable{Key: "matricula", Column: "VAR3"},
		campaignaudience.Variable{Key: "valor_pendencia", Column: "PENDENCIA", Missing: campaignaudience.MissingExclude},
	)
	count := o.count(t, chosen)
	if len(count.Blockers) != 0 || count.Sendable == nil || *count.Sendable != 1 || count.Out[campaignaudience.ReasonMissingVariable] != 1 {
		t.Fatalf("count = %+v", count)
	}
	if got := variableOf(t, count, "matricula"); got.Column != "VAR3" || got.Suggested || got.Missing != 0 {
		t.Fatalf("matricula = %+v", got)
	}
	recipients, err := o.service.Recipients(ctx, testActor, campaignaudience.ChannelWhatsApp, o.params, chosen)
	if err != nil {
		t.Fatal(err)
	}
	maria, joao := recipients[0], recipients[1]
	if !reflect.DeepEqual(maria.Variables, []string{"Maria", "R$ 10,00", "00123"}) {
		t.Fatalf("maria variables = %v", maria.Variables)
	}
	wantMeta := map[string]any{"matricula": "00123", "valor_pendencia": "R$ 10,00", "nome_cliente": "Maria Souza", "pendencia": "R$ 10,00"}
	if !reflect.DeepEqual(maria.Metadata, wantMeta) {
		t.Fatalf("maria metadata = %#v", maria.Metadata)
	}
	if joao.Skip != campaign.SkipMissingVariable || joao.Number != "5511900020202" || len(joao.Variables) != 0 {
		t.Fatalf("joao = %+v, the row without a pending value is left out", joao)
	}
	rule, err := o.rules.Rule("ws-1", o.params.Automation)
	if err != nil {
		t.Fatal(err)
	}
	if err := rule.Check([]campaign.EntryMetadata{campaign.EntryMetadataOf("", maria.Number, maria.Metadata).WithVariables(maria.Variables)}); err != nil {
		t.Fatalf("the create rule refuses what the audience let through: %v", err)
	}
}

func TestAnOptionalAgentVariableCanReadAListColumnOrKeepItsDefault(t *testing.T) {
	o := newOwnerWorld(t)
	preview := o.upload(t, campaignaudience.ChannelWhatsApp, "TELEFONE;VAR1;VAR2;VAR3;TOM\n5511900010142;Maria;R$ 10;00123;seco\n", View{Params: o.params})
	base := []campaignaudience.Variable{{Key: "matricula", Column: "VAR3"}, {Key: "valor_pendencia", Column: "VAR2"}}
	suggested := o.count(t, listSpec(preview.ID, base...))
	if got := variableOf(t, suggested, "tom"); got.Column != "TOM" || !got.Suggested || !got.Optional {
		t.Fatalf("tom = %+v, the column with its name is preselected", got)
	}
	fed, err := o.service.Recipients(ctx, testActor, campaignaudience.ChannelWhatsApp, o.params, listSpec(preview.ID, base...))
	if err != nil {
		t.Fatal(err)
	}
	if fed[0].Metadata["tom"] != "seco" {
		t.Fatalf("metadata = %v", fed[0].Metadata)
	}
	kept, err := o.service.Recipients(ctx, testActor, campaignaudience.ChannelWhatsApp, o.params, listSpec(preview.ID, append(base, campaignaudience.Variable{Key: "tom", Source: campaignaudience.SourceDefault})...))
	if err != nil {
		t.Fatal(err)
	}
	if _, sent := kept[0].Metadata["tom"]; sent {
		t.Fatalf("metadata = %v, the default leaves the key out so the agent uses its own", kept[0].Metadata)
	}
}

func TestAColumnMissingFromTheUploadedListBlocksTheCampaign(t *testing.T) {
	o := newOwnerWorld(t)
	preview := o.upload(t, campaignaudience.ChannelWhatsApp, ownerList, View{Params: o.params})
	spec := listSpec(preview.ID, campaignaudience.Variable{Key: "matricula", Column: "MATRICULA"}, campaignaudience.Variable{Key: "valor_pendencia", Column: "VAR2"})
	count := o.count(t, spec)
	if count.Blockers[0] != campaignaudience.BlockerListColumnMissing || !variableOf(t, count, "matricula").ColumnMissing {
		t.Fatalf("count = %+v", count)
	}
	if _, err := o.service.Recipients(ctx, testActor, campaignaudience.ChannelWhatsApp, o.params, spec); !errors.Is(err, campaignaudience.ErrListColumnUnknown) {
		t.Fatalf("recipients = %v", err)
	}
	inner := &recordingOfficial{}
	input := &wc.Campaign{WorkspaceID: "ws-1", TemplateID: "tpl-3", AgentID: "agent-1", EnableAgentResponses: true, Recipients: &campaignaudience.Request{Spec: spec, UserID: "user-1"}}
	if _, err := OfficialCreate(inner, o.service).Execute(ctx, input); !errors.Is(err, campaignaudience.ErrListColumnUnknown) || len(inner.got) != 0 {
		t.Fatalf("create = %v, inner %d", err, len(inner.got))
	}
}

func TestTheRowsShowTheBoundAutomationValuesAndWhatIsStillMissing(t *testing.T) {
	o := newOwnerWorld(t)
	view := View{Params: o.params, Variables: []campaignaudience.Variable{{Key: "matricula", Column: "VAR3"}, {Key: "valor_pendencia", Column: "PENDENCIA"}}}
	preview := o.upload(t, campaignaudience.ChannelWhatsApp, ownerList, view)
	wantColumns := []campaign.MetadataColumn{{Key: "nome_cliente", Label: "NOME_CLIENTE"}, {Key: "pendencia", Label: "PENDENCIA"}, {Key: "matricula", Label: "matricula"}, {Key: "valor_pendencia", Label: "valor_pendencia"}, {Key: "tom", Label: "tom"}}
	if !reflect.DeepEqual(preview.Columns.Metadata, wantColumns) {
		t.Fatalf("metadata columns = %+v", preview.Columns.Metadata)
	}
	page, err := o.service.Rows(ctx, testActor, campaignaudience.ChannelWhatsApp, preview.ID, RowsQuery{View: view, MissingOnly: true, Page: 1, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Fatalf("page = %+v", page)
	}
	joao := page.Items[0]
	if joao.Name != "João" || !reflect.DeepEqual(joao.MetadataMissing, []string{"valor_pendencia"}) || len(joao.Missing) != 0 || len(joao.Variables) != 3 {
		t.Fatalf("joao = %+v", joao)
	}
	if joao.Metadata["matricula"] != "00456" || joao.Metadata["nome_cliente"] != "João Lima" {
		t.Fatalf("joao metadata = %v", joao.Metadata)
	}
}

func TestALeadsAudienceRefusesAListColumn(t *testing.T) {
	o := newOwnerWorld(t)
	spec := campaignaudience.Spec{Source: campaignaudience.SourceLeads, Selection: confirmed(everyoneSelection(), 0), Variables: []campaignaudience.Variable{{Key: "matricula", Column: "VAR3"}}}
	if _, err := o.service.Count(ctx, testActor, campaignaudience.ChannelWhatsApp, o.params, spec); !errors.Is(err, campaignaudience.ErrColumnNeedsList) {
		t.Fatalf("count = %v", err)
	}
}

func TestTheSampleNamesAColumnForEachRequiredAutomationVariable(t *testing.T) {
	o := newOwnerWorld(t)
	file, err := o.service.Sample(ctx, testActor, campaignaudience.ChannelWhatsApp, o.params, []campaignaudience.Variable{{Key: "matricula", Column: "VAR3"}}, "csv")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(file.Data); !containsLine(got, "number;name;var1;var2;var3;matricula;valor_pendencia") {
		t.Fatalf("sample = %q", got)
	}
}

func containsLine(text, line string) bool {
	return strings.Contains(text, line)
}
