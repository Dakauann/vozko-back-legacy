package automationneeds

import (
	"vozko/domain/agent"
	"vozko/domain/conversation"
	"vozko/domain/workflow"
)

type DeclarationUser struct {
	Link
	DepartmentID string
}

func AgentVariableNames(a *agent.Agent) []string {
	names := make([]string, 0, len(a.Variables))
	for _, v := range a.Variables {
		names = append(names, v.Name)
	}
	names = append(names, agent.ExtractVariableNames(a)...)
	for _, tool := range a.InternalTools {
		if tool.Name != workflow.AgentSheetRowTool {
			continue
		}
		if name, set, err := workflow.SheetVariableOf(tool.Config[workflow.SheetStepVariableKey]); err == nil && set {
			names = append(names, name)
		}
	}
	return names
}

func WorkflowVariableNames(w *workflow.Workflow) []string {
	names := workflow.ExtractCampaignVars(w.Graph)
	for _, use := range w.Graph.Variables {
		names = append(names, use.Name)
	}
	return names
}

func DeclarationUsers(agents []*agent.Agent, workflows []*workflow.Workflow) map[string][]DeclarationUser {
	out := map[string][]DeclarationUser{}
	seen := map[string]map[conversation.Automation]bool{}
	add := func(names []string, user DeclarationUser) {
		for _, name := range names {
			if IsSystemKey(name) {
				continue
			}
			if seen[name] == nil {
				seen[name] = map[conversation.Automation]bool{}
			}
			if seen[name][user.Automation] {
				continue
			}
			seen[name][user.Automation] = true
			out[name] = append(out[name], user)
		}
	}
	for _, a := range agents {
		add(AgentVariableNames(a), DeclarationUser{Link: Link{Automation: conversation.Automation{Kind: conversation.AutomationAgent, ID: a.ID}, Name: a.Name}, DepartmentID: a.DepartmentID})
	}
	for _, w := range workflows {
		add(WorkflowVariableNames(w), DeclarationUser{Link: Link{Automation: conversation.Automation{Kind: conversation.AutomationWorkflow, ID: w.ID}, Name: w.Name}, DepartmentID: w.DepartmentID})
	}
	return out
}
