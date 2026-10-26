package inbox_assignment_usecase_test

import (
	"context"
	"testing"

	"vozko/domain/conversation"
	ia "vozko/domain/inbox_assignment"
	"vozko/domain/shared"
	conversation_usecase "vozko/usecases/conversation"
	"vozko/usecases/conversation/automationgatetest"
	inbox_assignment_usecase "vozko/usecases/inbox_assignment"
)

type gatedChannel struct {
	assignments *inbox_assignment_usecase.AssignmentService
	gate        *conversation_usecase.AutomationGate
	automation  *conversation_usecase.InboundAutomation
	replies     *automationgatetest.Replies
	triggers    *automationgatetest.Triggers
}

func newGatedChannel() gatedChannel {
	assignments, owners := inbox_assignment_usecase.NewOpenClaimFixture()
	gate := conversation_usecase.NewAutomationGate(conversation_usecase.AutomationGateDeps{
		Owners:      owners,
		Delegations: automationgatetest.Delegations{},
		Profiles:    automationgatetest.Profiles{Profile: conversation.AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true}},
	})
	replies, triggers := &automationgatetest.Replies{}, &automationgatetest.Triggers{}
	return gatedChannel{
		assignments: assignments,
		gate:        gate,
		automation:  conversation_usecase.NewInboundAutomation(triggers, replies, nil, nil, gate),
		replies:     replies,
		triggers:    triggers,
	}
}

func (c gatedChannel) customerWrites(entryType shared.EntryType) {
	agent, flow := "agent-1", "wf-1"
	c.automation.Consume(context.Background(), conversation_usecase.InboundAutomationInput{
		WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: entryType, Text: "oi",
		Config: conversation.ChannelAutomation{AgentID: &agent, EnableAgentResponses: true, WorkflowID: &flow, EnableWorkflow: true},
	})
}

func TestOpeningAnUnownedConversationStopsTheChannelAutomationFromAnsweringIt(t *testing.T) {
	c := newGatedChannel()
	c.customerWrites(shared.EntryTypeInstagram)
	if c.replies.Count() != 1 || c.triggers.Count() == 0 {
		t.Fatalf("replies %d, triggers %d; an unowned conversation is the automation's to answer", c.replies.Count(), c.triggers.Count())
	}
	repliesBefore, triggersBefore := c.replies.Count(), c.triggers.Count()

	claimed, err := c.assignments.AssignOnOpen("entry-1", string(shared.EntryTypeInstagram), "ana", false)
	if err != nil || !claimed {
		t.Fatalf("claimed %v, %v", claimed, err)
	}
	c.customerWrites(shared.EntryTypeInstagram)

	if c.replies.Count() != repliesBefore || c.triggers.Count() != triggersBefore {
		t.Fatalf("replies %d, triggers %d after ana opened it; the automation answered her conversation", c.replies.Count(), c.triggers.Count())
	}
}

func TestAnInConversationTemplateClaimStopsTheNumbersAutomationFromAnswering(t *testing.T) {
	c := newGatedChannel()
	admission := conversation_usecase.AutomationAdmission{WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: shared.EntryTypeWhatsApp}
	if _, mayAct := c.gate.Admit(context.Background(), admission); !mayAct {
		t.Fatal("the number's automation must answer an unowned conversation")
	}

	claimed, err := c.assignments.ClaimIfUnassigned("entry-1", string(shared.EntryTypeWhatsApp), "phone-1", "ws-1", "ana", ia.TriggerOutreachSent)
	if err != nil || !claimed {
		t.Fatalf("claimed %v, %v", claimed, err)
	}

	if _, mayAct := c.gate.Admit(context.Background(), admission); mayAct {
		t.Fatal("the reply to ana's template went to the number's automation")
	}
	if mayAct, err := c.gate.MayAct("ws-1", "entry-1", shared.EntryTypeWhatsApp); err != nil || mayAct {
		t.Fatalf("a waiting run may act = %v, %v after ana's template", mayAct, err)
	}
	if _, governed, err := c.gate.Governing("ws-1", "entry-1", shared.EntryTypeWhatsApp); err != nil || governed {
		t.Fatalf("the agent may continue = %v, %v after ana's template", governed, err)
	}
}

func TestAnAgentSwitchedOnAfterAPersonTookTheConversationStaysOut(t *testing.T) {
	c := newGatedChannel()
	claimed, err := c.assignments.AssignOnOpen("entry-1", string(shared.EntryTypeTelegram), "ana", false)
	if err != nil || !claimed {
		t.Fatalf("claimed %v, %v", claimed, err)
	}

	c.customerWrites(shared.EntryTypeTelegram)

	if c.replies.Count() != 0 || c.triggers.Count() != 0 {
		t.Fatalf("replies %d, triggers %d; the channel's new agent answered ana's conversation", c.replies.Count(), c.triggers.Count())
	}
}
