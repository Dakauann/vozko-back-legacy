package schema

import "time"

type WorkflowWebhookDeliverySchema struct {
	ID              string    `gorm:"primaryKey;type:uuid"`
	WebhookID       string    `gorm:"type:uuid;not null"`
	WorkflowID      string    `gorm:"type:uuid;not null"`
	WorkspaceID     string    `gorm:"type:uuid;not null"`
	ReceivedAt      time.Time `gorm:"not null"`
	Outcome         string    `gorm:"not null;size:20"`
	Reason          string    `gorm:"size:60"`
	HTTPStatus      int       `gorm:"column:http_status;not null"`
	EntryResolution string    `gorm:"size:20"`
	EntryID         string    `gorm:"size:100"`
	EntryType       string    `gorm:"size:40"`
	RunID           *string   `gorm:"type:uuid"`
	LatencyMs       int64     `gorm:"not null"`
	PayloadKeys     *string   `gorm:"type:jsonb"`
}

func (WorkflowWebhookDeliverySchema) TableName() string { return "workflow_webhook_deliveries" }

type AIToolCallSchema struct {
	ID             string    `gorm:"primaryKey;type:uuid"`
	ToolCallID     *string   `gorm:"size:200"`
	WorkspaceID    string    `gorm:"type:uuid;not null"`
	EntryID        string    `gorm:"not null;size:100"`
	EntryType      string    `gorm:"not null;size:40"`
	AgentID        string    `gorm:"not null;size:100"`
	SessionID      string    `gorm:"size:200"`
	ToolName       string    `gorm:"not null;size:120"`
	Args           *string   `gorm:"type:jsonb"`
	Outcome        string    `gorm:"not null;size:20"`
	Error          string    `gorm:"type:text"`
	ErrorCode      string    `gorm:"size:60"`
	DurationMs     int64     `gorm:"not null"`
	RunID          *string   `gorm:"type:uuid"`
	CreatedAt      time.Time `gorm:"not null"`
	AgentRunID     *string   `gorm:"type:uuid"`
	Source         string    `gorm:"type:varchar(20);not null;default:''"`
	ToolInstanceID string    `gorm:"type:varchar(64);not null;default:''"`
	ToolType       string    `gorm:"type:varchar(120);not null;default:''"`
	ToolRevision   int       `gorm:"not null;default:0"`
	Sequence       int       `gorm:"not null;default:0"`
	ModelCallIndex int       `gorm:"not null;default:0"`
	ErrorKind      string    `gorm:"type:varchar(30);not null;default:''"`
	ErrorChain     *string   `gorm:"type:jsonb"`
	HTTP           *string   `gorm:"column:http;type:jsonb"`
	ModelResult    *string   `gorm:"type:text"`
	MappedOutput   *string   `gorm:"type:jsonb"`
	FollowedBy     string    `gorm:"type:varchar(20);not null;default:''"`
	Simulated      bool      `gorm:"not null;default:false"`
}

func (AIToolCallSchema) TableName() string { return "ai_tool_calls" }

type WorkflowRuntimeAlertSchema struct {
	ID          string     `gorm:"primaryKey;type:uuid"`
	WorkspaceID string     `gorm:"type:uuid;not null;uniqueIndex:ux_wfra_open,priority:1,where:status = 'open'"`
	WorkflowID  string     `gorm:"type:varchar(36);not null;default:'';uniqueIndex:ux_wfra_open,priority:2,where:status = 'open'"`
	NodeID      string     `gorm:"type:varchar(100);not null;default:'';uniqueIndex:ux_wfra_open,priority:3,where:status = 'open'"`
	AgentID     string     `gorm:"type:varchar(100);not null;default:'';uniqueIndex:ux_wfra_open,priority:4,where:status = 'open'"`
	Subject     string     `gorm:"type:varchar(120);not null;default:'';uniqueIndex:ux_wfra_open,priority:5,where:status = 'open'"`
	Kind        string     `gorm:"type:varchar(40);not null;uniqueIndex:ux_wfra_open,priority:6,where:status = 'open'"`
	Count       int        `gorm:"not null;default:0"`
	FirstSeen   time.Time  `gorm:"type:timestamptz;not null"`
	LastSeen    time.Time  `gorm:"type:timestamptz;not null"`
	Sample      string     `gorm:"type:varchar(200);not null;default:''"`
	Status      string     `gorm:"type:varchar(20);not null"`
	ResolvedAt  *time.Time `gorm:"type:timestamptz"`
	NotifiedAt  *time.Time `gorm:"type:timestamptz"`
	CreatedAt   time.Time  `gorm:"type:timestamptz;not null"`
	UpdatedAt   time.Time  `gorm:"type:timestamptz;not null"`
}

func (WorkflowRuntimeAlertSchema) TableName() string { return "workflow_runtime_alerts" }
