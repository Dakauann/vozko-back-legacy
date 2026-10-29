package campaignaudience_usecase

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"vozko/domain/campaign"
	"vozko/domain/campaignaudience"
	"vozko/domain/sheetfit"
	uwc "vozko/domain/unofficial_whatsapp_campaign"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
	"vozko/usecases/campaignautomation"
)

const sheetKey = "planilha_de_gravacao"

const sheetList = "TELEFONE;NOME;planilha_de_gravacao\n5511900010142;Maria;sp-outra\n5511900020202;João;\n"

type sheetWorld struct {
	*world
	params Params
}

func franchiseFlow() *workflow.Workflow {
	return &workflow.Workflow{ID: "wf-sheet", WorkspaceID: "ws-1", Name: "Franquias", Graph: workflow.Graph{Nodes: []workflow.Node{
		{ID: "row", Type: workflow.NodeTypeActionAddSheetRow, Config: map[string]interface{}{
			workflow.SheetStepSheetKey: "model-pag", workflow.SheetStepVariableKey: "{{campvars." + sheetKey + "}}", workflow.SheetStepValuesKey: map[string]interface{}{"valor": "1"},
		}},
	}}}
}

func newSheetWorld(t *testing.T, fallback string) sheetWorld {
	t.Helper()
	w := newWorld(t)
	settings := map[string]string{variabledecl.SettingModel: "sp-model"}
	if fallback != "" {
		settings[variabledecl.SettingDefault] = fallback
	}
	w.values.Declarations = []variabledecl.Declaration{{Name: sheetKey, Label: "Planilha de gravação", Type: variabledecl.Type{Kind: variabledecl.KindSheet, Settings: settings}}}
	rules, err := campaignautomation.NewStep(fakeWorkflowBook{"wf-sheet": franchiseFlow()}, noAgents{})
	if err != nil {
		t.Fatal(err)
	}
	ww := newWhatsAppWorld(t)
	ww.deps.Templates = fakeTemplates{"tpl-1": approvedTemplate("Olá, tudo bem?")}
	ww.deps.Automation = rules
	official, err := NewOfficialChannel(ww.deps)
	if err != nil {
		t.Fatal(err)
	}
	w.service.AddChannel(official)
	return sheetWorld{world: w, params: Params{TemplateID: "tpl-1", Automation: campaign.Automation{WorkflowID: "wf-sheet", EnableWorkflow: true}}}
}

