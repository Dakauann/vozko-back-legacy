package workspace_plan_usecase

import (
	"errors"
	"strings"

	"github.com/google/uuid"

	"vozko/domain/shared/deps"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

type DuplicatePlanDeps struct {
	Plans        workspace_plan.PlanRepository
	Duplications workspace_plan.PlanDuplicationRepository
}

func (d DuplicatePlanDeps) require(component string) error {
	return deps.Require(component, map[string]any{
		"plan repository":        d.Plans,
		"duplication repository": d.Duplications,
	})
}

type duplicatePlanDefinitionUseCase struct {
	plans        workspace_plan.PlanRepository
	duplications workspace_plan.PlanDuplicationRepository
	view         workspace_plan.GetPlanDefinitionUseCase
}

type previewPlanDuplicateUseCase struct {
	plans        workspace_plan.PlanRepository
	duplications workspace_plan.PlanDuplicationRepository
}

func NewDuplicatePlanDefinitionUseCase(d DuplicatePlanDeps) (workspace_plan.DuplicatePlanDefinitionUseCase, error) {
	if err := d.require("duplicate plan"); err != nil {
		return nil, err
	}
	return &duplicatePlanDefinitionUseCase{
		plans:        d.Plans,
		duplications: d.Duplications,
		view:         NewGetPlanDefinitionUseCase(d.Plans),
	}, nil
}

func NewPreviewPlanDuplicateUseCase(d DuplicatePlanDeps) (workspace_plan.PreviewPlanDuplicateUseCase, error) {
	if err := d.require("preview plan duplicate"); err != nil {
		return nil, err
	}
	return &previewPlanDuplicateUseCase{plans: d.Plans, duplications: d.Duplications}, nil
}

func (uc *duplicatePlanDefinitionUseCase) Execute(input workspace_plan.DuplicatePlanInput) (*workspace_plan.DuplicatePlanResult, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)

	if result, err := uc.replay(input); !errors.Is(err, workspace_plan.ErrPlanDuplicationNotFound) {
		return result, err
	}

	source, err := uc.plans.GetByID(input.SourcePlanID)
	if err != nil {
		return nil, err
	}
	copied := source.Duplicate(uuid.NewString())
	assignItemIDs(copied.PricingItems)
	if err := copied.Validate(); err != nil {
		return nil, err
	}

	record := workspace_plan.PlanDuplication{
		SourcePlanID:   source.ID,
		PlanID:         copied.ID,
		RequestedBy:    input.RequestedBy,
		IdempotencyKey: input.IdempotencyKey,
	}
	nameAfterSource := func(taken []string) (string, error) {
		return workspace_plan.DuplicateName(source.Name, taken)
	}
	err = uc.duplications.CreateDuplicate(copied, record, nameAfterSource)
	if errors.Is(err, workspace_plan.ErrPlanDuplicationExists) {
		return uc.replay(input)
	}
	if err != nil {
		return nil, err
	}

	plan, err := uc.view.Execute(copied.ID)
	if err != nil {
		return nil, err
	}
	return &workspace_plan.DuplicatePlanResult{Plan: plan}, nil
}

func (uc *duplicatePlanDefinitionUseCase) replay(input workspace_plan.DuplicatePlanInput) (*workspace_plan.DuplicatePlanResult, error) {
	record, err := uc.duplications.FindDuplication(input.RequestedBy, input.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if record.SourcePlanID != input.SourcePlanID {
		return nil, workspace_plan.ErrDuplicateKeyReused
	}
	plan, err := uc.view.Execute(record.PlanID)
	if err != nil {
		return nil, err
	}
	return &workspace_plan.DuplicatePlanResult{Plan: plan, Replayed: true}, nil
}

func (uc *previewPlanDuplicateUseCase) Execute(sourcePlanID string) (*workspace_plan.PlanDuplicatePreview, error) {
	source, err := uc.plans.GetByID(sourcePlanID)
	if err != nil {
		return nil, err
	}
	taken, err := uc.duplications.PlanNames()
	if err != nil {
		return nil, err
	}
	name, err := workspace_plan.DuplicateName(source.Name, taken)
	if err != nil {
		return nil, err
	}
	return &workspace_plan.PlanDuplicatePreview{Name: name}, nil
}
