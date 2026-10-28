package schema

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type GoogleSheetEditSession struct {
	ID            string         `gorm:"primaryKey;type:uuid"`
	WorkspaceID   string         `gorm:"type:uuid;not null"`
	SpreadsheetID string         `gorm:"size:128;not null"`
	StartedBy     string         `gorm:"size:120;not null"`
	StartedAt     time.Time      `gorm:"type:timestamptz;not null"`
	ExpiresAt     time.Time      `gorm:"type:timestamptz;not null"`
	SettledAt     *time.Time     `gorm:"type:timestamptz"`
	EndedAt       *time.Time     `gorm:"type:timestamptz"`
	EndedBy       string         `gorm:"size:120;not null;default:''"`
	EndReason     string         `gorm:"size:16;not null;default:''"`
	WaitedRows    int            `gorm:"not null;default:0"`
	Check         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
}

func (GoogleSheetEditSession) TableName() string { return "google_sheet_edit_sessions" }

func (s *GoogleSheetEditSession) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return nil
}
