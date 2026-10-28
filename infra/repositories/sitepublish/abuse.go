package sitepublish_repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"vozko/domain/sitepublish"
	"vozko/infra/database/schema"
)

const suspendedSQL = `SELECT count(*) FROM site_abuse_cases WHERE project_id = ? AND status = ?`

type AbuseCases struct {
	db *gorm.DB
}

func NewAbuseCases(db *gorm.DB) *AbuseCases {
	return &AbuseCases{db: db}
}

func abuseRow(c sitepublish.AbuseCase) schema.SiteAbuseCase {
	return schema.SiteAbuseCase{
		ID: c.ID, WorkspaceID: c.WorkspaceID, ProjectID: c.ProjectID, DeploymentID: c.DeploymentID, Source: string(c.Source),
		Status: string(c.Status), Reason: c.Reason, DecidedBy: c.DecidedBy, DecidedAt: c.DecidedAt, CreatedAt: c.CreatedAt,
	}
}

func abuseCase(row schema.SiteAbuseCase) sitepublish.AbuseCase {
	return sitepublish.AbuseCase{
		ID: row.ID, WorkspaceID: row.WorkspaceID, ProjectID: row.ProjectID, DeploymentID: row.DeploymentID, Source: sitepublish.AbuseSource(row.Source),
		Status: sitepublish.AbuseStatus(row.Status), Reason: row.Reason, DecidedBy: row.DecidedBy, DecidedAt: row.DecidedAt, CreatedAt: row.CreatedAt,
	}
}

func (r *AbuseCases) Create(ctx context.Context, c sitepublish.AbuseCase) error {
	row := abuseRow(c)
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *AbuseCases) Update(ctx context.Context, c sitepublish.AbuseCase) error {
	row := abuseRow(c)
	res := r.db.WithContext(ctx).Model(&schema.SiteAbuseCase{}).Where("id = ?", c.ID).
		Select("status", "reason", "decided_by", "decided_at").Updates(&row)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return sitepublish.ErrAbuseCaseNotFound
	}
	return nil
}

func (r *AbuseCases) Get(ctx context.Context, id string) (sitepublish.AbuseCase, error) {
	var row schema.SiteAbuseCase
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sitepublish.AbuseCase{}, sitepublish.ErrAbuseCaseNotFound
	}
	if err != nil {
		return sitepublish.AbuseCase{}, err
	}
	return abuseCase(row), nil
}

func (r *AbuseCases) List(ctx context.Context, statuses []sitepublish.AbuseStatus, limit int) ([]sitepublish.AbuseCase, error) {
	q := r.db.WithContext(ctx).Order("created_at DESC, id DESC")
	if len(statuses) > 0 {
		names := make([]string, len(statuses))
		for i, s := range statuses {
			names[i] = string(s)
		}
		q = q.Where("status IN ?", names)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []schema.SiteAbuseCase
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]sitepublish.AbuseCase, len(rows))
	for i, row := range rows {
		out[i] = abuseCase(row)
	}
	return out, nil
}

func (r *AbuseCases) OpenForProject(ctx context.Context, projectID string) ([]sitepublish.AbuseCase, error) {
	var rows []schema.SiteAbuseCase
	err := r.db.WithContext(ctx).Where("project_id = ? AND status IN ?", projectID, []string{string(sitepublish.AbuseHeld), string(sitepublish.AbuseSuspended)}).
		Order("created_at, id").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]sitepublish.AbuseCase, len(rows))
	for i, row := range rows {
		out[i] = abuseCase(row)
	}
	return out, nil
}

func (r *AbuseCases) Suspended(ctx context.Context, projectID string) (bool, error) {
	var n int64
	if err := r.db.WithContext(ctx).Raw(suspendedSQL, projectID, string(sitepublish.AbuseSuspended)).Scan(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}
