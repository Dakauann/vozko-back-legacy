package campaignautomation

import (
	"errors"
	"testing"

	"vozko/domain/campaign"
	"vozko/domain/workflow"
)

func franchiseWorkflow() *workflow.Workflow {
	return &workflow.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Graph: workflow.Graph{Nodes: []workflow.Node{
		{ID: "row", Type: workflow.NodeTypeActionAddSheetRow, Config: map[string]interface{}{
			workflow.SheetStepSheetKey: "model-pag", workflow.SheetStepVariableKey: "{{campvars.planilha}}", workflow.SheetStepValuesKey: map[string]interface{}{"valor": "1"},
		}},
	}}}
}

func TestOnlyOfficialCampaignsCanChooseAPlanilhaPerConversation(t *testing.T) {
	step := newStep(t, fakeWorkflows{byID: map[string]*workflow.Workflow{"wf-1": franchiseWorkflow()}}, fakeAgents{})
	automation := campaign.Automation{WorkflowID: "wf-1", EnableWorkflow: true}
	entries := []campaign.EntryMetadata{{Label: "lead 1", Metadata: map[string]any{"planilha": "sp-centro"}}}
	if err := step.Check("ws-1", automation, entries); err != nil {
		t.Fatalf("an official campaign = %v", err)
	}
	if err := step.WithoutSheets().Check("ws-1", automation, entries); !errors.Is(err, campaign.ErrSheetsOfficialOnly) {
		t.Fatalf("an unofficial campaign = %v", err)
	}
	plain := newStep(t, fakeWorkflows{byID: map[string]*workflow.Workflow{"wf-1": needsSchoolWorkflow("ws-1")}}, fakeAgents{})
	if err := plain.WithoutSheets().Check("ws-1", automation, withSchool()); err != nil {
		t.Fatalf("a flow without a Planilha = %v", err)
	}
}