func (s sheetWorld) count(t *testing.T, spec campaignaudience.Spec) campaignaudience.Count {
	t.Helper()
	count, err := s.service.Count(ctx, testActor, campaignaudience.ChannelWhatsApp, s.params, spec)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestAPlanilhaStartsFromItsDefaultIsCheckedAgainstTheFlowAndIgnoresTheListColumn(t *testing.T) {
	s := newSheetWorld(t, "sp-centro")
	preview := s.upload(t, campaignaudience.ChannelWhatsApp, sheetList, View{Params: s.params})
	for _, column := range preview.Columns.Metadata {
		if column.Key == sheetKey {
			t.Fatalf("metadata columns = %+v, a Planilha is never a list column", preview.Columns.Metadata)
		}
	}
	count := s.count(t, listSpec(preview.ID))
	planilha := variableOf(t, count, sheetKey)
	if !planilha.Sheet || planilha.Source != campaign.BindLiteral || planilha.Value != "sp-centro" || !planilha.Suggested || planilha.Column != "" || planilha.Missing != 0 || planilha.ChoiceRequired {
		t.Fatalf("planilha = %+v", planilha)
	}
	if planilha.Type == nil || planilha.Type.Kind != variabledecl.KindSheet {
		t.Fatalf("type = %+v", planilha.Type)
	}
	if len(count.Blockers) != 0 {
		t.Fatalf("blockers = %v", count.Blockers)
	}
	wantUses := []sheetfit.Use{{ModelSheetID: "model-pag", Kind: sheetfit.UseAddRow, Mapping: map[string]string{"valor": "1"}}}
	if len(s.fits.calls) == 0 || s.fits.calls[0].value != "sp-centro" || s.fits.calls[0].kind != variabledecl.KindSheet || !reflect.DeepEqual(s.fits.calls[0].uses, wantUses) {
		t.Fatalf("fit calls = %+v", s.fits.calls)
	}
	recipients, err := s.service.Recipients(ctx, testActor, campaignaudience.ChannelWhatsApp, s.params, listSpec(preview.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recipients {
		if r.Metadata[sheetKey] != "sp-centro" {
			t.Fatalf("%s metadata = %v, every entry writes to the campaign spreadsheet", r.Number, r.Metadata)
		}
	}
	page, err := s.service.Rows(ctx, testActor, campaignaudience.ChannelWhatsApp, preview.ID, RowsQuery{View: View{Params: s.params}, Page: 1, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || len(page.Items[1].MetadataMissing) != 0 {
		t.Fatalf("rows = %+v", page.Items)
	}
}

func TestAPlanilhaRefusesAListColumnOrALeadFieldNamingTheVariable(t *testing.T) {
	s := newSheetWorld(t, "")
	preview := s.upload(t, campaignaudience.ChannelWhatsApp, sheetList, View{Params: s.params})
	for _, v := range []campaignaudience.Variable{{Key: sheetKey, Column: "planilha_de_gravacao"}, {Key: sheetKey, Source: campaign.BindFirstName}} {
		_, err := s.service.Count(ctx, testActor, campaignaudience.ChannelWhatsApp, s.params, listSpec(preview.ID, v))
		key, named := campaignaudience.VariableOf(err)
		if !errors.Is(err, campaignaudience.ErrSheetLiteralOnly) || !named || key != sheetKey {
			t.Fatalf("%+v = %v", v, err)
		}
	}
}

func TestAnUnchosenPlanilhaBlocksTheCampaign(t *testing.T) {
	s := newSheetWorld(t, "")
	preview := s.upload(t, campaignaudience.ChannelWhatsApp, sheetList, View{Params: s.params})
	count := s.count(t, listSpec(preview.ID))
	if !reflect.DeepEqual(count.Blockers, []campaignaudience.Blocker{campaignaudience.BlockerSheetRequired}) || count.Sendable == nil || *count.Sendable != 2 {
		t.Fatalf("count = %+v", count)
	}
	if len(s.fits.calls) != 0 {
		t.Fatalf("fit calls = %+v, nothing to check yet", s.fits.calls)
	}
	_, err := s.service.Recipients(ctx, testActor, campaignaudience.ChannelWhatsApp, s.params, listSpec(preview.ID))
	if key, _ := campaignaudience.VariableOf(err); !errors.Is(err, campaignaudience.ErrSheetRequired) || key != sheetKey {
		t.Fatalf("recipients = %v", err)
	}
}

func TestAPlanilhaThatDoesNotFitTheFlowBlocksTheCampaignWithTheReason(t *testing.T) {
	s := newSheetWorld(t, "")
	s.fits.misfits = map[string]string{"sp-norte": sheetfit.CodeTabMissing}
	preview := s.upload(t, campaignaudience.ChannelWhatsApp, sheetList, View{Params: s.params})
	spec := listSpec(preview.ID, campaignaudience.Variable{Key: sheetKey, Source: campaign.BindLiteral, Value: "sp-norte"})
	count := s.count(t, spec)
	if !reflect.DeepEqual(count.Blockers, []campaignaudience.Blocker{campaignaudience.BlockerSheetUnfit}) || variableOf(t, count, sheetKey).SheetUnfit != sheetfit.CodeTabMissing {
		t.Fatalf("count = %+v", count)
	}
	if _, err := s.service.Recipients(ctx, testActor, campaignaudience.ChannelWhatsApp, s.params, spec); !errors.Is(err, campaignaudience.ErrSheetUnfit) {
		t.Fatalf("recipients = %v", err)
	}
}

func TestAnUnreadableSpreadsheetFailsTheCountClosed(t *testing.T) {
	s := newSheetWorld(t, "sp-centro")
	s.fits.err = errors.New("google down")
	preview := s.upload(t, campaignaudience.ChannelWhatsApp, sheetList, View{Params: s.params})
	if _, err := s.service.Count(ctx, testActor, campaignaudience.ChannelWhatsApp, s.params, listSpec(preview.ID)); !errors.Is(err, campaignaudience.ErrUnavailable) {
		t.Fatalf("count = %v", err)
	}
}

func TestTheSampleLeavesThePlanilhaOut(t *testing.T) {
	s := newSheetWorld(t, "sp-centro")
	file, err := s.service.Sample(ctx, testActor, campaignaudience.ChannelWhatsApp, s.params, nil, "csv")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(file.Data); strings.Contains(got, sheetKey) || !strings.HasPrefix(got, "number;name\n") {
		t.Fatalf("sample = %q", got)
	}
}

func TestAnUnofficialCampaignRefusesAFlowThatWritesToAPlanilha(t *testing.T) {
	s := newSheetWorld(t, "sp-centro")
	rules, err := campaignautomation.NewStep(fakeWorkflowBook{"wf-sheet": franchiseFlow()}, noAgents{})
	if err != nil {
		t.Fatal(err)
	}
	ww := newWhatsAppWorld(t)
	ww.deps.Automation = rules.WithoutSheets()
	unofficial, err := NewUnofficialChannel(ww.deps)
	if err != nil {
		t.Fatal(err)
	}
	s.service.AddChannel(unofficial)
	params := Params{Message: &uwc.MessageSpec{Kind: uwc.KindText, Bodies: []string{"Oi"}}, Automation: s.params.Automation}
	preview := s.upload(t, campaignaudience.ChannelUnofficial, sheetList, View{})
	if _, err := s.service.Count(ctx, testActor, campaignaudience.ChannelUnofficial, params, listSpec(preview.ID)); !errors.Is(err, campaign.ErrSheetsOfficialOnly) {
		t.Fatalf("count = %v", err)
	}
}
