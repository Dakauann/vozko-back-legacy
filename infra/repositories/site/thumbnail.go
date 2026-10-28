package site_repository

import (
	"context"

	site_usecase "vozko/usecases/site"
)

const setThumbnailSQL = `UPDATE site_projects p SET thumbnail_media_id = ? FROM (SELECT id, thumbnail_media_id FROM site_projects WHERE id = ? AND workspace_id = ? FOR UPDATE) previous WHERE p.id = previous.id RETURNING previous.thumbnail_media_id`

func (r *Repository) SetThumbnail(ctx context.Context, workspaceID, projectID, mediaID string) (string, error) {
	if !validID(workspaceID) || !validID(projectID) {
		return "", site_usecase.ErrProjectNotFound
	}
	var previous []struct {
		ThumbnailMediaID *string
	}
	if err := r.db.WithContext(ctx).Raw(setThumbnailSQL, mediaID, projectID, workspaceID).Scan(&previous).Error; err != nil {
		return "", err
	}
	if len(previous) == 0 {
		return "", site_usecase.ErrProjectNotFound
	}
	return deref(previous[0].ThumbnailMediaID), nil
}
