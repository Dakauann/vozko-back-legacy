package agent_usecase

import (
	"context"

	"vozko/domain/agent"
	"vozko/domain/rag"
	"vozko/domain/tools"
)

type updateAgentUseCase struct {
	toolBindingGuard
	toolSecretSealing
	toolValidation
	repo              agent.Repository
	toolRegistry      tools.Service
	knowledgeBaseRepo rag.KnowledgeBaseRepository
}

func NewUpdateAgentUseCase(
	repo agent.Repository,
	toolRegistry tools.Service,
	knowledgeBaseRepo rag.KnowledgeBaseRepository,
) agent.UpdateAgentUseCase {
	return &updateAgentUseCase{
		repo:              repo,
		toolRegistry:      toolRegistry,
		knowledgeBaseRepo: knowledgeBaseRepo,
	}
}

func (uc *updateAgentUseCase) Execute(ctx context.Context, agentID string, in agent.UpdateAgentInput) (*agent.Agent, error) {
	if agentID == "" {
		return nil, agent.ErrAgentNotFound
	}

	current, err := uc.repo.FindByID(agentID)
	if err != nil {
		return nil, err
	}

	if current == nil {
		return nil, agent.ErrAgentNotFound
	}

	existingTools := make([]agent.ToolBinding, len(current.InternalTools))
	copy(existingTools, current.InternalTools)

	wasActive := current.IsActive
	current.ApplyUpdate(in)

	current.Normalize()
	if err := current.Validate(); err != nil {
		return nil, err
	}

	if err := validateKnowledgeBaseOwnership(ctx, uc.knowledgeBaseRepo, current.WorkspaceID, current.KnowledgeBaseIDs); err != nil {
		return nil, err
	}

	selectedTools, err := resolveInternalToolSelection(uc.toolRegistry, in.InternalTools, existingTools)
	if err != nil {
		return nil, err
	}
	if err := uc.refuseUngrantedBindings(ctx, uc.toolRegistry, current.WorkspaceID, in.RequestedBy, existingTools, selectedTools); err != nil {
		return nil, err
	}
	sealedTools, err := uc.sealBindings(ctx, uc.toolRegistry, current.WorkspaceID, in.RequestedBy.UserID, agentID, existingTools, selectedTools)
	if err != nil {
		return nil, err
	}
	current.InternalTools = sealedTools
	current.ChangedBy = in.RequestedBy.UserID
	if err := uc.refuseBrokenActivation(ctx, wasActive, current); err != nil {
		return nil, err
	}

	if err := uc.repo.Update(agentID, current); err != nil {
		return nil, err
	}

	updated, err := uc.repo.FindByID(agentID)
	if err != nil {
		return nil, err
	}

	return updated, nil
}
