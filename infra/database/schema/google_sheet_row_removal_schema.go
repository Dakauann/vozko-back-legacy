package schema

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/infra/crypto/piigorm"
)

type GoogleSheetRowRemoval struct {
	ID          string                  `gorm:"primaryKey;type:uuid"`
	WorkspaceID string                  `gorm:"type:uuid;not null"`
	SheetID     string                  `gorm:"type:uuid;not null"`
	RowID       string                  `gorm:"type:uuid;not null"`
	Kind        string                  `gorm:"size:16;not null"`
	Reason      piigorm.EncryptedString `gorm:"type:bytea;not null" json:"-"`
	RemovedBy   string                  `gorm:"size:120;not null"`
	Cells       piigorm.EncryptedString `gorm:"type:bytea;not null" json:"-"`
	PeriodKey   string                  `gorm:"size:16;not null;default:''"`
	RowNumber   *int
	OriginKind  string    `gorm:"size:16;not null"`
	AgentID     string    `gorm:"size:64;not null;default:''"`
	WorkflowID  string    `gorm:"size:64;not null;default:''"`
	RunID       string    `gorm:"size:64;not null;default:''"`
	ActorID     string    `gorm:"size:120;not null;default:''"`
	LeadID      *string   `gorm:"size:64"`
	EntryID     string    `gorm:"size:64;not null;default:''"`
	EntryType   string    `gorm:"size:32;not null;default:''"`
	RemovedAt   time.Time `gorm:"type:timestamptz;not null"`
}

func (GoogleSheetRowRemoval) TableName() string { return "google_sheet_row_removals" }

func (r *GoogleSheetRowRemoval) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	return nil
}
