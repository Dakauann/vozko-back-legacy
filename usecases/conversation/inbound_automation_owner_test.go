package conversation_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/conversation"
)

func gatedAutomation(gate *AutomationGate) (*InboundAutomation, *recordingEvaluator, *recordingReplier) {
	ev, rep := &recordingEvaluator{}, &recordingReplier{}
	return NewInboundAutomation(ev, rep, &recordingScheduler{}, stubInboundCounter{count: 1}, gate), ev, rep
}

func workflowOnly() conversation.ChannelAutomation {
	wf := "wf-1"
	return conversation.ChannelAutomation{WorkflowID: &wf, EnableWorkflow: true}
}

func TestOnEveryGenericChannelOwnershipDecidesWhetherAutomationAnswers(t *testing.T) {
	cases := []struct {
		name        string
		owners      *gateOwners
		delegations stubDelegations
		answers     bool
	}{
		{name: "unowned", owners: ownedBy(""), answers: true},
		{name: "owned by the agent", owners: ownedBy("ai:agent-1"), answers: true},
		{name: "owned by a workflow", owners: ownedBy("workflow:wf-1"), answers: true},
		{name: "delegated by the person who owns it", owners: ownedBy("user-1"), delegations: delegatedTo(conversation.AutomationAgent, "agent-1"), answers: true},
		{name: "owned by a person", owners: ownedBy("user-1")},
		{name: "owner unreadable", owners: &gateOwners{err: errors.New("db down")}},
	}
	for _, tc := range cases {
		t.Run(tc.name+": agent", func(t *testing.T) {
			auto, ev, rep := gatedAutomation(gateWith(tc.owners, tc.delegations))
			in := automationInput()
			in.Config = agentOnly()

			auto.Dispatch(context.Background(), in)

			if answered := len(rep.reqs) == 1; answered != tc.answers {
				t.Fatalf("agent replies = %d, want answered %v", len(rep.reqs), tc.answers)
			}
			if len(ev.events) != 0 {
				t.Fatalf("an agent-only channel fired workflows: %+v", ev.events)
			}
		})
		t.Run(tc.name+": workflow", func(t *testing.T) {
			delegations := tc.delegations
			if delegations.delegation != nil {
				delegations = delegatedTo(conversation.AutomationWorkflow, "wf-1")
			}
			auto, ev, rep := gatedAutomation(gateWith(tc.owners, delegations))
			in := automationInput()
			in.Config = workflowOnly()

			auto.Dispatch(context.Background(), in)

			if triggered := len(ev.events) > 0; triggered != tc.answers {
				t.Fatalf("workflow events = %d, want triggered %v", len(ev.events), tc.answers)
			}
			if len(rep.reqs) != 0 {
				t.Fatalf("a workflow-only channel called the agent: %+v", rep.reqs)
			}
		})
	}
}

func TestAPersonsConversationStillGetsItsAnalysis(t *testing.T) {
	ev, rep, sch := &recordingEvaluator{}, &recordingReplier{}, &recordingScheduler{}
	auto := NewInboundAutomation(ev, rep, sch, stubInboundCounter{count: 1}, gateWith(ownedBy("user-1"), stubDelegations{}))

	auto.Dispatch(context.Background(), automationInput())

	if len(rep.reqs) != 0 || len(ev.events) != 0 {
		t.Fatalf("replies %d, events %d on a person's conversation", len(rep.reqs), len(ev.events))
	}
	if len(sch.scheduled) != 1 {
		t.Fatalf("scheduled = %v, analysis is not an automated reply and keeps running", sch.scheduled)
	}
}

func TestAPersonsConversationDoesNotWakeAWaitingRun(t *testing.T) {
	evaluator := &waitingRunEvaluator{waiting: waitingRun()}
	auto := NewInboundAutomation(evaluator, &recordingReplier{}, &recordingScheduler{}, stubInboundCounter{count: 2}, gateWith(ownedBy("user-1"), stubDelegations{}))

	auto.Consume(context.Background(), automationInput())

	if woken, evaluated := evaluator.counts(); woken != 0 || evaluated != 0 {
		t.Fatalf("woken %d, evaluated %d; the run must not answer over the person", woken, evaluated)
	}
}

func TestInboundAutomationRefusesToBuildWithoutTheGate(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("inbound automation without the gate would answer without asking who owns the conversation")
		}
	}()
	NewInboundAutomation(nil, nil, nil, nil, nil)
}
