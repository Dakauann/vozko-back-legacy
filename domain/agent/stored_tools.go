package agent

import "context"

type StoredAgentTools struct {
	AgentID     string
	WorkspaceID string
	Bindings    []ToolBinding
}

type StoredToolScanner interface {
	AgentToolsAfter(ctx context.Context, afterID string, limit int) ([]StoredAgentTools, error)
	SaveAgentTools(ctx context.Context, agentID string, bindings []ToolBinding) error
}
