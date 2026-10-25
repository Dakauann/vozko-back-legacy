package agent

import (
	"context"
	"time"

	"vozko/domain/testinput"
	"vozko/domain/workspace"
)

type ToolDraft struct {
	Type        string
	Name        string
	Description string
	Config      map[string]interface{}
	Visibility  []ToolVisibility
}

type ToolEdit struct {
	Name        string
	Description string
	Config      map[string]interface{}
	Visibility  []ToolVisibility
	Revision    int
}

type ToolRevision struct {
	ToolID      string                 `json:"toolId"`
	Revision    int                    `json:"revision"`
	Type        string                 `json:"type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Visibility  []ToolVisibility       `json:"visibility,omitempty"`
	SavedBy     string                 `json:"savedBy"`
	SavedAt     time.Time              `json:"savedAt"`
}

type ToolRevisionReader interface {
	ToolRevisions(ctx context.Context, agentID, toolID string) ([]ToolRevision, error)
	ToolRevision(ctx context.Context, agentID, toolID string, revision int) (ToolRevision, error)
}

type AgentToolsUseCase interface {
	List(ctx context.Context, agentID string) ([]ToolBinding, error)
	Create(ctx context.Context, agentID string, draft ToolDraft, by workspace.Requester) (ToolBinding, error)
	Update(ctx context.Context, agentID, toolID string, edit ToolEdit, by workspace.Requester) (ToolBinding, error)
	Delete(ctx context.Context, agentID, toolID string, by workspace.Requester) error
	Duplicate(ctx context.Context, agentID, toolID string, by workspace.Requester) (ToolBinding, error)
	Reorder(ctx context.Context, agentID string, ids []string, by workspace.Requester) ([]ToolBinding, error)
	Revisions(ctx context.Context, agentID, toolID string) ([]ToolRevision, error)
	Revision(ctx context.Context, agentID, toolID string, revision int) (ToolRevision, error)
	SaveTestInputs(ctx context.Context, agentID, toolID string, inputs []testinput.TestInput) ([]testinput.TestInput, error)
}

type ToolTestInputStore interface {
	SaveToolTestInputs(ctx context.Context, agentID, toolID string, inputs []testinput.TestInput) error
}
