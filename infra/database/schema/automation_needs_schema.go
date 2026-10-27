package schema

import (
	"time"

	"github.com/lib/pq"
)

type ConversationVariable struct {
	EntryID     string    `gorm:"primaryKey;type:text"`
	EntryType   string    `gorm:"primaryKey;size:32"`
	Name        string    `gorm:"primaryKey;size:64"`
	WorkspaceID string    `gorm:"type:uuid;not null;index"`
	Value       string    `gorm:"type:text;not null"`
	SetBy       string    `gorm:"type:text;not null;default:''"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

func (ConversationVariable) TableName() string { return "conversation_variables" }

type AutomationSwitchOff struct {
	ID             string         `gorm:"primaryKey;type:uuid"`
	WorkspaceID    string         `gorm:"type:uuid;not null;index:idx_automation_switch_offs_slot,priority:1"`
	SlotKind       string         `gorm:"size:32;not null;index:idx_automation_switch_offs_slot,priority:2"`
	SlotID         string         `gorm:"type:text;not null;index:idx_automation_switch_offs_slot,priority:3"`
	SlotName       string         `gorm:"type:text;not null;default:''"`
	AutomationKind string         `gorm:"size:16;not null"`
	AutomationID   string         `gorm:"type:text;not null"`
	AutomationName string         `gorm:"type:text;not null;default:''"`
	Variables      pq.StringArray `gorm:"type:text[]"`
	SwitchedOffBy  string         `gorm:"type:text;not null;default:''"`
	SwitchedOffAt  time.Time      `gorm:"not null"`
	ResolvedAt     *time.Time
}

func (AutomationSwitchOff) TableName() string { return "automation_switch_offs" }
