package googlesheet

import (
	"context"
	"fmt"
	"time"

	"vozko/domain/googlesheet"
)

type editGate struct {
	sessions googlesheet.EditSessionRepository
}

func (g editGate) holding(ctx context.Context, workspaceID, spreadsheetID string, now time.Time) (*googlesheet.EditSession, error) {
	session, found, err := g.sessions.Current(ctx, workspaceID, spreadsheetID)
	if err != nil {
		return nil, fmt.Errorf("googlesheet: the edit lock of %s could not be read: %w", spreadsheetID, err)
	}
	if !found || !session.HoldsAt(now) {
		return nil, nil
	}
	return session, nil
}

func (g editGate) check(ctx context.Context, workspaceID, spreadsheetID string, now time.Time) error {
	session, err := g.holding(ctx, workspaceID, spreadsheetID, now)
	if err != nil {
		return err
	}
	if session != nil {
		return &googlesheet.EditTakenError{Session: session}
	}
	return nil
}
