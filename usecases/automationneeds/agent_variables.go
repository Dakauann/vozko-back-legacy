package automationneeds

import (
	"context"
	"fmt"

	"vozko/domain/agent"
	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
)

type AgentVariable struct {
	Need     domain.Need
	Required bool
	InAgent  bool
	InPrompt bool
	Used     bool
	FilledBy []Filler
	Problem  error
}

func (q *Queries) AgentVariables(ctx context.Context, workspaceID string, candidate *agent.Agent) ([]AgentVariable, error) {
	if candidate == nil || candidate.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("%w: agent", domain.ErrNotFound)
	}
	root := conversation.Automation{Kind: conversation.AutomationAgent, ID: candidate.ID}
	vars, err := domain.VariablesOf(domain.OverlayAgent(q.d.Service.Source(), candidate), workspaceID, root)
	if err != nil {
		return nil, err
	}
	labels, err := q.labels(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	own := make(map[string]agent.AgentVariable, len(candidate.Variables))
	for _, v := range candidate.Variables {
		if _, seen := own[v.Name]; !seen {
			own[v.Name] = v
		}
	}
	prompt := map[string]bool{}
	for _, name := range agent.ExtractVariableNames(candidate) {
		prompt[name] = true
	}
	problems := agent.VariableProblems(candidate.Variables)
	row := func(need domain.Need, required, used bool) AgentVariable {
		_, inAgent := own[need.Name]
		return AgentVariable{Need: need, Required: required, InAgent: inAgent, InPrompt: prompt[need.Name], Used: used, FilledBy: fillersOf(false, need, required), Problem: problems[need.Name]}
	}
	var rows []AgentVariable
	seen := map[string]bool{}
	for _, need := range labels.Apply(vars.Required) {
		seen[need.Name] = true
		rows = append(rows, row(need, true, true))
	}
	for _, need := range labels.Apply(vars.Optional) {
		seen[need.Name] = true
		rows = append(rows, row(need, false, true))
	}
	var unused domain.Needs
	for _, v := range candidate.Variables {
		if !seen[v.Name] {
			seen[v.Name] = true
			unused = append(unused, domain.Need{Name: v.Name, Description: v.Description, Default: v.DefaultValue})
		}
	}
	for _, need := range labels.Apply(unused) {
		rows = append(rows, row(need, need.Default == "", false))
	}
	return rows, nil
}
