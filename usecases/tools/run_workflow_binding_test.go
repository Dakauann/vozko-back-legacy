package tools_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agenttool"
	"vozko/domain/tools"
)

func TestARunWorkflowBindingNeedsAPublishedWorkflowCallableByAgents(t *testing.T) {
	h := newRunWorkflowHarness()
	validator := h.tool.(tools.BindingValidator)
	if err := validator.ValidateBinding(context.Background(), "ws-1", map[string]interface{}{"workflow_id": "wf-a"}); err != nil {
		t.Fatalf("a callable workflow is valid: %v", err)
	}
	for _, id := range []string{"wf-missing", ""} {
		problem, ok := tools.ProblemOf(validator.ValidateBinding(context.Background(), "ws-1", map[string]interface{}{"workflow_id": id}))
		if !ok || problem.Code != agenttool.CodeWorkflowNotCallable {
			t.Fatalf("%q: problem %+v", id, problem)
		}
	}
	if problem, ok := tools.ProblemOf(validator.ValidateBinding(context.Background(), "ws-other", map[string]interface{}{"workflow_id": "wf-a"})); !ok || problem.Code != agenttool.CodeWorkflowNotCallable {
		t.Fatalf("another workspace: %+v", problem)
	}
	h.finder.err = errors.New("db down")
	err := validator.ValidateBinding(context.Background(), "ws-1", map[string]interface{}{"workflow_id": "wf-a"})
	if _, ok := tools.ProblemOf(err); ok || err == nil {
		t.Fatalf("an unreadable store is not a binding problem: %v", err)
	}
}
