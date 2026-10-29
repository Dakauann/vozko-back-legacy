package webchat

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

type SiteWidgetFinder interface {
	FindByPublicKey(ctx context.Context, publicKey string) (*Widget, error)
	FindByID(ctx context.Context, workspaceID, id string) (*Widget, error)
}

func FindSiteWidget(ctx context.Context, finder SiteWidgetFinder, workspaceID, ref string) (*Widget, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.TrimSpace(workspaceID) == "" {
		return nil, ErrWidgetNotFound
	}
	w, err := finder.FindByPublicKey(ctx, ref)
	if errors.Is(err, ErrWidgetNotFound) && uuid.Validate(ref) == nil {
		w, err = finder.FindByID(ctx, workspaceID, ref)
	}
	if err != nil {
		return nil, err
	}
	if w == nil || w.WorkspaceID != workspaceID {
		return nil, ErrWidgetNotFound
	}
	return w, nil
}
