package conversation_usecase

import (
	"context"

	"vozko/domain/shared"
)

type AgentVariables interface {
	AgentValues(ctx context.Context, workspaceID, entryID string, entryType shared.EntryType, agentID string) (map[string]string, bool)
}
