package automationneeds

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agent"
	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/domain/sheetfit"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

type fakeSheetFiles struct {
	down   bool
	seen   []string
	uses   []sheetfit.Use
	chosen string
}

func (f *fakeSheetFiles) Resolve(_ context.Context, _ string, t variabledecl.Type, uses []sheetfit.Use, value string) (workflow.SheetFile, error) {
	f.seen = append(f.seen, value)
	switch {
	case f.down:
		return workflow.SheetFile{}, errors.New("sheets down")
	case t.Settings[variabledecl.SettingModel] != "sp-model" || len(uses) == 0:
		return workflow.SheetFile{}, &sheetfit.Misfit{Code: sheetfit.CodeModelMissing}
	case value == "sp-norte":
		return workflow.SheetFile{}, &sheetfit.Misfit{Code: sheetfit.CodeTabMissing}
	}
	return workflow.SheetFile{SpreadsheetID: value, Tabs: map[string]string{"model-pag": value + "-pag"}}, nil
}

func planilha(fallback string) variabledecl.Declaration {
	settings := map[string]string{variabledecl.SettingModel: "sp-model"}
	if fallback != "" {
		settings[variabledecl.SettingDefault] = fallback
	}
	return variabledecl.Declaration{Name: "planilha", Label: "Planilha da franquia", Type: variabledecl.Type{Kind: variabledecl.KindSheet, Settings: settings}}
}

func sheetService(t *testing.T, values fakeValues, fallback string, files *fakeSheetFiles) *Service {
	t.Helper()
	row := workflow.Node{ID: "row", Type: workflow.NodeTypeActionAddSheetRow, Config: map[string]interface{}{
		workflow.SheetStepSheetKey: "model-pag", workflow.SheetStepVariableKey: "{{campvars.planilha}}", workflow.SheetStepValuesKey: map[string]interface{}{"valor": "1"},
	}}
	s, err := NewService(ServiceDeps{
		Source: PublishedSource{
			Agents:    fakeAgents{},
			Workflows: fakeWorkflows{"franquias": {ID: "franquias", WorkspaceID: "ws", Name: "Franquias", Graph: workflow.Graph{Nodes: []workflow.Node{row}}}},
		},
		Values:       values,
		Declarations: fakeDeclarations{rows: []variabledecl.Declaration{planilha(fallback)}},
		Sheets:       files,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func franquiasRun() (*workflow.WorkflowRun, *workflow.Node) {
	run, node := triggeredRun("e1", "whatsapp")
	run.WorkflowID = "franquias"
	return run, node
}

func sheetRuntime(t *testing.T, s *Service) (*Runtime, *recordedPublisher) {
	t.Helper()
	publisher := &recordedPublisher{}
	alerts, _ := NewAlerts(publisher)
	r, err := NewRuntime(s, alerts)
	if err != nil {
		t.Fatal(err)
	}
	return r, publisher
}

func TestTheRunStartPinsTheSpreadsheetOfTheConversation(t *testing.T) {
	files := &fakeSheetFiles{}
	r, _ := sheetRuntime(t, sheetService(t, fakeValues{values: map[string]string{"planilha": "sp-centro"}}, "", files))
	run, node := franquiasRun()
	if _, err := r.BeforeStep(run, node); err != nil {
		t.Fatalf("BeforeStep: %v", err)
	}
	pinned := workflow.PinnedSheetFiles(&run.State)
	if pinned["planilha"].SpreadsheetID != "sp-centro" || pinned["planilha"].Tabs["model-pag"] != "sp-centro-pag" {
		t.Fatalf("pinned = %+v", pinned)
	}
}

func TestARunWithoutAFittingSpreadsheetNeverStarts(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]string
		files  *fakeSheetFiles
		cause  error
	}{
		{name: "no value", values: map[string]string{}, files: &fakeSheetFiles{}, cause: domain.ErrConversationMissing},
		{name: "a file that does not fit", values: map[string]string{"planilha": "sp-norte"}, files: &fakeSheetFiles{}, cause: domain.ErrSheetsUnmet},
		{name: "the files cannot be read", values: map[string]string{"planilha": "sp-centro"}, files: &fakeSheetFiles{down: true}, cause: domain.ErrUnreadable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, publisher := sheetRuntime(t, sheetService(t, fakeValues{values: c.values}, "", c.files))
			run, node := franquiasRun()
			if _, err := r.BeforeStep(run, node); !errors.Is(err, c.cause) {
				t.Fatalf("err = %v, want %v", err, c.cause)
			}
			if workflow.PinnedSheetFiles(&run.State) != nil {
				t.Fatal("a refused run pinned a file")
			}
			if c.cause != domain.ErrUnreadable && len(publisher.drafts) != 1 {
				t.Fatalf("drafts = %+v", publisher.drafts)
			}
		})
	}
}

