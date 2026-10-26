package unofficial_whatsapp

import (
	"errors"
	"testing"

	"vozko/domain/campaign"
	"vozko/domain/conversation"
	"vozko/usecases/conversation/automationgatetest"
)

func TestADelegatedAgentAnswersAManualCampaign(t *testing.T) {
	h := newAutoHarness(t, campaignAuto(campaign.Automation{}))
	h.uc.gate = automationgatetest.Gate(automationgatetest.Owners{}, automationgatetest.Delegations{To: &conversation.Automation{Kind: conversation.AutomationAgent, ID: "picked"}})
	h.deliverInbound(t)

	if reqs := h.ai.all(); len(reqs) != 1 || reqs[0].AgentID != "picked" || !reqs[0].AgentResponsesEnabled {
		t.Fatalf("AI replies = %+v, want one from the delegated agent", reqs)
	}
	if events := h.workflows.all(); len(events) != 0 {
		t.Fatalf("fired %d workflow triggers beside the delegated agent", len(events))
	}
}

func TestADelegatedWorkflowReplacesTheInstanceAutomation(t *testing.T) {
	h := newAutoHarness(t, nil)
	h.uc.gate = automationgatetest.Gate(automationgatetest.Owners{}, automationgatetest.Delegations{To: &conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "picked-wf"}})
	h.deliverInbound(t)

	events := h.workflows.all()
	if len(events) != 1 || events[0].Data["account_workflow_id"] != "picked-wf" {
		t.Fatalf("workflow events = %+v, want only the delegated workflow", events)
	}
	if reqs := h.ai.all(); len(reqs) != 0 {
		t.Fatalf("the instance agent answered beside the delegated workflow: %+v", reqs)
	}
}

func TestAnUnreadableDelegationSilencesTheInstance(t *testing.T) {
	h := newAutoHarness(t, nil)
	h.uc.gate = automationgatetest.Gate(automationgatetest.Owners{}, automationgatetest.Delegations{Err: errors.New("db down")})
	h.deliverInbound(t)

	if len(h.ai.all()) != 0 || len(h.workflows.all()) != 0 {
		t.Fatal("an unknown delegation must not let the instance automation answer")
	}
}
