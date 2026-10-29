package conversation_repository

import (
	"context"

	"gorm.io/gorm"

	"vozko/domain/conversation"
	sitepublish_usecase "vozko/usecases/sitepublish"
)

const selectPageProjectSQL = `SELECT name, status = ? AS live FROM site_projects WHERE id = ? AND workspace_id = ?`

type PageProjects struct {
	db *gorm.DB
}

func NewPageProjects(db *gorm.DB) *PageProjects {
	return &PageProjects{db: db}
}

func (p *PageProjects) PageProject(ctx context.Context, workspaceID, projectID string) (*conversation.PageProject, error) {
	var rows []conversation.PageProject
	if err := p.db.WithContext(ctx).Raw(selectPageProjectSQL, string(sitepublish_usecase.PublicationLive), projectID, workspaceID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}
