package ai_attendance

import (
	"time"

	"vozko/domain/calltrace"
	"vozko/domain/shared"
)

type ToolCallOutcome string

const (
	ToolCallOK        ToolCallOutcome = "ok"
	ToolCallError     ToolCallOutcome = "error"
	ToolCallRequested ToolCallOutcome = "requested"
)

type ToolCall struct {
	ID             string                   `json:"id"`
	ToolCallID     string                   `json:"toolCallId,omitempty"`
	WorkspaceID    string                   `json:"workspaceId"`
	EntryID        string                   `json:"entryId"`
	EntryType      string                   `json:"entryType"`
	AgentID        string                   `json:"agentId"`
	SessionID      string                   `json:"sessionId,omitempty"`
	ToolName       string                   `json:"toolName"`
	Args           map[string]interface{}   `json:"args,omitempty"`
	Outcome        ToolCallOutcome          `json:"outcome"`
	Error          string                   `json:"error,omitempty"`
	ErrorCode      string                   `json:"errorCode,omitempty"`
	DurationMs     int64                    `json:"durationMs"`
	RunID          string                   `json:"runId,omitempty"`
	WorkflowID     string                   `json:"workflowId,omitempty"`
	CreatedAt      time.Time                `json:"createdAt"`
	AgentRunID     string                   `json:"agentRunId,omitempty"`
	Source         string                   `json:"source,omitempty"`
	ToolInstanceID string                   `json:"toolInstanceId,omitempty"`
	ToolType       string                   `json:"toolType,omitempty"`
	ToolRevision   int                      `json:"toolRevision,omitempty"`
	Sequence       int                      `json:"sequence"`
	ModelCallIndex int                      `json:"modelCallIndex"`
	ErrorKind      string                   `json:"errorKind,omitempty"`
	ErrorChain     []string                 `json:"errorChain,omitempty"`
	HTTP           []calltrace.HTTPExchange `json:"http,omitempty"`
	ModelResult    string                   `json:"modelResult,omitempty"`
	MappedOutput   interface{}              `json:"mappedOutput,omitempty"`
	FollowedBy     string                   `json:"followedBy,omitempty"`
	Simulated      bool                     `json:"simulated,omitempty"`
}

type ToolCallLog interface {
	Record(calls []ToolCall) error
	ListByEntry(workspaceID, entryID, entryType string, pagination shared.Pagination) (*shared.PaginatedResult[*ToolCall], error)
	DeleteCreatedBefore(cutoff time.Time, limit int) (int64, error)
}
