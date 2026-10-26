package conversation_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/conversation"
	ia "vozko/domain/inbox_assignment"
	"vozko/domain/shared"
)

type gateOwners struct {
	owner string
	err   error
	asked []string
}

func (g *gateOwners) FindByEntry(workspaceID, entryID, entryType string) (*ia.InboxAssignment, error) {
	g.asked = append(g.asked, workspaceID+"|"+entryID+"|"+entryType)
	if g.err != nil {
		return nil, g.err
	}
	if g.owner == "" {
		return nil, nil
	}
	return &ia.InboxAssignment{WorkspaceID: workspaceID, EntryID: entryID, EntryType: entryType, AssignedUserID: g.owner}, nil
}

type gateProfiles struct {
	profile conversation.AutomationProfile
	err     error
}

func (g gateProfiles) EntryAutomation(string, string) (conversation.AutomationProfile, error) {
	return g.profile, g.err
}

func ownedBy(owner string) *gateOwners { return &gateOwners{owner: owner} }

func gateWith(owners *gateOwners, delegations conversation.DelegationRepository) *AutomationGate {
	return NewAutomationGate(AutomationGateDeps{Owners: owners, Delegations: delegations, Profiles: gateProfiles{}})
}

func openGate() *AutomationGate { return gateWith(ownedBy(""), stubDelegations{}) }

func admission(enabled *bool) AutomationAdmission {
	return AutomationAdmission{
		WorkspaceID: "ws-1",
		EntryID:     "e-1",
		EntryType:   shared.EntryTypeTelegram,
		Enabled:     enabled,
		Channel:     fullyEnabled(),
	}
}

func TestTheGateLetsAutomationActOnlyWhenNoPersonOwnsTheConversation(t *testing.T) {
	cases := []struct {
		name  string
		owner string
		want  bool
	}{
		{name: "nobody owns it", want: true},
		{name: "the agent owns it", owner: "ai:agent-1", want: true},
		{name: "a workflow owns it", owner: "workflow:wf-1", want: true},
		{name: "a person owns it", owner: "user-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owners := ownedBy(tc.owner)
			config, mayAct := gateWith(owners, stubDelegations{}).Admit(context.Background(), admission(nil))

			if mayAct != tc.want {
				t.Fatalf("mayAct = %v, want %v", mayAct, tc.want)
			}
			if *config.AgentID != "agent-1" || config.WorkflowRef() != "wf-1" {
				t.Fatalf("config = %+v, the channel automation must come back unchanged", config)
			}
			if len(owners.asked) != 1 || owners.asked[0] != "ws-1|e-1|telegram" {
				t.Fatalf("owner reads = %v", owners.asked)
			}
		})
	}
}

func TestTheGateLetsADelegatedAutomationActOverAPersonsOwnership(t *testing.T) {
	owners := ownedBy("user-1")
	config, mayAct := gateWith(owners, delegatedTo(conversation.AutomationAgent, "picked")).Admit(context.Background(), admission(nil))

	if !mayAct || *config.AgentID != "picked" || config.RunsWorkflows() {
		t.Fatalf("mayAct = %v, config = %+v", mayAct, config)
	}
	if len(owners.asked) != 0 {
		t.Fatalf("a delegation decides alone, the owner was read: %v", owners.asked)
	}
}

func TestTheGateKeepsAPausedConversationQuietWithoutReadingItsOwner(t *testing.T) {
	off := false
	for name, delegations := range map[string]stubDelegations{
		"undelegated": {},
		"delegated":   delegatedTo(conversation.AutomationWorkflow, "picked-wf"),
	} {
		t.Run(name, func(t *testing.T) {
			owners := ownedBy("")
			if _, mayAct := gateWith(owners, delegations).Admit(context.Background(), admission(&off)); mayAct {
				t.Fatal("a paused conversation let automation act")
			}
			if len(owners.asked) != 0 {
				t.Fatalf("owner reads = %v", owners.asked)
			}
		})
	}
}

