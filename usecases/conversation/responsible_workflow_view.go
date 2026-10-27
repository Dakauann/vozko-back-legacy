package conversation_usecase

import (
	"fmt"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	"vozko/domain/workflow"
)

type ResponsibleWorkflows interface {
	ResponsibleWorkflow(workspaceID, entryID string, entryType shared.EntryType) (string, bool, error)
}

type WorkflowFinder interface {
	FindByID(workflowID string) (*workflow.Workflow, error)
}

type ResponsibleWorkflow struct {
	ID    string                `json:"id"`
	Name  string                `json:"name"`
	State workflow.RunningState `json:"state"`
}

type ResponsibleWorkflowViewDeps struct {
	Gate       ResponsibleWorkflows
	Visibility conversation.Visibility
	Workflows  WorkflowFinder
}

type ResponsibleWorkflowView struct {
	gate       ResponsibleWorkflows
	visibility conversation.Visibility
	workflows  WorkflowFinder
}

func NewResponsibleWorkflowView(d ResponsibleWorkflowViewDeps) *ResponsibleWorkflowView {
	if err := deps.Require("responsible workflow view", map[string]any{
		"automation gate": d.Gate,
		"visibility":      d.Visibility,
		"workflows":       d.Workflows,
	}); err != nil {
		panic(err.Error())
	}
	return &ResponsibleWorkflowView{gate: d.Gate, visibility: d.Visibility, workflows: d.Workflows}
}

func (v *ResponsibleWorkflowView) Execute(viewer shared.Person, workspaceID, entryID, entryType string) (*ResponsibleWorkflow, error) {
	if err := conversation.RequireVisible(v.visibility, viewer, workspaceID, entryID, entryType); err != nil {
		return nil, err
	}
	workflowID, ok, err := v.gate.ResponsibleWorkflow(workspaceID, entryID, shared.EntryType(entryType))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	w, err := v.workflows.FindByID(workflowID)
	if err != nil {
		return nil, fmt.Errorf("responsible workflow %s: %w", workflowID, err)
	}
	if w == nil || w.WorkspaceID != workspaceID {
		return nil, nil
	}
	return &ResponsibleWorkflow{ID: w.ID, Name: w.Name, State: workflow.RunningStateOf(w)}, nil
}
