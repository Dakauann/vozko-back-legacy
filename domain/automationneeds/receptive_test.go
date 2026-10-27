package automationneeds

import (
	"errors"
	"reflect"
	"testing"

	"vozko/domain/conversation"
)

func ptr(s string) *string { return &s }

func TestRunningOfListsOnlyTheEnabledAutomations(t *testing.T) {
	cases := []struct {
		name   string
		config conversation.ChannelAutomation
		want   []conversation.Automation
	}{
		{"nothing chosen", conversation.ChannelAutomation{}, nil},
		{"an agent chosen but switched off", conversation.ChannelAutomation{AgentID: ptr("a1")}, nil},
		{"an enabled agent", conversation.ChannelAutomation{AgentID: ptr("a1"), EnableAgentResponses: true}, []conversation.Automation{agentRef("a1")}},
		{"an enabled workflow with a blank id", conversation.ChannelAutomation{WorkflowID: ptr(" "), EnableWorkflow: true}, nil},
		{"both enabled", conversation.ChannelAutomation{AgentID: ptr("a1"), EnableAgentResponses: true, WorkflowID: ptr("w1"), EnableWorkflow: true}, []conversation.Automation{agentRef("a1"), workflowRef("w1")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RunningOf(tc.config); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("running = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestImpactListsTheSlotsWhoseAutomationWouldNeedVariables(t *testing.T) {
	slots := []Slot{
		{Kind: SlotInstagram, ID: "ig", Name: "@loja", Running: []conversation.Automation{agentRef("escola")}},
		{Kind: SlotWhatsAppNumber, ID: "num", Name: "+55 11", Running: []conversation.Automation{workflowRef("livre")}},
		{Kind: SlotWebchat, ID: "wc", Name: "Site", Running: nil},
	}
	needs := map[conversation.Automation]Needs{
		agentRef("escola"):   {{Name: "turma", Via: []Link{{Automation: agentRef("escola"), Name: "Escola"}}}},
		workflowRef("livre"): nil,
	}
	got, err := Impact(slots, func(a conversation.Automation) (Needs, error) { return needs[a], nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Slot.ID != "ig" || got[0].Automation.Automation != agentRef("escola") || got[0].Automation.Name != "Escola" || got[0].Needs.Names()[0] != "turma" {
		t.Fatalf("impact = %+v", got)
	}
	if _, err := Impact(slots, func(conversation.Automation) (Needs, error) { return nil, ErrUnreadable }); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("an unreadable automation must fail the whole impact, got %v", err)
	}
}

func TestSlotKindsAreKnown(t *testing.T) {
	for _, kind := range SlotKinds() {
		if !kind.Valid() {
			t.Fatalf("%s should be valid", kind)
		}
	}
	if SlotKind("fax").Valid() {
		t.Fatal("an unknown kind is not valid")
	}
}

func TestEverySlotKindHasALabelAndAPath(t *testing.T) {
	for _, kind := range SlotKinds() {
		if SlotLabel(kind) == "" || SlotPath(kind, "x") == "" {
			t.Fatalf("%s has no label or path", kind)
		}
	}
	if AutomationPath(workflowRef("w1")) != "/dashboard/workflows/w1" || AutomationPath(agentRef("a1")) != "/dashboard/agents/a1" {
		t.Fatal("automation paths")
	}
}

func TestASwitchOffIsSettledByReEnablingOrChoosingAnotherAutomation(t *testing.T) {
	record := SwitchOffRecord{Automation: Link{Automation: agentRef("escola")}}
	cases := []struct {
		name   string
		config conversation.ChannelAutomation
		want   bool
	}{
		{"the same agent still off", conversation.ChannelAutomation{AgentID: ptr("escola")}, false},
		{"the same agent back on", conversation.ChannelAutomation{AgentID: ptr("escola"), EnableAgentResponses: true}, true},
		{"another agent chosen", conversation.ChannelAutomation{AgentID: ptr("outro")}, true},
		{"the agent cleared", conversation.ChannelAutomation{}, true},
	}
	for _, tc := range cases {
		if got := record.SettledBy(tc.config); got != tc.want {
			t.Fatalf("%s: settled = %v, want %v", tc.name, got, tc.want)
		}
	}
}
