package workflow_usecase

import (
	"strings"
	"testing"

	"vozko/domain/workflow"
)

func builderStateOver(t *testing.T) (*aiBuilderUC, *builderState) {
	t.Helper()
	uc := &aiBuilderUC{}
	st, err := uc.initState("", "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	st.wfType = workflow.WorkflowTypeMessages
	st.fullCatalog = productionCatalog(t)
	return uc, st
}

func catalogLineOf(lines []string, nodeType workflow.NodeType) string {
	for _, line := range lines {
		if strings.HasPrefix(line, "• "+string(nodeType)+" |") {
			return line
		}
	}
	return ""
}

func TestTheBuilderCatalogStatesEachTriggersScope(t *testing.T) {
	uc, st := builderStateOver(t)
	lines := uc.catalogLines(st)
	for _, nodeType := range []workflow.NodeType{workflow.NodeTypeTriggerStageAdded, workflow.NodeTypeTriggerMessageReceived, workflow.NodeTypeTriggerFirstMessage} {
		if line := catalogLineOf(lines, nodeType); !strings.Contains(line, "escopo: "+workflow.TriggerScopeConversation.Guide()) {
			t.Errorf("%s line does not say it is scoped to the conversation:\n%s", nodeType, line)
		}
	}
	for _, nodeType := range []workflow.NodeType{workflow.NodeTypeTriggerWebhook, workflow.NodeTypeTriggerFormSubmitted, workflow.NodeTypeTriggerAgentCall, workflow.NodeTypeTriggerBirthday} {
		if line := catalogLineOf(lines, nodeType); !strings.Contains(line, "escopo: "+workflow.TriggerScopeGlobal.Guide()) {
			t.Errorf("%s line does not say it is global:\n%s", nodeType, line)
		}
	}
	if line := catalogLineOf(lines, workflow.NodeTypeEnd); strings.Contains(line, "escopo:") {
		t.Errorf("a non-trigger node got a scope line:\n%s", line)
	}
}

func TestTheBuilderPromptExplainsWhereConversationTriggersComeFromAndNoLongerHidesToolMode(t *testing.T) {
	uc, st := builderStateOver(t)
	prompt := uc.systemPrompt(st)
	if !strings.Contains(prompt, "ESCOPO DOS GATILHOS") {
		t.Fatal("the prompt does not explain trigger scope")
	}
	if strings.Contains(prompt, "OCULTO") {
		t.Fatal("tool_mode is a declared field now, the prompt must not call it hidden")
	}
}

func TestTriggerScopeGuidesNeverUseDashes(t *testing.T) {
	for _, scope := range []workflow.TriggerScope{workflow.TriggerScopeConversation, workflow.TriggerScopeGlobal} {
		guide := scope.Guide()
		if guide == "" || strings.ContainsAny(guide, "\u2013\u2014") {
			t.Errorf("%s guide = %q", scope, guide)
		}
	}
	if workflow.TriggerScope("other").Guide() != "" {
		t.Error("an unknown scope has no guide")
	}
}
