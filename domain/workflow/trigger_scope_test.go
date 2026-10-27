package workflow

import "testing"

func TestEveryTriggerHasOneScope(t *testing.T) {
	want := map[TriggerType]TriggerScope{
		TriggerMessageReceived: TriggerScopeConversation,
		TriggerFirstMessage:    TriggerScopeConversation,
		TriggerStageAdded:      TriggerScopeConversation,
		TriggerWebhook:         TriggerScopeGlobal,
		TriggerFormSubmitted:   TriggerScopeGlobal,
		TriggerAgentCall:       TriggerScopeGlobal,
		TriggerBirthday:        TriggerScopeGlobal,
		TriggerCallReceived:    TriggerScopeGlobal,
	}
	for trigger, scope := range want {
		got, known := trigger.Scope()
		if !known || got != scope {
			t.Errorf("%s scope = %q (known %v), want %q", trigger, got, known, scope)
		}
	}
	for _, n := range triggerNodeTypes {
		if _, known := TriggerType(n).Scope(); !known {
			t.Errorf("trigger node %s has no scope, so it would never fire", n)
		}
		if _, listed := want[TriggerType(n)]; !listed {
			t.Errorf("trigger node %s is missing from this table", n)
		}
	}
}

func TestAnUnknownTriggerHasNoScope(t *testing.T) {
	for _, trigger := range []TriggerType{"", "trigger_lead_created", "action_send_text"} {
		if scope, known := trigger.Scope(); known || scope != "" {
			t.Errorf("%q scope = %q (known %v), want none so it fails closed", trigger, scope, known)
		}
	}
}

func TestOnlyConversationTriggersFollowTheResponsibleWorkflow(t *testing.T) {
	if !TriggerStageAdded.FollowsResponsibleWorkflow() || !TriggerMessageReceived.FollowsResponsibleWorkflow() || !TriggerFirstMessage.FollowsResponsibleWorkflow() {
		t.Fatal("conversation triggers must only start the workflow responsible for the conversation")
	}
	for _, global := range []TriggerType{TriggerWebhook, TriggerFormSubmitted, TriggerAgentCall, TriggerBirthday, TriggerCallReceived, "unknown"} {
		if global.FollowsResponsibleWorkflow() {
			t.Errorf("%s must not be narrowed to the responsible workflow", global)
		}
	}
}

func TestTriggerDefinitionsCarryTheirScope(t *testing.T) {
	catalog := NodeCatalogMap(BuiltinDefinitions())
	for _, n := range triggerNodeTypes {
		def, found := catalog[n]
		if !found {
			continue
		}
		want, _ := TriggerType(n).Scope()
		if def.TriggerScope != want {
			t.Errorf("%s definition scope = %q, want %q", n, def.TriggerScope, want)
		}
	}
	if def := catalog[NodeTypeEnd]; def.TriggerScope != "" {
		t.Errorf("a non-trigger node carries scope %q", def.TriggerScope)
	}
}

func TestATriggerDefinedOutsideTheBuiltinsStillGetsItsScope(t *testing.T) {
	def := NormalizeNodeDefinition(NodeDefinition{Type: NodeTypeTriggerStageAdded, Category: NodeCategoryTrigger, TriggerScope: TriggerScopeGlobal})
	if def.TriggerScope != TriggerScopeConversation {
		t.Fatalf("scope = %q, want the domain rule to win over a hand-written value", def.TriggerScope)
	}
}
