package crmbulk_usecase

import (
	"context"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

const (
	ActionClose    = "close"
	ActionMarkRead = "mark_read"
)

type ReadMarker interface {
	MarkAsRead(entryID string, entryType shared.EntryType, messageIDs []string, userID string) error
}

type PersonalMarks interface {
	MarkRead(ctx context.Context, by shared.Person, workspaceID string, entry shared.EntryRef)
}

type ConversationActions struct {
	Finish   conversation.PersonFinishUseCase
	Read     ReadMarker
	Personal PersonalMarks
}

func (s *Service) applyConversationAction(ctx context.Context, in BulkInput, t EntryRef) error {
	by := shared.Person{UserID: in.ActorID, SystemAdmin: in.IsAdmin}
	entry := shared.EntryRef{EntryID: t.EntryID, EntryType: shared.EntryType(t.EntryType)}
	switch in.Action {
	case ActionClose:
		return s.conversations.Finish.Finish(ctx, by, in.WorkspaceID, entry, in.OutcomeCode)
	case ActionMarkRead:
		if err := s.conversations.Read.MarkAsRead(entry.EntryID, entry.EntryType, nil, in.ActorID); err != nil {
			return err
		}
		s.conversations.Personal.MarkRead(ctx, by, in.WorkspaceID, entry)
		return nil
	}
	return ErrUnknownAction
}
