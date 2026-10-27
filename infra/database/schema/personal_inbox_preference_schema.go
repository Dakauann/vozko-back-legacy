package schema

import "time"

type PersonalInboxPreference struct {
	WorkspaceID string    `gorm:"primaryKey;type:uuid"`
	UserID      string    `gorm:"primaryKey;type:uuid"`
	HideGroups  bool      `gorm:"not null;default:false"`
	UpdatedAt   time.Time `gorm:"type:timestamptz;not null"`
}

func (PersonalInboxPreference) TableName() string { return "personal_inbox_preferences" }
