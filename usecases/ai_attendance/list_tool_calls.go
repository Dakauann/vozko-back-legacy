package ai_attendance_usecase

import (
	"log/slog"

	aa "vozko/domain/ai_attendance"
	"vozko/domain/conversation"
	"vozko/domain/shared"
)

var ErrConversationNotVisible = conversation.ErrNotVisible

type ListToolCallsUseCase struct {
	calls      aa.ToolCallLog
	visibility conversation.Visibility
}

func NewListToolCallsUseCase(calls aa.ToolCallLog, visibility conversation.Visibility) *ListToolCallsUseCase {
	return &ListToolCallsUseCase{calls: calls, visibility: visibility}
}

func (uc *ListToolCallsUseCase) Execute(person shared.Person, workspaceID, entryType, entryID string, pagination shared.Pagination) (*shared.PaginatedResult[*aa.ToolCall], error) {
	if err := conversation.RequireVisible(uc.visibility, person, workspaceID, entryID, entryType); err != nil {
		if err != ErrConversationNotVisible {
			slog.Warn("ai_attendance.tool_calls.visibility_unreadable",
				slog.String("workspace_id", workspaceID), slog.String("entry_id", entryID), slog.String("error", err.Error()))
		}
		return nil, err
	}
	return uc.calls.ListByEntry(workspaceID, entryID, entryType, pagination)
}
