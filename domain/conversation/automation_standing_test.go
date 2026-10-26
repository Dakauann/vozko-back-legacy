package conversation

import "testing"

func TestWhetherAutomationMayActOnAConversation(t *testing.T) {
	cases := []struct {
		name     string
		standing AutomationStanding
		want     bool
	}{
		{name: "nobody owns it", standing: AutomationStanding{}, want: true},
		{name: "nobody owns it and automation is explicitly on", standing: AutomationStanding{Enabled: boolPtr(true)}, want: true},
		{name: "the agent owns it", standing: AutomationStanding{Owner: "ai:agent-1"}, want: true},
		{name: "a workflow owns it", standing: AutomationStanding{Owner: "workflow:wf-1"}, want: true},
		{name: "a person owns it", standing: AutomationStanding{Owner: "user-1"}, want: false},
		{name: "a person owns it with automation explicitly on", standing: AutomationStanding{Owner: "user-1", Enabled: boolPtr(true)}, want: false},
		{name: "a person delegated it to an automation", standing: AutomationStanding{Owner: "user-1", Delegated: true}, want: true},
		{name: "paused with nobody owning it", standing: AutomationStanding{Enabled: boolPtr(false)}, want: false},
		{name: "paused while the agent owns it", standing: AutomationStanding{Owner: "ai:agent-1", Enabled: boolPtr(false)}, want: false},
		{name: "a paused delegation", standing: AutomationStanding{Delegated: true, Enabled: boolPtr(false)}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.standing.MayAct(); got != tc.want {
				t.Fatalf("MayAct = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTheOwnerOnlyMattersWhenAutomationIsOnAndNotDelegated(t *testing.T) {
	cases := []struct {
		name     string
		standing AutomationStanding
		want     bool
	}{
		{name: "on and not delegated", standing: AutomationStanding{}, want: true},
		{name: "paused", standing: AutomationStanding{Enabled: boolPtr(false)}},
		{name: "delegated", standing: AutomationStanding{Delegated: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.standing.OwnerDecides(); got != tc.want {
				t.Fatalf("OwnerDecides = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAProfileStandsDelegatedOnlyWithAUsableDelegate(t *testing.T) {
	off := boolPtr(false)
	cases := []struct {
		name    string
		profile AutomationProfile
		want    AutomationStanding
	}{
		{name: "no delegate", profile: AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true}, want: AutomationStanding{}},
		{name: "a delegate", profile: AutomationProfile{Delegate: &Automation{Kind: AutomationWorkflow, ID: "wf-1"}}, want: AutomationStanding{Delegated: true}},
		{name: "a blank delegate", profile: AutomationProfile{Delegate: &Automation{Kind: AutomationAgent}}, want: AutomationStanding{}},
		{name: "paused", profile: AutomationProfile{AutomationEnabled: off}, want: AutomationStanding{Enabled: off}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.profile.Standing(); got != tc.want {
				t.Fatalf("Standing = %+v, want %+v", got, tc.want)
			}
		})
	}
}
