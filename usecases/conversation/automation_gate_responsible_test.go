package conversation_usecase

import (
	"errors"
	"fmt"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

func responsibilityGate(owners *gateOwners, profiles gateProfiles) *AutomationGate {
	return NewAutomationGate(AutomationGateDeps{Owners: owners, Delegations: stubDelegations{}, Profiles: profiles})
}

func TestTheGateNamesTheWorkflowResponsibleForTheConversation(t *testing.T) {
	delegate := conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "wf-delegate"}
	cases := []struct {
		name    string
		profile conversation.AutomationProfile
		wantID  string
		wantOK  bool
	}{
		{"the container's workflow", conversation.AutomationProfile{WorkflowID: "wf-1", WorkflowEnabled: true}, "wf-1", true},
		{"a delegated workflow", conversation.AutomationProfile{WorkflowID: "wf-1", WorkflowEnabled: true, Delegate: &delegate}, "wf-delegate", true},
		{"an agent container", conversation.AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true}, "", false},
		{"workflows on with none chosen", conversation.AutomationProfile{WorkflowEnabled: true}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owners := ownedBy("user-1")
			id, ok, err := responsibilityGate(owners, gateProfiles{profile: tc.profile}).ResponsibleWorkflow("ws-1", "e-1", shared.EntryTypeInstagram)
			if err != nil || id != tc.wantID || ok != tc.wantOK {
				t.Fatalf("ResponsibleWorkflow = %q, %v, %v; want %q, %v", id, ok, err, tc.wantID, tc.wantOK)
			}
			if len(owners.asked) != 0 {
				t.Fatalf("owner read %v: responsibility does not depend on who holds the conversation", owners.asked)
			}
		})
	}
}

func TestAnUnreadableAutomationIsAnErrorSoNoRunStarts(t *testing.T) {
	down := errors.New("db down")
	_, ok, err := responsibilityGate(ownedBy(""), gateProfiles{err: down}).ResponsibleWorkflow("ws-1", "e-1", shared.EntryTypeWhatsApp)
	if ok || !errors.Is(err, down) {
		t.Fatalf("ok=%v err=%v, want the read error", ok, err)
	}
}

func TestAConversationThatCannotHaveAutomationHasNoResponsibleWorkflow(t *testing.T) {
	for _, missing := range []error{conversation.ErrEntryTypeInvalid, conversation.ErrConversationNotFound} {
		wrapped := fmt.Errorf("entry info: %w", missing)
		id, ok, err := responsibilityGate(ownedBy(""), gateProfiles{err: wrapped}).ResponsibleWorkflow("ws-1", "e-1", "lead")
		if err != nil || ok || id != "" {
			t.Fatalf("%v: id=%q ok=%v err=%v, want no responsible workflow and nothing to retry", missing, id, ok, err)
		}
	}
}
