package conversation_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/conversation"
	wc "vozko/domain/whatsapp_campaign"
)

type officialOwnerCase struct {
	name        string
	owners      *gateOwners
	delegations stubDelegations
	answers     bool
}

func officialOwnerCases(delegate conversation.AutomationKind, delegateID string) []officialOwnerCase {
	return []officialOwnerCase{
		{name: "unowned", owners: ownedBy(""), answers: true},
		{name: "owned by the agent", owners: ownedBy("ai:agent-1"), answers: true},
		{name: "owned by a workflow", owners: ownedBy("workflow:wf-1"), answers: true},
		{name: "delegated by the person who owns it", owners: ownedBy("user-1"), delegations: delegatedTo(delegate, delegateID), answers: true},
		{name: "owned by a person", owners: ownedBy("user-1")},
		{name: "owner unreadable", owners: &gateOwners{err: errors.New("db down")}},
	}
}

func TestOnOfficialWhatsAppOwnershipDecidesWhetherTheAgentAnswers(t *testing.T) {
	for _, tc := range officialOwnerCases(conversation.AutomationAgent, "agent-1") {
		t.Run(tc.name, func(t *testing.T) {
			h := agentHeldWhatsApp(&waitingRunEvaluator{})
			h.uc.automationGate = gateWith(tc.owners, tc.delegations)

			if err := h.uc.Execute(context.Background(), customerText("12345678900")); err != nil {
				t.Fatal(err)
			}

			if answered := len(h.ai.inputs) == 1 && len(h.client.sent) == 1; answered != tc.answers {
				t.Fatalf("generated %d, sent %d, want answered %v", len(h.ai.inputs), len(h.client.sent), tc.answers)
			}
			if len(h.history.records) == 0 || h.history.records[0].Text != "12345678900" {
				t.Fatalf("records = %+v, the message must still be recorded", h.history.records)
			}
		})
	}
}

func TestOnOfficialWhatsAppOwnershipDecidesWhetherTheWorkflowIsTriggered(t *testing.T) {
	for _, tc := range officialOwnerCases(conversation.AutomationWorkflow, "wf-1") {
		t.Run(tc.name, func(t *testing.T) {
			h := heldWhatsApp(&waitingRunEvaluator{}, &wc.Campaign{ID: "camp-1", WorkspaceID: "ws-1", BusinessPhoneID: "phone-1", WorkflowID: "wf-1", EnableWorkflow: true})
			h.uc.automationGate = gateWith(tc.owners, tc.delegations)

			if err := h.uc.Execute(context.Background(), customerText("12345678900")); err != nil {
				t.Fatal(err)
			}

			if tc.answers {
				eventually(t, "the workflow trigger", func() bool { _, evaluated := h.evaluator.counts(); return evaluated > 0 })
				return
			}
			time.Sleep(20 * time.Millisecond)
			if _, evaluated := h.evaluator.counts(); evaluated != 0 || len(h.ai.inputs) != 0 {
				t.Fatalf("evaluated %d, generated %d on a conversation automation may not touch", evaluated, len(h.ai.inputs))
			}
		})
	}
}

func TestOnOfficialWhatsAppAPersonsConversationDoesNotWakeAWaitingRun(t *testing.T) {
	h := agentHeldWhatsApp(&waitingRunEvaluator{waiting: waitingRun()})
	h.uc.automationGate = gateWith(ownedBy("user-1"), stubDelegations{})

	if err := h.uc.Execute(context.Background(), customerText("12345678900")); err != nil {
		t.Fatal(err)
	}

	time.Sleep(20 * time.Millisecond)
	if woken, evaluated := h.evaluator.counts(); woken != 0 || evaluated != 0 || len(h.ai.inputs) != 0 {
		t.Fatalf("woken %d, evaluated %d, generated %d; the person owns the conversation", woken, evaluated, len(h.ai.inputs))
	}
}
