package agent

import "strings"

func (a *Agent) TakesConversationsIn(workspaceID string) bool {
	workspaceID = strings.TrimSpace(workspaceID)
	return a != nil && workspaceID != "" && a.WorkspaceID == workspaceID && a.IsActive
}