func TestADeclaredDefaultFillsTheSpreadsheetOfInboundConversations(t *testing.T) {
	files := &fakeSheetFiles{}
	s := sheetService(t, fakeValues{values: map[string]string{}}, "sp-padrao", files)
	r, _ := sheetRuntime(t, s)
	run, node := franquiasRun()
	if _, err := r.BeforeStep(run, node); err != nil || workflow.PinnedSheetFiles(&run.State)["planilha"].SpreadsheetID != "sp-padrao" {
		t.Fatalf("BeforeStep = %v, pinned %+v", err, workflow.PinnedSheetFiles(&run.State))
	}
	channel := conversation.ChannelAutomation{WorkflowID: id("franquias"), EnableWorkflow: true}
	if err := s.AdmitReceptive("ws", domain.SlotInstagram, channel); err != nil {
		t.Fatalf("a Planilha with a default = %v", err)
	}
	strict := sheetService(t, fakeValues{}, "", &fakeSheetFiles{})
	if err := strict.AdmitReceptive("ws", domain.SlotInstagram, channel); !errors.Is(err, domain.ErrReceptiveUnmet) {
		t.Fatalf("a Planilha without a default = %v", err)
	}
}

func TestAHandOffChecksTheExactFitOfThePendingSpreadsheet(t *testing.T) {
	s := sheetService(t, fakeValues{values: map[string]string{}}, "", &fakeSheetFiles{})
	target := ref(conversation.AutomationWorkflow, "franquias")
	if err := s.ConversationMissingWith(context.Background(), "ws", "e1", shared.EntryTypeWhatsApp, target, map[string]string{"planilha": "sp-norte"}); !errors.Is(err, domain.ErrSheetsUnmet) {
		t.Fatalf("an unfit file = %v", err)
	}
	if err := s.ConversationMissingWith(context.Background(), "ws", "e1", shared.EntryTypeWhatsApp, target, map[string]string{"planilha": "sp-centro"}); err != nil {
		t.Fatalf("a fitting file = %v", err)
	}
}

func TestFlowsWithoutAPlanilhaNeverReadTheSpreadsheets(t *testing.T) {
	files := &fakeSheetFiles{down: true}
	s := newService(t, fakeValues{values: map[string]string{"valor": "10"}})
	s.sheets = files
	if _, err := s.ValuesFor(context.Background(), "ws", "e1", shared.EntryTypeWhatsApp, ref(conversation.AutomationWorkflow, "cobranca")); err != nil || len(files.seen) != 0 {
		t.Fatalf("err = %v, seen %v", err, files.seen)
	}
}

func TestAnAgentReadsTheSpreadsheetOfItsConversation(t *testing.T) {
	tool := map[string]interface{}{workflow.SheetStepSheetKey: "model-pag", workflow.SheetStepVariableKey: "{{campvars.planilha}}"}
	s, err := NewService(ServiceDeps{
		Source: PublishedSource{
			Agents:    fakeAgents{"cobrador": {ID: "cobrador", WorkspaceID: "ws", Name: "Cobrador", InternalTools: []agent.ToolBinding{{Name: workflow.AgentSheetRowTool, Config: tool}}}},
			Workflows: fakeWorkflows{},
		},
		Values:       fakeValues{values: map[string]string{"planilha": "sp-centro"}},
		Declarations: fakeDeclarations{rows: []variabledecl.Declaration{planilha("")}},
		Sheets:       &fakeSheetFiles{},
	})
	if err != nil {
		t.Fatal(err)
	}
	files, err := s.AgentSheetFiles(context.Background(), "ws", "e1", shared.EntryTypeWhatsApp, "cobrador")
	if err != nil || files["planilha"].Tabs["model-pag"] != "sp-centro-pag" {
		t.Fatalf("files = %+v, %v", files, err)
	}
}

