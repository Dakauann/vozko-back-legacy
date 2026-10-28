package workflow_usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"vozko/domain/shared/deps"
	"vozko/domain/workflow"
)

type unreachableModels struct{}

func (unreachableModels) IsValidModel(context.Context, string) (bool, error) {
	return false, errors.New("model catalog unreachable")
}

func TestTheSharedLinterRefusesEveryMissingDependency(t *testing.T) {
	uc, err := NewActivateWorkflowUseCase(ActivateWorkflowDeps{})
	var missing *deps.MissingError
	if uc != nil || !errors.As(err, &missing) || len(missing.Missing) != 24 {
		t.Fatalf("uc = %v err = %v, want every dependency refused", uc, err)
	}
}

func TestAnAgentStepIsBlockedWhenItsModelCannotBeChecked(t *testing.T) {
	catalog := productionCatalog(t)
	uc := &activateWorkflowUseCase{repo: NewMockWorkflowRepository(), catalogFn: func() []workflow.NodeDefinition { return catalog }, modelLookup: unreachableModels{}}
	draft := &workflow.Workflow{WorkspaceID: "ws-1", Type: workflow.WorkflowTypeMessages, Graph: workflow.Graph{Nodes: []workflow.Node{{ID: "agent", Type: workflow.NodeTypeActionAIAgent, Config: map[string]interface{}{"source": "prompt", "model": "openai/gpt-5"}}}}}
	report, err := uc.Lint(context.Background(), draft)
	if err != nil || !reflect.DeepEqual(issueCodes(report.Issues)[workflow.LintValidationUnavailable], []string{"agent"}) {
		t.Fatalf("an unverifiable model passed: %v %+v", err, report.Issues)
	}
}
