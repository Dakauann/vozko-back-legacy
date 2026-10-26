package workspace_plan_repository

import (
	"errors"

	"gorm.io/gorm"

	workspace_plan "vozko/domain/workspace/workspace_plan"
	"vozko/infra/database"
	"vozko/infra/database/schema"
)

const planNamesLock = "workspace_plan_definitions:names"

func NewPlanDuplicationRepository(db *gorm.DB) workspace_plan.PlanDuplicationRepository {
	return &planRepository{db: db}
}

func (r *planRepository) PlanNames() ([]string, error) {
	return planNames(r.db)
}

func planNames(db *gorm.DB) ([]string, error) {
	var names []string
	if err := db.Model(&schema.WorkspacePlanDefinition{}).Pluck("name", &names).Error; err != nil {
		return nil, err
	}
	return names, nil
}

func (r *planRepository) FindDuplication(requestedBy, idempotencyKey string) (*workspace_plan.PlanDuplication, error) {
	var row schema.WorkspacePlanDuplication
	err := r.db.Where("requested_by = ? AND idempotency_key = ?", requestedBy, idempotencyKey).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, workspace_plan.ErrPlanDuplicationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &workspace_plan.PlanDuplication{
		SourcePlanID:   row.SourcePlanID,
		PlanID:         row.PlanID,
		RequestedBy:    row.RequestedBy,
		IdempotencyKey: row.IdempotencyKey,
		CreatedAt:      row.CreatedAt,
	}, nil
}

func (r *planRepository) CreateDuplicate(plan *workspace_plan.PlanDefinition, record workspace_plan.PlanDuplication, name workspace_plan.PlanNamer) error {
	if plan == nil || name == nil {
		return workspace_plan.ErrPlanNotFound
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", planNamesLock).Error; err != nil {
			return err
		}
		taken, err := planNames(tx)
		if err != nil {
			return err
		}
		chosen, err := name(taken)
		if err != nil {
			return err
		}
		plan.Name = chosen
		if err := createPlan(tx, plan); err != nil {
			return err
		}
		if err := replacePricingItems(tx, plan.ID, plan.PricingItems); err != nil {
			return err
		}
		return tx.Create(&schema.WorkspacePlanDuplication{
			SourcePlanID:   record.SourcePlanID,
			PlanID:         plan.ID,
			RequestedBy:    record.RequestedBy,
			IdempotencyKey: record.IdempotencyKey,
		}).Error
	})
	if database.IsUniqueViolationOf(err, schema.PlanDuplicationKeyIndex) {
		return workspace_plan.ErrPlanDuplicationExists
	}
	return err
}
