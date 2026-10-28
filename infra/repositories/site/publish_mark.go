package site_repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"vozko/infra/database/schema"
	site_usecase "vozko/usecases/site"
)

const markPublishedSQL = `UPDATE site_projects SET working_revision_id = ? WHERE id = ? AND workspace_id = ? AND version = ? AND working_revision_id = ? RETURNING *`

var _ site_usecase.PublishMarkStore = (*Repository)(nil)

func (r *Repository) MarkPublished(ctx context.Context, m site_usecase.PublishMark) (site_usecase.PublishMarked, error) {
	if !validID(m.WorkspaceID) || !validID(m.ProjectID) || !validID(m.Revision.ID) {
		return site_usecase.PublishMarked{}, site_usecase.ErrProjectNotFound
	}
	rev, err := revisionRecord(m.Revision)
	if err != nil {
		return site_usecase.PublishMarked{}, err
	}
	var out site_usecase.PublishMarked
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := insertBlobs(tx, m.WorkspaceID, m.Blobs, m.Revision.UpdatedAt); err != nil {
			return err
		}
		if err := tx.Create(&rev).Error; err != nil {
			return err
		}
		var moved []schema.SiteProject
		if err := tx.Raw(markPublishedSQL, m.Revision.ID, m.ProjectID, m.WorkspaceID, m.SourceVersion, m.SourceRevisionID).Scan(&moved).Error; err != nil {
			return err
		}
		if len(moved) == 1 {
			p := toProject(moved[0])
			out = site_usecase.PublishMarked{Project: &p, Current: true}
			return nil
		}
		var row schema.SiteProject
		err := tx.Where("id = ? AND workspace_id = ?", m.ProjectID, m.WorkspaceID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return site_usecase.ErrProjectNotFound
		}
		if err != nil {
			return err
		}
		p := toProject(row)
		out = site_usecase.PublishMarked{Project: &p}
		return nil
	})
	if err != nil {
		return site_usecase.PublishMarked{}, err
	}
	return out, nil
}
