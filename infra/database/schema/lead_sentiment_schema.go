package schema

import "time"

type LeadSentiment struct {
	LeadID      string    `gorm:"primaryKey;type:uuid"`
	WorkspaceID string    `gorm:"type:uuid;not null"`
	Sentiment   string    `gorm:"size:16;not null"`
	AnalysisID  string    `gorm:"type:uuid;not null"`
	RefreshedAt time.Time `gorm:"type:timestamptz;not null"`
}

func (LeadSentiment) TableName() string { return "lead_sentiments" }
