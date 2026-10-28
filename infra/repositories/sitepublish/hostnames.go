package sitepublish_repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"vozko/domain/sitepublish"
	"vozko/infra/database"
	"vozko/infra/database/schema"
)

const (
	activeHostnamesSQL    = `SELECT hostname FROM site_hostnames WHERE project_id = ? AND status = ? ORDER BY hostname`
	countCustomSQL        = `SELECT count(*) FROM site_hostnames WHERE workspace_id = ? AND kind = 'custom' AND status <> 'removing'`
	hostnamePageFilter    = "status IN ? AND kind = ? AND id::text > ?"
	holdingProviderFilter = "cloudflare_ref <> ''"
	hostnamePageOrder     = "id::text"
)

var errUnboundedPage = errors.New("sitepublish: a hostname page needs a positive limit")

type Hostnames struct {
	db *gorm.DB
}

func NewHostnames(db *gorm.DB) *Hostnames {
	return &Hostnames{db: db}
}

func hostnameRow(rec sitepublish.HostnameRecord) (schema.SiteHostname, error) {
	verification, err := encodeJSON(rec.Verification, "{}")
	return schema.SiteHostname{
		ID: rec.ID, WorkspaceID: rec.WorkspaceID, ProjectID: rec.ProjectID, Hostname: rec.Hostname, Kind: string(rec.Kind),
		Status: string(rec.Status), CloudflareRef: rec.CloudflareRef, Verification: verification, CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
	}, err
}

func hostnameRecord(row schema.SiteHostname) (sitepublish.HostnameRecord, error) {
	rec := sitepublish.HostnameRecord{
		ID: row.ID, WorkspaceID: row.WorkspaceID, ProjectID: row.ProjectID, Hostname: row.Hostname, Kind: sitepublish.HostnameKind(row.Kind),
		Status: sitepublish.HostnameStatus(row.Status), CloudflareRef: row.CloudflareRef, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if !rec.Kind.Valid() || !rec.Status.Valid() {
		return rec, fmt.Errorf("site hostname %s has unknown kind or status", row.ID)
	}
	if err := decodeJSON(row.Verification, &rec.Verification); err != nil {
		return rec, fmt.Errorf("site hostname %s is unreadable: %w", row.ID, err)
	}
	return rec, nil
}

func (r *Hostnames) Create(ctx context.Context, rec sitepublish.HostnameRecord) error {
	row, err := hostnameRow(rec)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		if database.IsUniqueViolation(err) {
			return fmt.Errorf("%w: %s", sitepublish.ErrHostnameTaken, rec.Hostname)
		}
		return err
	}
	return nil
}

func (r *Hostnames) Update(ctx context.Context, rec sitepublish.HostnameRecord) error {
	row, err := hostnameRow(rec)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&schema.SiteHostname{}).Where("id = ?", rec.ID).
		Select("status", "cloudflare_ref", "verification", "updated_at").Updates(&row)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return sitepublish.ErrHostnameNotFound
	}
	return nil
}

func (r *Hostnames) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&schema.SiteHostname{}).Error
}

func (r *Hostnames) list(ctx context.Context, limit int, query string, args ...any) ([]sitepublish.HostnameRecord, error) {
	var rows []schema.SiteHostname
	q := r.db.WithContext(ctx).Where(query, args...).Order("updated_at, id")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return hostnameRecords(rows)
}

func hostnameRecords(rows []schema.SiteHostname) ([]sitepublish.HostnameRecord, error) {
	out := make([]sitepublish.HostnameRecord, 0, len(rows))
	for _, row := range rows {
		rec, err := hostnameRecord(row)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func statusNames(statuses []sitepublish.HostnameStatus) []string {
	names := make([]string, len(statuses))
	for i, s := range statuses {
		names[i] = string(s)
	}
	return names
}

func (r *Hostnames) Get(ctx context.Context, workspaceID, id string) (sitepublish.HostnameRecord, error) {
	var row schema.SiteHostname
	err := r.db.WithContext(ctx).Where("id = ? AND workspace_id = ?", id, workspaceID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sitepublish.HostnameRecord{}, sitepublish.ErrHostnameNotFound
	}
	if err != nil {
		return sitepublish.HostnameRecord{}, err
	}
	return hostnameRecord(row)
}

func (r *Hostnames) ListByProject(ctx context.Context, workspaceID, projectID string) ([]sitepublish.HostnameRecord, error) {
	return r.list(ctx, 0, "workspace_id = ? AND project_id = ?", workspaceID, projectID)
}

func (r *Hostnames) ListByProjects(ctx context.Context, workspaceID string, projectIDs []string) ([]sitepublish.HostnameRecord, error) {
	if len(projectIDs) == 0 {
		return nil, nil
	}
	return r.list(ctx, 0, "workspace_id = ? AND project_id IN ?", workspaceID, projectIDs)
}

func (r *Hostnames) ListByStatus(ctx context.Context, statuses []sitepublish.HostnameStatus, kind sitepublish.HostnameKind, limit int) ([]sitepublish.HostnameRecord, error) {
	return r.list(ctx, limit, "status IN ? AND kind = ?", statusNames(statuses), string(kind))
}

func (r *Hostnames) ListPage(ctx context.Context, page sitepublish.HostnamePage) ([]sitepublish.HostnameRecord, error) {
	if page.Limit <= 0 {
		return nil, errUnboundedPage
	}
	q := r.db.WithContext(ctx).Where(hostnamePageFilter, statusNames(page.Statuses), string(page.Kind), page.AfterID)
	if page.HoldingProviderRecord {
		q = q.Where(holdingProviderFilter)
	}
	var rows []schema.SiteHostname
	if err := q.Order(hostnamePageOrder).Limit(page.Limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return hostnameRecords(rows)
}

func (r *Hostnames) ActiveHostnames(ctx context.Context, projectID string) ([]string, error) {
	var hosts []string
	if err := r.db.WithContext(ctx).Raw(activeHostnamesSQL, projectID, string(sitepublish.HostnameActive)).Scan(&hosts).Error; err != nil {
		return nil, err
	}
	return hosts, nil
}

func (r *Hostnames) CountCustom(ctx context.Context, workspaceID string) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Raw(countCustomSQL, workspaceID).Scan(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}
