package agent_usecase

import (
	"context"

	"github.com/google/uuid"

	"vozko/domain/agent"
	"vozko/domain/rag"
	"vozko/domain/tools"
	workspace_department "vozko/domain/workspace/workspace_department"
)

type createAgentUseCase struct {
	toolBindingGuard
	toolSecretSealing
	toolValidation
	repo               agent.Repository
	toolRegistry       tools.Service
	knowledgeBaseRepo  rag.KnowledgeBaseRepository
	departmentResolver workspace_department.CreationDepartmentResolver
}

func NewCreateAgentUseCase(
	repo agent.Repository,
	toolRegistry tools.Service,
	knowledgeBaseRepo rag.KnowledgeBaseRepository,
	departmentResolver ...workspace_department.CreationDepartmentResolver,
) agent.CreateAgentUseCase {
	var resolver workspace_department.CreationDepartmentResolver
	if len(departmentResolver) > 0 {
		resolver = departmentResolver[0]
	}
	return &createAgentUseCase{
		repo:               repo,
		toolRegistry:       toolRegistry,
		knowledgeBaseRepo:  knowledgeBaseRepo,
		departmentResolver: resolver,
	}
}

func (uc *createAgentUseCase) Execute(ctx context.Context, in agent.CreateAgentInput) (*agent.Agent, error) {
	a := agent.BuildForCreate(in)

	a.Normalize()
	if err := a.Validate(); err != nil {
		return nil, err
	}

	if uc.departmentResolver != nil {
		departmentID, err := uc.departmentResolver.Resolve(ctx, a.WorkspaceID)
		if err != nil {
			return nil, err
		}
		a.DepartmentID = departmentID
	}

	if a.ID == "" {
		a.ID = uuid.New().String()
	}

	if !a.IsActive {
		a.IsActive = true
	}

	if err := validateKnowledgeBaseOwnership(ctx, uc.knowledgeBaseRepo, a.WorkspaceID, a.KnowledgeBaseIDs); err != nil {
		return nil, err
	}

	selectedTools, err := resolveInternalToolSelection(uc.toolRegistry, a.InternalTools, nil)
	if err != nil {
		return nil, err
	}
	if err := uc.refuseUngrantedBindings(ctx, uc.toolRegistry, a.WorkspaceID, in.RequestedBy, nil, selectedTools); err != nil {
		return nil, err
	}
	sealedTools, err := uc.sealBindings(ctx, uc.toolRegistry, a.WorkspaceID, in.RequestedBy.UserID, a.ID, nil, selectedTools)
	if err != nil {
		return nil, err
	}
	a.InternalTools = sealedTools
	a.ChangedBy = in.RequestedBy.UserID

	if err := uc.repo.Create(a); err != nil {
		return nil, err
	}

	created, err := uc.repo.FindByID(a.ID)
	if err != nil {
		return nil, err
	}

	return created, nil
}
