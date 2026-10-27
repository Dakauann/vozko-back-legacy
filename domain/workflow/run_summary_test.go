package workflow

import (
	"reflect"
	"testing"
)

func onlyCondition(t *testing.T, conditions []RunCondition, kind EntryPointKind) RunCondition {
	t.Helper()
	var found []RunCondition
	for _, c := range conditions {
		if c.Kind == kind {
			found = append(found, c)
		}
	}
	if len(found) != 1 {
		t.Fatalf("conditions of kind %s = %+v, want exactly one", kind, conditions)
	}
	return found[0]
}

func TestTheRunStateFollowsPublicationAndStatus(t *testing.T) {
	cases := map[RunningState]*Workflow{
		RunningDraft:  entryWorkflow(WorkflowStatusDraft, false),
		RunningPaused: entryWorkflow(WorkflowStatusPaused, true),
		RunningActive: entryWorkflow(WorkflowStatusActive, true),
	}
	for want, w := range cases {
		if got := RunningStateOf(w); got != want {
			t.Errorf("RunningStateOf(%s) = %s, want %s", w.Status, got, want)
		}
	}
	if got := RunningStateOf(entryWorkflow(WorkflowStatusArchived, true)); got != RunningPaused {
		t.Errorf("an archived workflow does not run: %s", got)
	}
}

