package schema

import "time"

type WorkflowTestValue struct {
	WorkflowID  string    `gorm:"primaryKey;type:uuid"`
	UserID      string    `gorm:"primaryKey;type:text"`
	Name        string    `gorm:"primaryKey;size:64"`
	WorkspaceID string    `gorm:"type:uuid;not null;index"`
	Value       string    `gorm:"type:text;not null"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

func (WorkflowTestValue) TableName() string { return "workflow_test_values" }
