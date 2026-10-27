package personalinbox

import (
	"context"
	"errors"
	"strings"
)

var ErrWorkspaceRequired = errors.New("personalinbox: preferences need their workspace")

type Preferences struct {
	HideGroups bool
}

func DefaultPreferences() Preferences {
	return Preferences{}
}

func ValidatePreferencesOwner(workspaceID, userID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return ErrWorkspaceRequired
	}
	if strings.TrimSpace(userID) == "" {
		return ErrUserRequired
	}
	return nil
}

type PreferencesRepository interface {
	FindPreferences(ctx context.Context, workspaceID, userID string) (Preferences, error)
	SavePreferences(ctx context.Context, workspaceID, userID string, prefs Preferences) error
}
