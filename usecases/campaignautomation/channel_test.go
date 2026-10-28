package campaignautomation

import (
	"errors"
	"testing"

	"vozko/domain/automationneeds"
	"vozko/domain/campaign"
	"vozko/domain/shared"
	"vozko/domain/workflow"
)

func templateWorkflow(workspaceID string) *workflow.Workflow {
	return &workflow.Workflow{ID: "wf-1", WorkspaceID: workspaceID, Name: "Boleto", Graph: workflow.Graph{
		Nodes: []workflow.Node{
			{ID: "t", Type: workflow.NodeTypeTriggerMessageReceived, Config: map[string]interface{}{}},
			{ID: "tpl", Type: workflow.NodeTypeActionSendTemplate, Config: map[string]interface{}{"template_id": "boleto"}},
		},
		Edges: []workflow.Edge{{Source: "t", Target: "tpl"}},
	}}
}

func templateStepRules(t *testing.T) *Step {
	t.Helper()
	step, err := NewStep(fakeWorkflows{byID: map[string]*workflow.Workflow{"wf-1": templateWorkflow("ws-1")}}, fakeAgents{})
	if err != nil {
		t.Fatal(err)
	}
	return step
}

func TestAnUnofficialCampaignCannotRunAWorkflowThatSendsTemplates(t *testing.T) {
	automation := campaign.Automation{WorkflowID: "wf-1", EnableWorkflow: true}
	err := templateStepRules(t).OnChannel(shared.EntryTypeUnofficialWhatsApp).Check("ws-1", automation, withSchool())
	refusal, ok := automationneeds.RefusalOf(err)
	if !errors.Is(err, automationneeds.ErrChannelUnsupported) || !ok || len(refusal.Channels) != 1 {
		t.Fatalf("err = %v", err)
	}
}

func TestAnOfficialCampaignRunsTheSameWorkflow(t *testing.T) {
	automation := campaign.Automation{WorkflowID: "wf-1", EnableWorkflow: true}
	if err := templateStepRules(t).OnChannel(shared.EntryTypeWhatsApp).Check("ws-1", automation, withSchool()); err != nil {
		t.Fatalf("err = %v", err)
	}
}
