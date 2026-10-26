package schema

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const PlanDuplicationKeyIndex = "ux_workspace_plan_duplications_key"

type WorkspacePlanDuplication struct {
	ID             string    `gorm:"primaryKey;type:uuid"`
	SourcePlanID   string    `gorm:"type:uuid;not null;index"`
	PlanID         string    `gorm:"type:uuid;not null;uniqueIndex"`
	RequestedBy    string    `gorm:"type:uuid;not null;uniqueIndex:ux_workspace_plan_duplications_key,priority:1"`
	IdempotencyKey string    `gorm:"type:varchar(128);not null;uniqueIndex:ux_workspace_plan_duplications_key,priority:2"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`

	Plan WorkspacePlanDefinition `gorm:"foreignKey:PlanID;constraint:OnDelete:CASCADE"`
}

func (WorkspacePlanDuplication) TableName() string { return "workspace_plan_duplications" }

func (d *WorkspacePlanDuplication) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	return nil
}
