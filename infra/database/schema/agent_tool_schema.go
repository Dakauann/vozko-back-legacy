package schema

import (
	"time"

	"gorm.io/gorm"
)

type AgentTool struct {
	ID          string         `gorm:"primaryKey;type:uuid"`
	WorkspaceID string         `gorm:"type:uuid;not null;index:idx_agent_tools_workspace"`
	AgentID     string         `gorm:"type:uuid;not null;index:idx_agent_tools_agent_position,priority:1"`
	Type        string         `gorm:"size:64;not null"`
	Name        string         `gorm:"size:64;not null"`
	Description string         `gorm:"type:text;not null;default:''"`
	Config      string         `gorm:"type:jsonb;not null;default:'{}'"`
	Visibility  string         `gorm:"type:jsonb;not null;default:'[]'"`
	Position    int            `gorm:"not null;default:0;index:idx_agent_tools_agent_position,priority:2"`
	Revision    int            `gorm:"not null;default:1"`
	TargetKey   string         `gorm:"size:200;not null;default:''"`
	TestInputs  string         `gorm:"type:jsonb;not null;default:'[]'"`
	CreatedBy   string         `gorm:"size:64;not null;default:''"`
	UpdatedBy   string         `gorm:"size:64;not null;default:''"`
	CreatedAt   time.Time      `gorm:"not null"`
	UpdatedAt   time.Time      `gorm:"not null"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (AgentTool) TableName() string { return "agent_tools" }

type AgentToolRevision struct {
	ToolID      string    `gorm:"primaryKey;type:uuid"`
	Revision    int       `gorm:"primaryKey"`
	WorkspaceID string    `gorm:"type:uuid;not null"`
	AgentID     string    `gorm:"type:uuid;not null;index:idx_agent_tool_revisions_agent"`
	Type        string    `gorm:"size:64;not null"`
	Name        string    `gorm:"size:64;not null"`
	Description string    `gorm:"type:text;not null;default:''"`
	Config      string    `gorm:"type:jsonb;not null;default:'{}'"`
	Visibility  string    `gorm:"type:jsonb;not null;default:'[]'"`
	SavedBy     string    `gorm:"size:64;not null;default:''"`
	SavedAt     time.Time `gorm:"not null;index:idx_agent_tool_revisions_saved"`
}

func (AgentToolRevision) TableName() string { return "agent_tool_revisions" }

func AgentToolModels() []interface{} {
	return []interface{}{&AgentTool{}, &AgentToolRevision{}, &AgentVersion{}}
}

type AgentVersion struct {
	ID          string    `gorm:"primaryKey;type:uuid"`
	AgentID     string    `gorm:"type:uuid;not null;uniqueIndex:ux_agent_versions_agent_version,priority:1"`
	Version     int       `gorm:"not null;uniqueIndex:ux_agent_versions_agent_version,priority:2"`
	WorkspaceID string    `gorm:"type:uuid;not null"`
	Snapshot    string    `gorm:"type:jsonb;not null"`
	PublishedBy string    `gorm:"size:64;not null;default:''"`
	PublishedAt time.Time `gorm:"not null"`
}

func (AgentVersion) TableName() string { return "agent_versions" }
