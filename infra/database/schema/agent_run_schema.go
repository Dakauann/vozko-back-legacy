package schema

import "time"

type AgentRunSchema struct {
	ID              string    `gorm:"primaryKey;type:uuid"`
	WorkspaceID     string    `gorm:"type:uuid;not null"`
	AgentID         string    `gorm:"type:varchar(100);not null"`
	AgentVersion    int       `gorm:"not null;default:0"`
	EntryID         *string   `gorm:"type:uuid"`
	EntryType       string    `gorm:"type:varchar(40);not null;default:''"`
	Source          string    `gorm:"type:varchar(20);not null"`
	Model           string    `gorm:"type:varchar(200);not null;default:''"`
	Status          string    `gorm:"type:varchar(20);not null"`
	ErrorKind       string    `gorm:"type:varchar(30);not null;default:''"`
	Error           string    `gorm:"type:text;not null;default:''"`
	DurationMs      int64     `gorm:"not null;default:0"`
	InputTokens     int       `gorm:"not null;default:0"`
	OutputTokens    int       `gorm:"not null;default:0"`
	CachedTokens    int       `gorm:"not null;default:0"`
	ReasoningTokens int       `gorm:"not null;default:0"`
	CostMicros      *int64    `gorm:"type:bigint"`
	ToolCalls       int       `gorm:"not null;default:0"`
	FailedToolCalls int       `gorm:"not null;default:0"`
	HandledFailures int       `gorm:"not null;default:0"`
	ModelCalls      string    `gorm:"type:jsonb;not null;default:'[]'"`
	Tools           string    `gorm:"type:jsonb;not null;default:'[]'"`
	UserMessage     string    `gorm:"type:text;not null;default:''"`
	Reply           string    `gorm:"type:text;not null;default:''"`
	StartedAt       time.Time `gorm:"type:timestamptz;not null"`
}

func (AgentRunSchema) TableName() string { return "agent_runs" }