func TestTheGateFailsClosed(t *testing.T) {
	cases := []struct {
		name        string
		owners      *gateOwners
		delegations stubDelegations
		workspace   string
	}{
		{name: "the owner is unreadable", owners: &gateOwners{err: errors.New("db down")}, workspace: "ws-1"},
		{name: "the delegation is unreadable", owners: ownedBy(""), delegations: stubDelegations{err: errors.New("db down")}, workspace: "ws-1"},
		{name: "the workspace is unknown", owners: ownedBy(""), workspace: "  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := admission(nil)
			in.WorkspaceID = tc.workspace
			if _, mayAct := gateWith(tc.owners, tc.delegations).Admit(context.Background(), in); mayAct {
				t.Fatal("automation acted on a conversation whose standing could not be read")
			}
		})
	}
}

func TestTheGateDecidesAStoredConversationFromItsProfileAndOwner(t *testing.T) {
	off := false
	cases := []struct {
		name    string
		profile conversation.AutomationProfile
		owner   string
		want    bool
	}{
		{name: "unowned", profile: conversation.AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true}, want: true},
		{name: "owned by the agent", profile: conversation.AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true}, owner: "ai:agent-1", want: true},
		{name: "owned by a person", profile: conversation.AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true}, owner: "user-1"},
		{name: "delegated by a person", profile: conversation.AutomationProfile{Delegate: &conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "wf-1"}}, owner: "user-1", want: true},
		{name: "paused", profile: conversation.AutomationProfile{AutomationEnabled: &off}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gate := NewAutomationGate(AutomationGateDeps{Owners: ownedBy(tc.owner), Delegations: stubDelegations{}, Profiles: gateProfiles{profile: tc.profile}})

			mayAct, err := gate.MayAct("ws-1", "e-1", shared.EntryTypeWhatsApp)

			if err != nil || mayAct != tc.want {
				t.Fatalf("MayAct = %v, %v, want %v", mayAct, err, tc.want)
			}
		})
	}
}

func TestTheGateReportsAStoredConversationItCannotRead(t *testing.T) {
	cases := map[string]AutomationGateDeps{
		"profile": {Owners: ownedBy(""), Delegations: stubDelegations{}, Profiles: gateProfiles{err: errors.New("db down")}},
		"owner":   {Owners: &gateOwners{err: errors.New("db down")}, Delegations: stubDelegations{}, Profiles: gateProfiles{}},
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			mayAct, err := NewAutomationGate(d).MayAct("ws-1", "e-1", shared.EntryTypeWhatsApp)
			if err == nil || mayAct {
				t.Fatalf("MayAct = %v, %v, want an error and no automation", mayAct, err)
			}
		})
	}
}

func TestTheGateNamesTheGoverningAutomationOnlyWhenItMayAct(t *testing.T) {
	agent := conversation.AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true}

	free := NewAutomationGate(AutomationGateDeps{Owners: ownedBy("ai:agent-1"), Delegations: stubDelegations{}, Profiles: gateProfiles{profile: agent}})
	governing, governed, err := free.Governing("ws-1", "e-1", shared.EntryTypeInstagram)
	if err != nil || !governed || governing != (conversation.Automation{Kind: conversation.AutomationAgent, ID: "agent-1"}) {
		t.Fatalf("Governing = %+v, %v, %v", governing, governed, err)
	}

	taken := NewAutomationGate(AutomationGateDeps{Owners: ownedBy("user-1"), Delegations: stubDelegations{}, Profiles: gateProfiles{profile: agent}})
	if _, governed, err := taken.Governing("ws-1", "e-1", shared.EntryTypeInstagram); err != nil || governed {
		t.Fatalf("a person's conversation is governed = %v, %v", governed, err)
	}
}

func TestTheGateRefusesToBuildWithoutItsReaders(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a gate without readers would let automation act blind")
		}
	}()
	NewAutomationGate(AutomationGateDeps{})
}
