package tools

import (
	"errors"
	"fmt"
	"testing"
)

func TestTheZeroPolicyIsSingle(t *testing.T) {
	var d Definition
	if d.Instances.Effective() != InstancesSingle || d.Instances.AllowsMany() {
		t.Fatalf("policy %+v", d.Instances)
	}
}

func TestPerTargetReadsItsTargetFromTheConfig(t *testing.T) {
	d := Definition{Name: "run_workflow", Instances: PerTarget("workflow_id")}
	if got := d.TargetOf(map[string]interface{}{"workflow_id": " wf-a "}); got != "wf-a" {
		t.Fatalf("target %q", got)
	}
	if got := d.TargetOf(map[string]interface{}{}); got != "" {
		t.Fatalf("target %q", got)
	}
	if !d.Instances.AllowsMany() {
		t.Fatal("per target allows many instances")
	}
	if (Definition{Instances: Many()}).TargetOf(map[string]interface{}{"workflow_id": "x"}) != "" {
		t.Fatal("only per target types have a target")
	}
}

func TestLegacyNamesKeepTodaysModelFacingNames(t *testing.T) {
	bound := Definition{Name: "run_workflow", Instances: PerTarget("workflow_id")}
	if got := bound.LegacyName(map[string]interface{}{"workflow_id": "wf-a"}); got != "run_workflow_wf-a" {
		t.Fatalf("got %q", got)
	}
	if got := (Definition{Name: "http_request", Instances: Many()}).LegacyName(map[string]interface{}{"url": "x"}); got != "http_request" {
		t.Fatalf("got %q", got)
	}
	if got := bound.LegacyName(map[string]interface{}{}); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestABindingProblemIsReadBackThroughWrapping(t *testing.T) {
	err := fmt.Errorf("wrap: %w", Invalid("tool_sheet_invalid", "planilha"))
	problem, ok := ProblemOf(err)
	if !ok || problem.Code != "tool_sheet_invalid" {
		t.Fatalf("problem %+v ok %v", problem, ok)
	}
	if _, ok := ProblemOf(errors.New("db down")); ok {
		t.Fatal("an infrastructure error is not a binding problem")
	}
}
