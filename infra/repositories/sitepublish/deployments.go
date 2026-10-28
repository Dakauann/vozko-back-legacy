package sitepublish_repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/infra/database/schema"
)

const (
	supersedeSQL = `UPDATE site_deployments SET status = 'superseded' WHERE project_id = ? AND status = 'active' AND id <> ?`
	activateSQL  = `UPDATE site_deployments SET status = 'active', published_by = ?, published_at = ? WHERE id = ? AND project_id = ? AND status IN ('ready','superseded')`
)

type Deployments struct {
	db *gorm.DB
}

func NewDeployments(db *gorm.DB) *Deployments {
	return &Deployments{db: db}
}

func encodeJSON(v any, empty string) (datatypes.JSON, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if string(raw) == "null" {
		raw = []byte(empty)
	}
	return datatypes.JSON(raw), nil
}

func decodeJSON(raw datatypes.JSON, into any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, into)
}

func deploymentRow(rec sitepublish.DeploymentRecord) (schema.SiteDeployment, error) {
	row := schema.SiteDeployment{
		ID: rec.ID, WorkspaceID: rec.WorkspaceID, ProjectID: rec.ProjectID, RevisionID: rec.RevisionID,
		Status: string(rec.Status), PublishedBy: rec.PublishedBy, PublishedAt: rec.PublishedAt, CreatedAt: rec.CreatedAt,
	}
	var err error
	if row.Manifest, err = encodeJSON(rec.Files, "{}"); err != nil {
		return row, err
	}
	if row.Checks, err = encodeJSON(rec.Checks, "[]"); err != nil {
		return row, err
	}
	if row.Gaps, err = encodeJSON(rec.Gaps, "[]"); err != nil {
		return row, err
	}
	if row.Forms, err = encodeJSON(rec.Forms, "{}"); err != nil {
		return row, err
	}
	if row.Redirects, err = encodeJSON(rec.Redirects, "[]"); err != nil {
		return row, err
	}
	if row.Settings, err = encodeJSON(rec.Settings, "{}"); err != nil {
		return row, err
	}
	row.Outbound, err = encodeJSON(rec.Outbound, "[]")
	return row, err
}

func deploymentRecord(row schema.SiteDeployment) (sitepublish.DeploymentRecord, error) {
	rec := sitepublish.DeploymentRecord{
		ID: row.ID, WorkspaceID: row.WorkspaceID, ProjectID: row.ProjectID, RevisionID: row.RevisionID,
		Status: sitepublish.DeploymentStatus(row.Status), PublishedBy: row.PublishedBy, PublishedAt: row.PublishedAt, CreatedAt: row.CreatedAt,
	}
	if !rec.Status.Valid() {
		return rec, fmt.Errorf("site deployment %s has unknown status %q", row.ID, row.Status)
	}
	var checks []site.Check
	var gaps []site.Issue
	var redirects []site.Redirect
	for _, part := range []struct {
		raw  datatypes.JSON
		into any
	}{{row.Manifest, &rec.Files}, {row.Checks, &checks}, {row.Gaps, &gaps}, {row.Forms, &rec.Forms}, {row.Redirects, &redirects}, {row.Settings, &rec.Settings}, {row.Outbound, &rec.Outbound}} {
		if err := decodeJSON(part.raw, part.into); err != nil {
			return rec, fmt.Errorf("site deployment %s is unreadable: %w", row.ID, err)
		}
	}
	rec.Checks, rec.Gaps, rec.Redirects = checks, gaps, redirects
	return rec, nil
}

func (r *Deployments) Create(ctx context.Context, rec sitepublish.DeploymentRecord) error {
	row, err := deploymentRow(rec)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *Deployments) Update(ctx context.Context, rec sitepublish.DeploymentRecord) error {
	row, err := deploymentRow(rec)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&schema.SiteDeployment{}).Where("id = ? AND project_id = ?", rec.ID, rec.ProjectID).
		Select("revision_id", "status", "manifest", "checks", "gaps", "forms", "redirects", "settings", "outbound", "published_by", "published_at").
		Updates(&row)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return sitepublish.ErrDeploymentNotFound
	}
	return nil
}

func (r *Deployments) one(ctx context.Context, query string, args ...any) (sitepublish.DeploymentRecord, error) {
	var row schema.SiteDeployment
	err := r.db.WithContext(ctx).Where(query, args...).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sitepublish.DeploymentRecord{}, sitepublish.ErrDeploymentNotFound
	}
	if err != nil {
		return sitepublish.DeploymentRecord{}, err
	}
	return deploymentRecord(row)
}

func (r *Deployments) many(ctx context.Context, limit int, query string, args ...any) ([]sitepublish.DeploymentRecord, error) {
	var rows []schema.SiteDeployment
	q := r.db.WithContext(ctx).Where(query, args...).Order("created_at DESC, id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]sitepublish.DeploymentRecord, 0, len(rows))
	for _, row := range rows {
		rec, err := deploymentRecord(row)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func (r *Deployments) Get(ctx context.Context, workspaceID, id string) (sitepublish.DeploymentRecord, error) {
	return r.one(ctx, "id = ? AND workspace_id = ?", id, workspaceID)
}

func (r *Deployments) GetByID(ctx context.Context, id string) (sitepublish.DeploymentRecord, error) {
	return r.one(ctx, "id = ?", id)
}

func (r *Deployments) Active(ctx context.Context, projectID string) (sitepublish.DeploymentRecord, bool, error) {
	rec, err := r.one(ctx, "project_id = ? AND status = ?", projectID, string(sitepublish.DeploymentActive))
	if errors.Is(err, sitepublish.ErrDeploymentNotFound) {
		return sitepublish.DeploymentRecord{}, false, nil
	}
	return rec, err == nil, err
}

func (r *Deployments) ListByProject(ctx context.Context, workspaceID, projectID string, limit int) ([]sitepublish.DeploymentRecord, error) {
	return r.many(ctx, limit, "workspace_id = ? AND project_id = ?", workspaceID, projectID)
}

func (r *Deployments) ListActiveByWorkspace(ctx context.Context, workspaceID string) ([]sitepublish.DeploymentRecord, error) {
	return r.many(ctx, 0, "workspace_id = ? AND status = ?", workspaceID, string(sitepublish.DeploymentActive))
}

func (r *Deployments) Activate(ctx context.Context, projectID, deploymentID, publishedBy string, at time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(supersedeSQL, projectID, deploymentID).Error; err != nil {
			return err
		}
		res := tx.Exec(activateSQL, publishedBy, at, deploymentID, projectID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return fmt.Errorf("%w: %s is not ready", sitepublish.ErrDeploymentTransition, deploymentID)
		}
		return nil
	})
}

func (r *Deployments) Latest(ctx context.Context, projectID string) (sitepublish.DeploymentRecord, error) {
	list, err := r.many(ctx, 1, "project_id = ?", projectID)
	if err != nil {
		return sitepublish.DeploymentRecord{}, err
	}
	if len(list) == 0 {
		return sitepublish.DeploymentRecord{}, sitepublish.ErrDeploymentNotFound
	}
	return list[0], nil
}

func (r *Deployments) ListServable(ctx context.Context, projectID string) ([]sitepublish.DeploymentRecord, error) {
	return r.many(ctx, 0, "project_id = ? AND status IN ?", projectID, []string{string(sitepublish.DeploymentReady), string(sitepublish.DeploymentActive), string(sitepublish.DeploymentSuperseded)})
}
