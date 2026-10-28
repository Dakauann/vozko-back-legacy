package siteform_repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/conversation"
	"vozko/infra/database/schema"
	"vozko/usecases/siteforms"
)

const selectProjectWorkspaceSQL = `SELECT workspace_id FROM site_projects WHERE id = ?`

var (
	ErrReferenceCollision = errors.New("siteform repository: the page reference code is already taken, try again")
)

type ReferenceStore struct {
	db  *gorm.DB
	now func() time.Time
}

func NewReferenceStore(db *gorm.DB) *ReferenceStore {
	return &ReferenceStore{db: db, now: func() time.Time { return time.Now().UTC() }}
}

func referenceOf(row schema.SiteWhatsAppReference) siteforms.PageReference {
	return siteforms.PageReference{
		Key:  siteforms.PageReferenceKey{ProjectID: row.ProjectID, Route: row.Route, NumberRef: row.NumberRef, TextDigest: row.TextDigest},
		Code: row.Code, WorkspaceID: row.WorkspaceID, ShortLinkID: row.ShortLinkID, URL: row.URL,
	}
}

func (s *ReferenceStore) Find(ctx context.Context, key siteforms.PageReferenceKey) (*siteforms.PageReference, error) {
	var rows []schema.SiteWhatsAppReference
	err := s.db.WithContext(ctx).Where("project_id = ? AND route = ? AND number_ref = ? AND text_digest = ?", key.ProjectID, key.Route, key.NumberRef, key.TextDigest).
		Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	ref := referenceOf(rows[0])
	return &ref, nil
}

func (s *ReferenceStore) Create(ctx context.Context, ref siteforms.PageReference) (siteforms.PageReference, bool, error) {
	row := schema.SiteWhatsAppReference{
		Code: ref.Code, WorkspaceID: ref.WorkspaceID, ProjectID: ref.Key.ProjectID, Route: ref.Key.Route, NumberRef: ref.Key.NumberRef,
		TextDigest: ref.Key.TextDigest, ShortLinkID: ref.ShortLinkID, URL: ref.URL, CreatedAt: s.now(),
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return siteforms.PageReference{}, false, result.Error
	}
	if result.RowsAffected == 1 {
		return ref, true, nil
	}
	existing, err := s.Find(ctx, ref.Key)
	if err != nil {
		return siteforms.PageReference{}, false, err
	}
	if existing == nil {
		return siteforms.PageReference{}, false, ErrReferenceCollision
	}
	return *existing, false, nil
}

func (s *ReferenceStore) Target(ctx context.Context, code string) (*conversation.PageReferenceTarget, error) {
	var rows []schema.SiteWhatsAppReference
	if err := s.db.WithContext(ctx).Where("code = ?", code).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return nil, err
	}
	row := rows[0]
	return &conversation.PageReferenceTarget{WorkspaceID: row.WorkspaceID, ProjectID: row.ProjectID, Route: row.Route, NumberRef: row.NumberRef}, nil
}

type ProjectDirectory struct {
	db *gorm.DB
}

func NewProjectDirectory(db *gorm.DB) *ProjectDirectory {
	return &ProjectDirectory{db: db}
}

func (d *ProjectDirectory) WorkspaceOf(ctx context.Context, projectID string) (string, error) {
	var workspaces []string
	if err := d.db.WithContext(ctx).Raw(selectProjectWorkspaceSQL, projectID).Scan(&workspaces).Error; err != nil {
		return "", err
	}
	if len(workspaces) == 0 || workspaces[0] == "" {
		return "", siteforms.ErrProjectNotFound
	}
	return workspaces[0], nil
}
