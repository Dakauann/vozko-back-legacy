package workflow_usecase

import (
	"context"

	"vozko/domain/shared/deps"
	"vozko/domain/workflow"
)

type updateWorkflowUseCase struct {
	repo         workflow.WorkflowRepository
	modelCatalog ModelCatalog
}

func NewUpdateWorkflowUseCase(repo workflow.WorkflowRepository, catalog ModelCatalog) workflow.UpdateWorkflowUseCase {
	if err := deps.Require("workflow update", map[string]any{"workflows": repo, "model catalog": catalog}); err != nil {
		panic(err.Error())
	}
	return &updateWorkflowUseCase{repo: repo, modelCatalog: catalog}
}

func (uc *updateWorkflowUseCase) Execute(ctx context.Context, workflowID string, input *workflow.Workflow, revision int) (*workflow.Workflow, error) {
	if workflowID == "" {
		return nil, workflow.ErrWorkflowNotFound
	}
	if input == nil {
		return nil, workflow.ErrNameRequired
	}
	if err := workflow.RequireDraftRevision(revision); err != nil {
		return nil, err
	}

	existing, err := uc.repo.FindByID(workflowID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, workflow.ErrWorkflowNotFound
	}

	existing.Name = input.Name
	existing.Description = input.Description

	if input.Type != "" {
		existing.Type = input.Type
	}
	existing.TriggerType = ""
	existing.TriggerConfig = nil
	existing.Graph = workflow.RestoreWithheldPins(input.Graph, existing.Graph)

	existing.Normalize()
	if err := existing.Validate(); err != nil {
		return nil, err
	}
	if err := checkGraphReasoning(ctx, uc.modelCatalog, &existing.Graph); err != nil {
		return nil, err
	}

	if _, err := uc.repo.SaveDraft(existing, revision); err != nil {
		return nil, err
	}

	return findWithPublication(uc.repo, workflowID)
}
