package personalinbox_usecase

import (
	"context"
	"log"
	"strings"

	"vozko/domain/conversation"
	"vozko/domain/personalinbox"
	"vozko/domain/shared"
	"vozko/domain/workspace"
)

func (s *Service) InboxPreferences(ctx context.Context, workspaceID, userID string) personalinbox.Preferences {
	if personalinbox.ValidatePreferencesOwner(workspaceID, userID) != nil {
		return personalinbox.DefaultPreferences()
	}
	prefs, err := s.deps.Preferences.FindPreferences(ctx, strings.TrimSpace(workspaceID), strings.TrimSpace(userID))
	if err != nil {
		log.Printf("[personal-inbox] reading the inbox preferences of %s in %s failed, using the default: %v", userID, workspaceID, err)
		return personalinbox.DefaultPreferences()
	}
	return prefs
}

func (s *Service) SaveInboxPreferences(ctx context.Context, by shared.Person, workspaceID string, prefs personalinbox.Preferences) (personalinbox.Preferences, error) {
	workspaceID, userID := strings.TrimSpace(workspaceID), strings.TrimSpace(by.UserID)
	if err := personalinbox.ValidatePreferencesOwner(workspaceID, userID); err != nil {
		return personalinbox.Preferences{}, err
	}
	if !s.deps.Access.HasWorkspacePermission(userID, workspaceID, string(workspace.ResourceConversations), string(workspace.ActionRead), by.SystemAdmin) {
		return personalinbox.Preferences{}, conversation.ErrUnauthorized
	}
	if err := s.deps.Preferences.SavePreferences(ctx, workspaceID, userID, prefs); err != nil {
		return personalinbox.Preferences{}, err
	}
	s.deps.Announcer.AnnounceInboxPreferences(workspaceID, userID, prefs)
	return prefs, nil
}
