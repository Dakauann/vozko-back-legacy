package workflow_usecase

import (
	"context"
	"fmt"

	"vozko/domain/automationneeds"
	"vozko/domain/workflow"
)

type VariableRules = automationneeds.ValueRules

func variableDefaultIssues(ctx context.Context, rules VariableRules, w *workflow.Workflow) ([]workflow.LintIssue, error) {
	defaults := map[string]string{}
	for _, use := range w.Graph.Variables {
		if use.Optional() {
			defaults[use.Name] = use.Default
		}
	}
	rejected, err := automationneeds.RejectedDefaults(ctx, rules, w.WorkspaceID, defaults)
	if err != nil {
		return nil, err
	}
	var issues []workflow.LintIssue
	for _, need := range rejected {
		issues = append(issues, workflow.LintIssue{
			Code: workflow.LintGraphVariableDefaultInvalid, Severity: workflow.SeverityBlocking, Path: need.Name,
			Message: fmt.Sprintf("O valor padrão de %s não está no formato desse dado", need.Display()),
			Hint:    "Abra Dados do fluxo e corrija o valor padrão.",
			Values:  map[string]string{"name": need.Name, "label": need.Display()},
		})
	}
	return issues, nil
}

func labelConversationRefs(ctx context.Context, rules VariableRules, workspaceID string, issues []workflow.LintIssue) {
	var needs automationneeds.Needs
	at := map[string][]int{}
	for i, issue := range issues {
		name, ok := workflow.CampaignVarOf(issue.Path)
		if !ok || issue.Values["label"] != "" {
			continue
		}
		if _, seen := at[name]; !seen {
			needs = append(needs, automationneeds.Need{Name: name})
		}
		at[name] = append(at[name], i)
	}
	if len(needs) == 0 {
		return
	}
	for _, need := range rules.Label(ctx, workspaceID, needs) {
		for _, i := range at[need.Name] {
			values := map[string]string{"label": need.Display()}
			for k, v := range issues[i].Values {
				values[k] = v
			}
			values["label"] = need.Display()
			issues[i].Values = values
		}
	}
}
