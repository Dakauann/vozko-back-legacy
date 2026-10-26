package inbox_assignment

import (
	"errors"
	"fmt"
	"testing"

	"vozko/domain/conversation"
)

func delegatedProfile() conversation.AutomationProfile {
	return conversation.AutomationProfile{
		AgentID:               "agent-1",
		AgentResponsesEnabled: true,
		Delegate:              &conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "wf-1"},
	}
}

func TestAStartByAPersonGivesThemAnUnownedConversationAndStopsTheAutomation(t *testing.T) {
	got := StartOwnershipFor("ana", nil, conversation.AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true})

	want := StartOwnership{Owner: "ana", Takes: true, Pauses: true}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestAStartByAPersonTakesTheConversationFromTheNumbersAutomation(t *testing.T) {
	for _, holder := range []string{"ai:agent-1", "workflow:wf-1"} {
		got := StartOwnershipFor("ana", &InboxAssignment{AssignedUserID: holder}, conversation.AutomationProfile{WorkflowID: "wf-1", WorkflowEnabled: true})

		want := StartOwnership{Owner: "ana", Takes: true, Pauses: true}
		if got != want {
			t.Fatalf("held by %s: got %+v, want %+v", holder, got, want)
		}
	}
}

func TestAStartLeavesAColleaguesConversationWithThemAndTheirAutomationSwitchAlone(t *testing.T) {
	got := StartOwnershipFor("ana", &InboxAssignment{AssignedUserID: "bob"}, conversation.AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true})

	want := StartOwnership{Owner: "bob"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestAStartOnTheStartersOwnConversationChangesNothing(t *testing.T) {
	got := StartOwnershipFor("ana", &InboxAssignment{AssignedUserID: "ana"}, conversation.AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true})

	want := StartOwnership{Owner: "ana"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestAStartKeepsAnExplicitDelegation(t *testing.T) {
	for _, holder := range []*InboxAssignment{nil, {AssignedUserID: "workflow:wf-1"}} {
		got := StartOwnershipFor("ana", holder, delegatedProfile())

		want := StartOwnership{Owner: holder.Owner()}
		if got != want {
			t.Fatalf("held by %q: got %+v, want %+v", holder.Owner(), got, want)
		}
	}
}

func TestAPausedDelegationDoesNotKeepTheConversationFromTheStarter(t *testing.T) {
	paused := false
	profile := delegatedProfile()
	profile.AutomationEnabled = &paused

	got := StartOwnershipFor("ana", nil, profile)

	want := StartOwnership{Owner: "ana", Takes: true}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestAnAlreadyPausedConversationNeedsNoPause(t *testing.T) {
	paused := false

	got := StartOwnershipFor("ana", &InboxAssignment{AssignedUserID: "ai:agent-1"}, conversation.AutomationProfile{AutomationEnabled: &paused})

	if !got.Takes || got.Pauses {
		t.Fatalf("got %+v, the automation is already off", got)
	}
}

func TestOnlyAPersonStartsAConversation(t *testing.T) {
	cases := map[string]bool{
		"ana":           true,
		"":              false,
		"  ":            false,
		"system":        false,
		"ai:agent-1":    false,
		"workflow:wf-1": false,
		"campaign:c-1":  false,
	}
	for starter, want := range cases {
		if got := (ConversationStart{StarterID: starter}).ByPerson(); got != want {
			t.Fatalf("ByPerson(%q) = %v, want %v", starter, got, want)
		}
	}
}

func TestAFailedStartClaimIsNamedByItsCode(t *testing.T) {
	if !errors.Is(StarterNotAssigned(errors.New("db down")), ErrStarterNotAssigned) {
		t.Fatal("the wrapped failure must still read as ErrStarterNotAssigned")
	}
	if got := StarterNotAssigned(fmt.Errorf("pause: %w", errors.New("db down"))).Error(); got == ErrStarterNotAssigned.Error() {
		t.Fatalf("the cause must stay in the message, got %q", got)
	}
	if CodeStarterNotAssigned != "starter_not_assigned" {
		t.Fatalf("code = %q, the front translates starter_not_assigned", CodeStarterNotAssigned)
	}
}
