package agentrun

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"vozko/domain/ai"
	"vozko/domain/ai_attendance"
	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/workflow"
)

const (
	PayloadRetention = 30 * 24 * time.Hour
	RowRetention     = 180 * 24 * time.Hour
)

const ErrorKindModel = "model"

var (
	ErrRunNotFound   = errors.New("agent run: not found")
	ErrFilterInvalid = errors.New("agent run: invalid filter")
)

type Source string

const (
	SourceConversation Source = "conversation"
	SourceSimulator    Source = "simulator"
	SourceScenario     Source = "scenario"
	SourceWorkflow     Source = "workflow"
)

func (s Source) Known() bool {
	switch s {
	case SourceConversation, SourceSimulator, SourceScenario, SourceWorkflow:
		return true
	}
	return false
}

func (s Source) Sandboxed() bool {
	return s == SourceSimulator || s == SourceScenario
}

func SandboxedSources() []string {
	return []string{string(SourceSimulator), string(SourceScenario)}
}

type Status string

const (
	StatusOK         Status = "ok"
	StatusToolFailed Status = "tool_failed"
	StatusFailed     Status = "failed"
)

func (s Status) Known() bool {
	return s == StatusOK || s == StatusToolFailed || s == StatusFailed
}

func StatusOf(modelFailed bool, failedToolCalls int) Status {
	switch {
	case modelFailed:
		return StatusFailed
	case failedToolCalls > 0:
		return StatusToolFailed
	}
	return StatusOK
}

type FollowedBy string

const (
	FollowedRetry     FollowedBy = "retry"
	FollowedOtherTool FollowedBy = "other_tool"
	FollowedAnswer    FollowedBy = "answer"
	FollowedNothing   FollowedBy = "nothing"
)

func (f FollowedBy) Handled() bool {
	return f == FollowedRetry || f == FollowedOtherTool
}

type Step struct {
	Name      string
	Failed    bool
	ModelCall int
}

func FollowUps(steps []Step, answered bool) []FollowedBy {
	out := make([]FollowedBy, len(steps))
	for i, s := range steps {
		if !s.Failed {
			continue
		}
		out[i] = followUpOf(steps, s, answered)
	}
	return out
}

func followUpOf(steps []Step, failed Step, answered bool) FollowedBy {
	next := -1
	for _, s := range steps {
		if s.ModelCall > failed.ModelCall && (next < 0 || s.ModelCall < next) {
			next = s.ModelCall
		}
	}
	if next < 0 {
		if answered {
			return FollowedAnswer
		}
		return FollowedNothing
	}
	for _, s := range steps {
		if s.ModelCall == next && s.Name == failed.Name {
			return FollowedRetry
		}
	}
	return FollowedOtherTool
}

type ToolPin struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
}

type Run struct {
	ID              string         `json:"id"`
	WorkspaceID     string         `json:"workspaceId"`
	AgentID         string         `json:"agentId"`
	AgentVersion    int            `json:"agentVersion,omitempty"`
	EntryID         string         `json:"entryId,omitempty"`
	EntryType       string         `json:"entryType,omitempty"`
	Source          Source         `json:"source"`
	Model           string         `json:"model"`
	Status          Status         `json:"status"`
	ErrorKind       string         `json:"errorKind,omitempty"`
	Error           string         `json:"error,omitempty"`
	DurationMs      int64          `json:"durationMs"`
	InputTokens     int            `json:"inputTokens"`
	OutputTokens    int            `json:"outputTokens"`
	CachedTokens    int            `json:"cachedTokens"`
	ReasoningTokens int            `json:"reasoningTokens"`
	CostMicros      *int64         `json:"costMicros"`
	ToolCalls       int            `json:"toolCalls"`
	FailedToolCalls int            `json:"failedToolCalls"`
	HandledFailures int            `json:"handledFailures"`
	ModelCalls      []ai.ModelCall `json:"modelCalls"`
	Tools           []ToolPin      `json:"tools"`
	UserMessage     string         `json:"userMessage,omitempty"`
	Reply           string         `json:"reply,omitempty"`
	StartedAt       time.Time      `json:"startedAt"`
}

type Detail struct {
	Run   Run                      `json:"run"`
	Spans []ai_attendance.ToolCall `json:"spans"`
}

type Filter struct {
	From           time.Time
	To             time.Time
	Source         Source
	Status         Status
	ToolInstanceID string
	ErrorKind      string
	EntryID        string
	HandledFailure bool
}

func (f Filter) Check() error {
	if f.Source != "" && !f.Source.Known() {
		return ErrFilterInvalid
	}
	if f.Status != "" && !f.Status.Known() {
		return ErrFilterInvalid
	}
	if !f.From.IsZero() && !f.To.IsZero() && f.From.After(f.To) {
		return ErrFilterInvalid
	}
	if f.EntryID != "" {
		if _, err := uuid.Parse(f.EntryID); err != nil {
			return ErrFilterInvalid
		}
	}
	return nil
}

type Log interface {
	Record(run *Run, spans []ai_attendance.ToolCall) error
}

type Repository interface {
	Log
	List(workspaceID, agentID string, filter Filter, visibility conversation.VisibilityScope, pagination shared.Pagination) (*shared.PaginatedResult[*Run], error)
	Find(workspaceID, agentID, runID string) (*Detail, error)
	PrunePayloads(before time.Time, limit int) (int64, error)
	DeleteStartedBefore(before time.Time, limit int) (int64, error)
}

func (r Run) InConversation() bool {
	return r.EntryID != "" && r.EntryType != workflow.EntryTypeSIPCall && r.EntryType != workflow.EntryTypeLead
}