func TestConversationConditionsNameTheLinkedChannels(t *testing.T) {
	facts := EntryPointFacts{Channels: []EntryChannel{
		{Kind: ChannelUnofficialWhatsAppInstance, ID: "i-1", Name: "+55 11 90000-0001", WorkflowEnabled: true},
		{Kind: ChannelUnofficialWhatsAppInstance, ID: "i-1", Name: "+55 11 90000-0001", WorkflowEnabled: true},
		{Kind: ChannelWhatsAppCampaign, ID: "c-1", Name: "Black Friday"},
	}}
	conditions := RunConditionsOf([]Node{triggerOf(NodeTypeTriggerMessageReceived, nil)}, facts)

	got := onlyCondition(t, conditions, EntryMessageReceived)
	want := RunCondition{
		Kind: EntryMessageReceived, Scope: TriggerScopeConversation, NodeID: "t-" + string(NodeTypeTriggerMessageReceived),
		Targets: []RunTarget{
			{Kind: RunTargetKind(ChannelUnofficialWhatsAppInstance), ID: "i-1", Name: "+55 11 90000-0001"},
			{Kind: RunTargetKind(ChannelWhatsAppCampaign), ID: "c-1", Name: "Black Friday", Off: true},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("condition = %+v\nwant %+v", got, want)
	}
}

func TestAConversationConditionWithNoLinkSaysSo(t *testing.T) {
	for _, nodeType := range []NodeType{NodeTypeTriggerMessageReceived, NodeTypeTriggerFirstMessage, NodeTypeTriggerStageAdded} {
		c := RunConditionsOf([]Node{triggerOf(nodeType, nil)}, EntryPointFacts{})[0]
		if c.Reason != EntryBlockedNoChannel || c.Scope != TriggerScopeConversation || len(c.Targets) != 0 {
			t.Errorf("%s with no link = %+v", nodeType, c)
		}
	}
}

func TestAConversationConditionCountsTheChannelsItDoesNotList(t *testing.T) {
	var channels []EntryChannel
	for i := 0; i < MaxListedChannels+3; i++ {
		channels = append(channels, EntryChannel{Kind: ChannelFacebookPage, ID: string(rune('a' + i)), WorkflowEnabled: true})
	}
	c := RunConditionsOf([]Node{triggerOf(NodeTypeTriggerFirstMessage, nil)}, EntryPointFacts{Channels: channels})[0]
	if len(c.Targets) != MaxListedChannels || c.MoreTargets != 3 {
		t.Fatalf("listed %d, more %d; want %d and 3", len(c.Targets), c.MoreTargets, MaxListedChannels)
	}
}

func TestTheStageConditionNamesTheStageOrAnyStage(t *testing.T) {
	facts := EntryPointFacts{Stages: map[string]string{"s-1": "Pagamento recebido"}}
	named := RunConditionsOf([]Node{triggerOf(NodeTypeTriggerStageAdded, map[string]interface{}{DataKeyStageID: "s-1"})}, facts)[0]
	if !reflect.DeepEqual(named.Stage, &RunTarget{Kind: RunTargetStage, ID: "s-1", Name: "Pagamento recebido"}) {
		t.Fatalf("stage = %+v", named.Stage)
	}
	gone := RunConditionsOf([]Node{triggerOf(NodeTypeTriggerStageAdded, map[string]interface{}{DataKeyStageID: "s-9"})}, facts)[0]
	if !reflect.DeepEqual(gone.Stage, &RunTarget{Kind: RunTargetStage, ID: "s-9", Missing: true}) {
		t.Fatalf("a deleted stage = %+v", gone.Stage)
	}
	anyStage := RunConditionsOf([]Node{triggerOf(NodeTypeTriggerStageAdded, nil)}, facts)[0]
	if anyStage.Stage != nil {
		t.Fatalf("no stage means any stage: %+v", anyStage.Stage)
	}
}

func TestGlobalConditionsCarryTheirOwnTargets(t *testing.T) {
	facts := EntryPointFacts{
		Webhook: activeWebhook(),
		Forms:   map[string]EntryForm{"f-1": {ID: "f-1", Name: "Cadastro", Active: true}},
		Agents:  []EntryAgent{{ID: "a-1", Name: "Bia", Active: true}},
		Trunks:  map[string]EntryTrunk{"tr-1": {Name: "Linha central", Enabled: true}},
	}
	conditions := RunConditionsOf([]Node{
		triggerOf(NodeTypeTriggerWebhook, nil),
		triggerOf(NodeTypeTriggerFormSubmitted, map[string]interface{}{FormTriggerFormIDsKey: []interface{}{"f-1"}}),
		triggerOf(NodeTypeTriggerAgentCall, nil),
		triggerOf(NodeTypeTriggerBirthday, map[string]interface{}{BirthdayHourKey: 8.0}),
		triggerOf(NodeTypeTriggerCallReceived, map[string]interface{}{"trunk_id": "tr-1"}),
	}, facts)

	if c := onlyCondition(t, conditions, EntryWebhook); c.Scope != TriggerScopeGlobal || c.Reason != "" {
		t.Errorf("webhook = %+v", c)
	}
	if c := onlyCondition(t, conditions, EntryFormSubmitted); !reflect.DeepEqual(c.Targets, []RunTarget{{Kind: RunTargetForm, ID: "f-1", Name: "Cadastro"}}) {
		t.Errorf("form = %+v", c)
	}
	if c := onlyCondition(t, conditions, EntryAgentCall); !reflect.DeepEqual(c.Targets, []RunTarget{{Kind: RunTargetAgent, ID: "a-1", Name: "Bia"}}) {
		t.Errorf("agent call = %+v", c)
	}
	if c := onlyCondition(t, conditions, EntryBirthday); c.Hour == nil || *c.Hour != 8 || c.Scope != TriggerScopeGlobal {
		t.Errorf("birthday = %+v", c)
	}
	if c := onlyCondition(t, conditions, EntryCallReceived); !reflect.DeepEqual(c.Targets, []RunTarget{{Kind: RunTargetSIPTrunk, ID: "tr-1", Name: "Linha central"}}) || c.Reason != "" {
		t.Errorf("call = %+v", c)
	}
}

func TestAFormConditionWithNoFormRunsForNoForm(t *testing.T) {
	for name, config := range map[string]map[string]interface{}{
		"no list":    nil,
		"empty list": {FormTriggerFormIDsKey: []interface{}{}},
	} {
		c := RunConditionsOf([]Node{triggerOf(NodeTypeTriggerFormSubmitted, config)}, EntryPointFacts{})[0]
		if c.Reason != EntryBlockedNoForm || len(c.Targets) != 0 {
			t.Errorf("%s = %+v", name, c)
		}
	}
}

func TestACallConditionWithoutALineDoesNotRun(t *testing.T) {
	noLine := RunConditionsOf([]Node{triggerOf(NodeTypeTriggerCallReceived, map[string]interface{}{"trunk_id": ""})}, EntryPointFacts{})[0]
	if noLine.Reason != EntryBlockedNoTrunk || len(noLine.Targets) != 0 {
		t.Fatalf("no line = %+v", noLine)
	}
	gone := RunConditionsOf([]Node{triggerOf(NodeTypeTriggerCallReceived, map[string]interface{}{"trunk_id": "tr-9"})}, EntryPointFacts{})[0]
	if gone.Reason != EntryBlockedNoTrunk || !reflect.DeepEqual(gone.Targets, []RunTarget{{Kind: RunTargetSIPTrunk, ID: "tr-9", Missing: true}}) {
		t.Fatalf("a deleted line = %+v", gone)
	}
	off := RunConditionsOf([]Node{triggerOf(NodeTypeTriggerCallReceived, map[string]interface{}{"trunk_id": "tr-1"})}, EntryPointFacts{Trunks: map[string]EntryTrunk{"tr-1": {Name: "Central"}}})[0]
	if !reflect.DeepEqual(off.Targets, []RunTarget{{Kind: RunTargetSIPTrunk, ID: "tr-1", Name: "Central", Off: true}}) {
		t.Fatalf("a disabled line = %+v", off)
	}
}

func TestLeadExitsAreARunConditionToo(t *testing.T) {
	facts := EntryPointFacts{LeadExits: []LeadExitEntry{
		{Source: LeadExitFromCampaign, ID: "c-1", Name: "Black Friday", Button: "Quero"},
		{Source: LeadExitFromTemplate, Name: "boas_vindas", Language: "pt_BR", Button: "Falar"},
	}}
	c := onlyCondition(t, RunConditionsOf([]Node{triggerOf(NodeTypeTriggerMessageReceived, nil)}, facts), EntryLeadExit)
	want := []RunTarget{
		{Kind: RunTargetCampaignButton, ID: "c-1", Name: "Black Friday", Button: "Quero"},
		{Kind: RunTargetTemplateButton, Name: "boas_vindas", Language: "pt_BR", Button: "Falar"},
	}
	if !reflect.DeepEqual(c.Targets, want) || c.Reason != "" {
		t.Fatalf("lead exit = %+v", c)
	}
}

func TestTheConditionListIsNeverNull(t *testing.T) {
	if got := RunConditionsOf(nil, EntryPointFacts{}); got == nil {
		t.Fatal("no trigger must be an empty list, not null")
	}
}

func activeTrigger(id, name string, nodeType NodeType, config map[string]interface{}) ActiveTrigger {
	return ActiveTrigger{WorkflowID: id, WorkflowName: name, Node: Node{ID: "t", Type: nodeType, Config: config}}
}

func formIDs(ids ...interface{}) map[string]interface{} {
	return map[string]interface{}{FormTriggerFormIDsKey: ids}
}

func TestTwoActiveWorkflowsOnTheSameFormOverlap(t *testing.T) {
	active := []ActiveTrigger{
		activeTrigger("wf-1", "Eu mesmo", NodeTypeTriggerFormSubmitted, formIDs("f-1")),
		activeTrigger("wf-2", "Cadastro CRM", NodeTypeTriggerFormSubmitted, formIDs("f-1", "f-2")),
		activeTrigger("wf-3", "Outro form", NodeTypeTriggerFormSubmitted, formIDs("f-3")),
		activeTrigger("wf-4", "Sem form", NodeTypeTriggerFormSubmitted, nil),
		activeTrigger("wf-5", "Aniversário", NodeTypeTriggerBirthday, nil),
	}
	facts := EntryPointFacts{Forms: map[string]EntryForm{"f-1": {ID: "f-1", Name: "Cadastro", Active: true}}}
	overlaps := RunOverlapsOf("wf-1", []Node{triggerOf(NodeTypeTriggerFormSubmitted, formIDs("f-1"))}, active, facts)
	want := []RunOverlap{{
		Kind: EntryFormSubmitted, Target: &RunTarget{Kind: RunTargetForm, ID: "f-1", Name: "Cadastro"},
		Workflows: []WorkflowRef{{ID: "wf-2", Name: "Cadastro CRM"}},
	}}
	if !reflect.DeepEqual(overlaps, want) {
		t.Fatalf("overlaps = %+v\nwant %+v", overlaps, want)
	}
}

func TestAFormTriggerWithoutFormsOverlapsWithNothing(t *testing.T) {
	active := []ActiveTrigger{activeTrigger("wf-2", "Cadastro", NodeTypeTriggerFormSubmitted, formIDs("f-1"))}
	if got := RunOverlapsOf("wf-1", []Node{triggerOf(NodeTypeTriggerFormSubmitted, nil)}, active, EntryPointFacts{}); len(got) != 0 {
		t.Fatalf("overlaps = %+v", got)
	}
}

func TestEveryOtherActiveBirthdayWorkflowOverlaps(t *testing.T) {
	active := []ActiveTrigger{
		activeTrigger("wf-1", "Parabéns", NodeTypeTriggerBirthday, nil),
		activeTrigger("wf-3", "Cupom", NodeTypeTriggerBirthday, map[string]interface{}{BirthdayHourKey: 14.0}),
		activeTrigger("wf-2", "Aniversário VIP", NodeTypeTriggerBirthday, nil),
	}
	overlaps := RunOverlapsOf("wf-1", []Node{triggerOf(NodeTypeTriggerBirthday, nil)}, active, EntryPointFacts{})
	want := []RunOverlap{{Kind: EntryBirthday, Workflows: []WorkflowRef{{ID: "wf-2", Name: "Aniversário VIP"}, {ID: "wf-3", Name: "Cupom"}}}}
	if !reflect.DeepEqual(overlaps, want) {
		t.Fatalf("overlaps = %+v\nwant %+v", overlaps, want)
	}
}

func TestOnlyTheSameLineOverlapsForCalls(t *testing.T) {
	active := []ActiveTrigger{
		activeTrigger("wf-2", "URA antiga", NodeTypeTriggerCallReceived, map[string]interface{}{"trunk_id": "tr-1"}),
		activeTrigger("wf-3", "Outra linha", NodeTypeTriggerCallReceived, map[string]interface{}{"trunk_id": "tr-2"}),
	}
	facts := EntryPointFacts{Trunks: map[string]EntryTrunk{"tr-1": {Name: "Central", Enabled: true}}}
	overlaps := RunOverlapsOf("wf-1", []Node{triggerOf(NodeTypeTriggerCallReceived, map[string]interface{}{"trunk_id": " tr-1 "})}, active, facts)
	want := []RunOverlap{{Kind: EntryCallReceived, Target: &RunTarget{Kind: RunTargetSIPTrunk, ID: "tr-1", Name: "Central"}, Workflows: []WorkflowRef{{ID: "wf-2", Name: "URA antiga"}}}}
	if !reflect.DeepEqual(overlaps, want) {
		t.Fatalf("overlaps = %+v\nwant %+v", overlaps, want)
	}
}

func TestNamedAndConversationTriggersNeverOverlap(t *testing.T) {
	active := []ActiveTrigger{
		activeTrigger("wf-2", "Outro", NodeTypeTriggerWebhook, nil),
		activeTrigger("wf-3", "Outro", NodeTypeTriggerMessageReceived, nil),
		activeTrigger("wf-4", "Outro", NodeTypeTriggerAgentCall, nil),
	}
	triggers := []Node{triggerOf(NodeTypeTriggerWebhook, nil), triggerOf(NodeTypeTriggerMessageReceived, nil), triggerOf(NodeTypeTriggerAgentCall, nil)}
	if got := RunOverlapsOf("wf-1", triggers, active, EntryPointFacts{}); len(got) != 0 {
		t.Fatalf("overlaps = %+v", got)
	}
}

func TestALegacyTriggerConfigCountsForTheFormOverlapLikeAtRunTime(t *testing.T) {
	legacy := ActiveTrigger{WorkflowID: "wf-2", WorkflowName: "Antigo", Node: Node{ID: "t", Type: NodeTypeTriggerFormSubmitted}, LegacyConfig: formIDs("f-1")}
	overlaps := RunOverlapsOf("wf-1", []Node{triggerOf(NodeTypeTriggerFormSubmitted, formIDs("f-1"))}, []ActiveTrigger{legacy}, EntryPointFacts{})
	if len(overlaps) != 1 || overlaps[0].Workflows[0].ID != "wf-2" {
		t.Fatalf("overlaps = %+v", overlaps)
	}
}

func TestTheSummaryWarnsOnlyWhileTheWorkflowRuns(t *testing.T) {
	active := []ActiveTrigger{activeTrigger("wf-2", "Cupom", NodeTypeTriggerBirthday, nil)}
	triggers := []Node{triggerOf(NodeTypeTriggerBirthday, nil)}
	running := RunSummaryOf(entryWorkflow(WorkflowStatusActive, true), triggers, EntryPointFacts{}, active)
	if running.State != RunningActive || len(running.Overlaps) != 1 || len(running.Conditions) != 1 {
		t.Fatalf("running = %+v", running)
	}
	paused := RunSummaryOf(entryWorkflow(WorkflowStatusPaused, true), triggers, EntryPointFacts{}, active)
	if paused.State != RunningPaused || len(paused.Overlaps) != 0 || len(paused.Conditions) != 1 {
		t.Fatalf("paused = %+v", paused)
	}
}

func TestThePublishPreviewReadsTheDraftAsIfItRan(t *testing.T) {
	w := entryWorkflow(WorkflowStatusDraft, false)
	w.Graph = Graph{Nodes: []Node{{ID: "b", Type: NodeTypeTriggerBirthday}, {ID: "end", Type: NodeTypeEnd}}}
	active := []ActiveTrigger{activeTrigger("wf-2", "Cupom", NodeTypeTriggerBirthday, nil), activeTrigger("wf-1", "Eu, publicado", NodeTypeTriggerBirthday, nil)}
	preview := PublishPreviewOf(w, EntryPointFacts{}, active)
	if preview.State != RunningActive || len(preview.Conditions) != 1 || preview.Conditions[0].NodeID != "b" {
		t.Fatalf("preview = %+v", preview)
	}
	if len(preview.Overlaps) != 1 || !reflect.DeepEqual(preview.Overlaps[0].Workflows, []WorkflowRef{{ID: "wf-2", Name: "Cupom"}}) {
		t.Fatalf("the preview warns about the others only: %+v", preview.Overlaps)
	}
}
