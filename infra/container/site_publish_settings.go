package container

import (
	"context"

	"vozko/domain/site"
	sitepublish_usecase "vozko/usecases/sitepublish"
)

func (a siteProjectsAdapter) CurrentSettings(ctx context.Context, workspaceID, projectID string) (site.Settings, error) {
	stored, err := a.storage.GetSettings(ctx, workspaceID, projectID)
	if err != nil {
		return site.Settings{}, a.translate(err)
	}
	if stored == nil {
		return site.Settings{}, sitepublish_usecase.ErrProjectNotFound
	}
	return stored.Settings, nil
}
