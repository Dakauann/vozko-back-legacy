package workflow_usecase

import (
	"strings"
	"testing"

	"vozko/domain/workflow"
)

func onInstagram(run *workflow.WorkflowRun) { run.EntryType = "instagram" }

func TestATemplateStepRunOnInstagramFailsTheRunWithACodeInsteadOfSkipping(t *testing.T) {
	h := newTraceHarness()
	sent := false
	h.register(workflow.NodeTypeActionSendTemplate, func(*workflow.NodeContext) (*workflow.NodeResult, error) {
		sent = true
		return &workflow.NodeResult{Skipped: true}, nil
	})
	run := h.startWith(linearWorkflow(workflow.Node{ID: "n1", Type: workflow.NodeTypeActionSendTemplate, Config: map[string]interface{}{"template_id": "tpl"}}), onInstagram)
	if sent {
		t.Fatal("the template executor must not run on a channel that never sends templates")
	}
	if run.Status != workflow.RunStatusError {
		t.Fatalf("status = %s", run.Status)
	}
	step := h.step(t, "n1")
	if step.Status != workflow.LogStatusFailed || !strings.Contains(step.Error, workflow.CodeChannelUnsupported) {
		t.Fatalf("step = %+v", step)
	}
}

func TestAFormStepRunOnInstagramTakesItsWiredFailurePath(t *testing.T) {
	h := newTraceHarness()
	h.register(workflow.NodeTypeActionSendForm, func(*workflow.NodeContext) (*workflow.NodeResult, error) {
		t.Fatal("the form executor must not run on a channel without forms")
		return nil, nil
	})
	reached := false
	h.register(workflow.NodeTypeActionSendText, func(*workflow.NodeContext) (*workflow.NodeResult, error) {
		reached = true
		return &workflow.NodeResult{}, nil
	})
	w := &workflow.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Type: workflow.WorkflowTypeMessages, Graph: workflow.Graph{
		Nodes: []workflow.Node{
			{ID: "t1", Type: workflow.NodeTypeTriggerWebhook},
			{ID: "n1", Type: workflow.NodeTypeActionSendForm, Config: map[string]interface{}{workflow.FormNodeFormIDKey: "f1"}},
			{ID: "n2", Type: workflow.NodeTypeActionSendText},
			{ID: "end", Type: workflow.NodeTypeEnd},
		},
		Edges: []workflow.Edge{
			{Source: "t1", Target: "n1"},
			{Source: "n1", Target: "n2", Label: workflow.FormHandleSendFailed},
			{Source: "n2", Target: "end"},
		},
	}}
	run := h.startWith(w, onInstagram)
	if !reached || run.Status != workflow.RunStatusCompleted {
		t.Fatalf("status = %s, reached = %v", run.Status, reached)
	}
	if step := h.step(t, "n1"); step.Status != workflow.LogStatusFailed {
		t.Fatalf("step = %+v", step)
	}
}
