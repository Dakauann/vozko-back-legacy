package workflow

import (
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestTheEntryPointsOfAPublishedWorkflowComeFromItsPublishedTriggers(t *testing.T) {
	published := entryWorkflow(WorkflowStatusActive, true)
	published.Graph = Graph{Nodes: []Node{triggerOf(NodeTypeTriggerMessageReceived, nil)}}
	draft := entryWorkflow(WorkflowStatusDraft, false)
	draft.ID = "wf-2"
	draft.Graph = Graph{Nodes: []Node{triggerOf(NodeTypeTriggerWebhook, nil), {ID: "end", Type: NodeTypeEnd}}}
	triggers := map[string][]Node{"wf-1": {triggerOf(NodeTypeTriggerAgentCall, nil)}}

	if got := RunningTriggers(published, triggers); len(got) != 1 || got[0].Type != NodeTypeTriggerAgentCall {
		t.Fatalf("published triggers = %+v, want the published agent trigger, not the draft", got)
	}
	if got := RunningTriggers(draft, triggers); len(got) != 1 || got[0].Type != NodeTypeTriggerWebhook {
		t.Fatalf("never published triggers = %+v, want the draft triggers", got)
	}
	unreadable := entryWorkflow(WorkflowStatusActive, true)
	unreadable.ID = "wf-3"
	unreadable.Graph = published.Graph
	if got := RunningTriggers(unreadable, triggers); len(got) != 0 {
		t.Fatalf("a published version that could not be read must not fall back to the draft: %+v", got)
	}
}

func TestTheSourceQueryAsksForEveryChosenFormOnce(t *testing.T) {
	a := entryWorkflow(WorkflowStatusActive, true)
	b := entryWorkflow(WorkflowStatusDraft, false)
	b.ID = "wf-2"
	b.Graph = Graph{Nodes: []Node{
		triggerOf(NodeTypeTriggerFormSubmitted, map[string]interface{}{FormTriggerFormIDsKey: []interface{}{"f-2", "f-1"}}),
	}}
	triggers := map[string][]Node{"wf-1": {triggerOf(NodeTypeTriggerFormSubmitted, map[string]interface{}{FormTriggerFormIDsKey: []interface{}{"f-1"}})}}

	q := EntryPointQueryOf("ws-1", []*Workflow{a, b, nil}, triggers)
	sort.Strings(q.FormIDs)
	if q.WorkspaceID != "ws-1" || !reflect.DeepEqual(q.WorkflowIDs, []string{"wf-1", "wf-2"}) || !reflect.DeepEqual(q.FormIDs, []string{"f-1", "f-2"}) {
		t.Fatalf("query = %+v", q)
	}
}

func TestTheFactsOfOneWorkflowSplitTheWorkspaceChannels(t *testing.T) {
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	sources := &EntryPointSources{
		Webhooks: map[string]WebhookEntry{"wf-1": {Active: true}},
		Agents:   map[string][]EntryAgent{"wf-1": {{ID: "a-1", Active: true}}, "wf-2": {{ID: "a-2"}}},
		LinkedChannels: []WorkspaceChannel{
			{EntryChannel: EntryChannel{Kind: ChannelFacebookPage, ID: "p-1", WorkflowEnabled: true}, WorkflowID: "wf-1"},
			{EntryChannel: EntryChannel{Kind: ChannelWhatsAppCampaign, ID: "c-1"}, WorkflowID: "wf-2"},
		},
		LastStarts:     map[string]map[EntryPointKind]time.Time{"wf-1": {EntryLeadExit: at}},
		LastDeliveries: map[string]EntryDelivery{"wf-1": {At: at, Outcome: WebhookOutcomeAccepted}},
	}
	facts := sources.FactsOf("wf-1")
	if facts.Webhook == nil || !facts.Webhook.Active || len(facts.Agents) != 1 || facts.LastDelivery == nil || !facts.LastStarts[EntryLeadExit].Equal(at) {
		t.Fatalf("facts = %+v", facts)
	}
	if len(facts.Channels) != 1 || facts.Channels[0].ID != "p-1" {
		t.Fatalf("bound channels = %+v, want only the ones bound to wf-1", facts.Channels)
	}
	if other := sources.FactsOf("wf-3"); other.Webhook != nil || len(other.Channels) != 0 || other.LastDelivery != nil {
		t.Fatalf("a workflow with nothing linked = %+v", other)
	}
}

func TestTheSourceQueryAsksForTheChosenStagesAndLinesOnce(t *testing.T) {
	triggers := map[string][]Node{
		"wf-1": {
			triggerOf(NodeTypeTriggerStageAdded, map[string]interface{}{DataKeyStageID: "s-1"}),
			triggerOf(NodeTypeTriggerCallReceived, map[string]interface{}{TrunkIDKey: " tr-1 "}),
		},
		"wf-2": {
			triggerOf(NodeTypeTriggerStageAdded, map[string]interface{}{DataKeyStageID: "s-1"}),
			triggerOf(NodeTypeTriggerStageAdded, map[string]interface{}{DataKeyStageID: ""}),
		},
	}
	q := EntryPointQueryFor("ws-1", triggers)
	sort.Strings(q.WorkflowIDs)
	if !reflect.DeepEqual(q.WorkflowIDs, []string{"wf-1", "wf-2"}) || !reflect.DeepEqual(q.StageIDs, []string{"s-1"}) || !reflect.DeepEqual(q.TrunkIDs, []string{"tr-1"}) {
		t.Fatalf("query = %+v", q)
	}
}

func TestTheFactsOfOneWorkflowCarryTheWorkspaceStageAndLineNames(t *testing.T) {
	sources := &EntryPointSources{
		Stages: map[string]string{"s-1": "Pago"},
		Trunks: map[string]EntryTrunk{"tr-1": {Name: "Central", Enabled: true}},
	}
	facts := sources.FactsOf("wf-1")
	if facts.Stages["s-1"] != "Pago" || facts.Trunks["tr-1"].Name != "Central" {
		t.Fatalf("facts = %+v", facts)
	}
}