func (f *fakeSheetFiles) Candidates(_ context.Context, _ string, t variabledecl.Type, uses []sheetfit.Use, chosen string) ([]sheetfit.Candidate, error) {
	if f.down {
		return nil, errors.New("sheets down")
	}
	f.uses, f.chosen = uses, chosen
	return []sheetfit.Candidate{{File: googlesheet.Spreadsheet{ID: "sp-centro"}}}, nil
}

func TestThePickerListsFilesForTheExactUsesOfTheAutomation(t *testing.T) {
	files := &fakeSheetFiles{}
	s := sheetService(t, fakeValues{}, "", files)
	target := ref(conversation.AutomationWorkflow, "franquias")
	chosen := sheetfit.Choice{SpreadsheetID: "sp-centro", Tabs: map[string]string{"model-pag": "sp-centro-pag"}}.Value()
	got, err := s.SheetCandidates(context.Background(), "ws", &target, "planilha", chosen)
	if err != nil || !got.Exact || files.chosen != chosen || len(files.uses) != 1 || files.uses[0].ModelSheetID != "model-pag" || len(got.Candidates) != 1 || got.Variable.Label != "Planilha da franquia" {
		t.Fatalf("SheetCandidates = %+v, %v, uses %+v", got, err, files.uses)
	}
	whole, err := s.SheetCandidates(context.Background(), "ws", nil, "planilha", "")
	if err != nil || whole.Exact || files.uses != nil {
		t.Fatalf("without an automation = %+v, %v, uses %+v", whole, err, files.uses)
	}
	if _, err := s.SheetCandidates(context.Background(), "ws", nil, "matricula", ""); !errors.Is(err, ErrNotASheet) {
		t.Fatalf("not a Planilha = %v", err)
	}
	broken := sheetService(t, fakeValues{}, "", &fakeSheetFiles{down: true})
	if _, err := broken.SheetCandidates(context.Background(), "ws", nil, "planilha", ""); !errors.Is(err, domain.ErrUnreadable) {
		t.Fatalf("unreadable = %v", err)
	}
}

func (f *fakeSheetFiles) Attention(_ context.Context, _ string, file workflow.SheetFile) (bool, error) {
	return file.SpreadsheetID == "sp-busy", nil
}

func TestACampaignSpreadsheetIsCheckedForFitAndAttentionBeforeLaunch(t *testing.T) {
	s := sheetService(t, fakeValues{}, "", &fakeSheetFiles{})
	targets := []conversation.Automation{ref(conversation.AutomationWorkflow, "franquias")}
	cases := []struct {
		name   string
		values map[string]string
		want   SheetLaunch
	}{
		{name: "a file that fits", values: map[string]string{"planilha": "sp-centro"}, want: SheetLaunch{Checked: true}},
		{name: "a file that does not fit", values: map[string]string{"planilha": "sp-norte"}, want: SheetLaunch{Checked: true, Unfit: true}},
		{name: "a tab that needs attention", values: map[string]string{"planilha": "sp-busy"}, want: SheetLaunch{Checked: true, Attention: true}},
		{name: "chosen per row", values: map[string]string{}, want: SheetLaunch{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := s.CampaignSheets(context.Background(), "ws", targets, c.values)
			if err != nil || got != c.want {
				t.Fatalf("CampaignSheets = %+v, %v", got, err)
			}
		})
	}
	broken := sheetService(t, fakeValues{}, "", &fakeSheetFiles{down: true})
	if _, err := broken.CampaignSheets(context.Background(), "ws", targets, map[string]string{"planilha": "sp-centro"}); err == nil {
		t.Fatal("an unreadable check must not pass")
	}
}
