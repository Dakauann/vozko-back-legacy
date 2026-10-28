package workflow_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/domain/workflow"
)

type fixedChannels struct {
	channels []shared.EntryType
	err      error
	asked    []string
}

func (f *fixedChannels) ChannelsOf(_ context.Context, w *workflow.Workflow) ([]shared.EntryType, error) {
	f.asked = append(f.asked, w.ID)
	return f.channels, f.err
}

const leadsSheetID = "leads"

func leadsWorkbook(required bool) map[string]*googlesheet.Sheet {
	return map[string]*googlesheet.Sheet{
		leadsSheetID: {ID: leadsSheetID, WorkspaceID: "ws1", Title: "Leads", TabName: "Leads", Columns: []googlesheet.Column{
			{Key: "nome", Header: "Nome", Type: googlesheet.TypeText, Source: googlesheet.SourceLeadName},
			{Key: "telefone", Header: "Telefone", Type: googlesheet.TypePhone, Source: googlesheet.SourceLeadPhone, Required: required},
		}},
	}
}

func channelDraft(step workflow.Node) *workflow.Workflow {
	return &workflow.Workflow{ID: "wf-1", WorkspaceID: "ws1", Type: workflow.WorkflowTypeMessages, Graph: workflow.Graph{
		Nodes: []workflow.Node{{ID: "t", Type: workflow.NodeTypeTriggerMessageReceived, Config: map[string]interface{}{}}, step, {ID: "end", Type: workflow.NodeTypeEnd}},
		Edges: []workflow.Edge{{Source: "t", Target: step.ID}, {Source: step.ID, Target: "end", Label: workflow.SheetStepWritten}},
	}}
}

func leadsRow() workflow.Node {
	return workflow.Node{ID: "n1", Type: workflow.NodeTypeActionAddSheetRow, Config: map[string]interface{}{"sheet_id": leadsSheetID, "values": map[string]interface{}{}}}
}

func templateNode() workflow.Node {
	return workflow.Node{ID: "n1", Type: workflow.NodeTypeActionSendTemplate, Config: map[string]interface{}{"template_id": "tpl-1"}}
}

func channelLintUseCase(sheets GoogleSheetLookup, channels WorkflowChannels) *activateWorkflowUseCase {
	return &activateWorkflowUseCase{repo: NewMockWorkflowRepository(), catalogFn: sheetStepCatalog, googleSheets: sheets, channels: channels}
}

func instagramAndWhatsApp() *fixedChannels {
	return &fixedChannels{channels: []shared.EntryType{shared.EntryTypeWhatsApp, shared.EntryTypeInstagram}}
}

func TestAPhoneColumnOnAWorkflowLinkedToInstagramWarnsInTheEditor(t *testing.T) {
	channels := instagramAndWhatsApp()
	report, err := channelLintUseCase(sheetsByID{sheets: leadsWorkbook(true)}, channels).Lint(context.Background(), channelDraft(leadsRow()))
	if err != nil {
		t.Fatal(err)
	}
	found := issuesCoded(report.Issues, workflow.LintChannelDataMayFail)
	if len(found) != 1 || found[0].Severity != workflow.SeverityAdvisory || found[0].Values["channels"] != "Instagram" || found[0].Values["subject"] != "Telefone" {
		t.Fatalf("issues = %+v", report.Issues)
	}
	if len(channels.asked) != 1 || channels.asked[0] != "wf-1" {
		t.Fatalf("asked = %v", channels.asked)
	}
}

func TestAnOptionalPhoneColumnOnInstagramIsAnEditorNote(t *testing.T) {
	report, err := channelLintUseCase(sheetsByID{sheets: leadsWorkbook(false)}, instagramAndWhatsApp()).Lint(context.Background(), channelDraft(leadsRow()))
	if err != nil {
		t.Fatal(err)
	}
	if found := issuesCoded(report.Issues, workflow.LintChannelDataMayBeEmpty); len(found) != 1 || found[0].Severity != workflow.SeverityInfo {
		t.Fatalf("issues = %+v", report.Issues)
	}
}

func TestPublishingATemplateStepOnAWorkflowLinkedToInstagramIsRefused(t *testing.T) {
	err := channelLintUseCase(sheetsByID{}, instagramAndWhatsApp()).validateRunnable(context.Background(), channelDraft(templateNode()))
	blocked, ok := workflow.NotPublishableOf(err)
	if !ok {
		t.Fatalf("err = %v", err)
	}
	found := issuesCoded(blocked.Issues, workflow.LintChannelStepUnsupported)
	if len(found) != 1 || found[0].NodeID != "n1" || found[0].Values["channels"] != "Instagram" {
		t.Fatalf("issues = %+v", blocked.Issues)
	}
}

func TestChannelsThatCannotBeReadBlockPublishing(t *testing.T) {
	channels := &fixedChannels{err: errors.New("slots down")}
	report, err := channelLintUseCase(sheetsByID{sheets: leadsWorkbook(true)}, channels).Lint(context.Background(), channelDraft(leadsRow()))
	if err != nil {
		t.Fatal(err)
	}
	if found := issuesCoded(report.Blocking(), workflow.LintValidationUnavailable); len(found) != 1 {
		t.Fatalf("issues = %+v", report.Issues)
	}
}
