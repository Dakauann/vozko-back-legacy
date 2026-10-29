package schema

import "time"

type TutorialProgress struct {
	UserID     string    `gorm:"primaryKey;type:uuid"`
	TutorialID string    `gorm:"primaryKey;type:varchar(80)"`
	Status     string    `gorm:"type:varchar(16);not null"`
	Step       int       `gorm:"not null;default:0"`
	UpdatedAt  time.Time `gorm:"type:timestamptz;not null"`
}

func (TutorialProgress) TableName() string { return "tutorial_progress" }
