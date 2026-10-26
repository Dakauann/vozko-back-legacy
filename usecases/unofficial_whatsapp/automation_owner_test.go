package unofficial_whatsapp

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/shared/deps"
	conversation_usecase "vozko/usecases/conversation"
	"vozko/usecases/conversation/automationgatetest"
)

func openAutomationGate() *conversation_usecase.AutomationGate { return automationgatetest.Open() }

func TestOnUnofficialWhatsAppOwnershipDecidesWhetherTheWorkflowIsTriggered(t *testing.T) {
	for _, tc := range automationgatetest.Cases(conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "inst-workflow"}) {
		t.Run(tc.Name, func(t *testing.T) {
			h := newAutoHarness(t, nil)
			h.uc.gate = tc.Gate

			h.deliverInbound(t)

			if triggered := len(h.workflows.all()) > 0; triggered != tc.Answers {
				t.Fatalf("workflow events = %d, want triggered %v", len(h.workflows.all()), tc.Answers)
			}
		})
	}
}

func TestOnUnofficialWhatsAppOwnershipDecidesWhetherTheAgentAnswers(t *testing.T) {
	for _, tc := range automationgatetest.Cases(conversation.Automation{Kind: conversation.AutomationAgent, ID: "inst-agent"}) {
		t.Run(tc.Name, func(t *testing.T) {
			h := newAutoHarness(t, nil)
			h.uc.gate = tc.Gate
			instance, err := h.uc.instances.FindByID(context.Background(), "inst-1")
			if err != nil {
				t.Fatal(err)
			}
			instance.EnableWorkflow = false

			h.deliverInbound(t)

			if answered := len(h.ai.all()) == 1; answered != tc.Answers {
				t.Fatalf("agent replies = %d, want answered %v", len(h.ai.all()), tc.Answers)
			}
		})
	}
}

func TestTheUnofficialWebhookRefusesToBuildWithoutTheAutomationGate(t *testing.T) {
	_, err := NewHandleWebhookUseCase(HandleWebhookDeps{
		LinkGate: openLinkGate{}, Listener: quietConnections{}, Handover: NewLineHandover(noLineSiblings{}),
		GroupWatcher: quietGroupWatcher{}, CampaignSink: quietCampaignSink{}, CampaignAutomation: noCampaignAutomation{},
		ExitStep: noLeadExit{}, HistoryWindow: 1,
	})
	var missing *deps.MissingError
	if !errors.As(err, &missing) || len(missing.Missing) != 1 || missing.Missing[0] != "automation gate" {
		t.Fatalf("err = %v, want the automation gate named", err)
	}
}
