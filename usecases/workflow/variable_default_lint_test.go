package workflow_usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"vozko/domain/automationneeds"
	"vozko/domain/workflow"
)

type defaultRules struct {
	rejected []string
	err      error
	asked    map[string]string
}

func (d *defaultRules) Rejected(_ context.Context, _ string, values map[string]string) ([]string, error) {
	d.asked = values
	return d.rejected, d.err
}

func (d *defaultRules) Label(_ context.Context, _ string, needs automationneeds.Needs) automationneeds.Needs {
	out := append(automationneeds.Needs(nil), needs...)
	for i := range out {
		out[i].Label = "Valor da parcela"
	}
	return out
}

func TestAFlowDefaultThatDoesNotFitItsTypeBlocksPublish(t *testing.T) {
	w := &workflow.Workflow{WorkspaceID: "ws", Graph: workflow.Graph{Variables: []workflow.VariableUse{{Name: "valor", Default: "dez"}, {Name: "turma"}}}}
	rules := &defaultRules{rejected: []string{"valor"}}
	issues, err := variableDefaultIssues(context.Background(), rules, w)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rules.asked, map[string]string{"valor": "dez"}) {
		t.Fatalf("asked = %v", rules.asked)
	}
	if len(issues) != 1 || issues[0].Code != workflow.LintGraphVariableDefaultInvalid || issues[0].Severity != workflow.SeverityBlocking || issues[0].Values["label"] != "Valor da parcela" || issues[0].Path != "valor" {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestFlowDefaultsFailClosedWhenTheTypesCannotBeRead(t *testing.T) {
	w := &workflow.Workflow{WorkspaceID: "ws", Graph: workflow.Graph{Variables: []workflow.VariableUse{{Name: "valor", Default: "1"}}}}
	if _, err := variableDefaultIssues(context.Background(), &defaultRules{err: errors.New("down")}, w); err == nil {
		t.Fatal("unreadable types passed")
	}
	if issues, err := variableDefaultIssues(context.Background(), &defaultRules{err: errors.New("down")}, &workflow.Workflow{}); err != nil || issues != nil {
		t.Fatalf("a flow without defaults reads nothing: %v %v", issues, err)
	}
}

func TestLintIssuesAboutConversationDataCarryTheFriendlyName(t *testing.T) {
	issues := []workflow.LintIssue{{Code: workflow.LintDataRefNeverSet, Path: "campvars.valor"}, {Code: workflow.LintDataRefNeverSet, Path: "var.outro"}}
	labelConversationRefs(context.Background(), &defaultRules{}, "ws", issues)
	if issues[0].Values["label"] != "Valor da parcela" || issues[1].Values != nil {
		t.Fatalf("issues = %+v", issues)
	}
}
