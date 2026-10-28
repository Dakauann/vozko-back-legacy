package sitepublish_repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"vozko/domain/sitepublish"
	"vozko/infra/database"
	"vozko/infra/database/schema"
)

const (
	recordPublishedSQL = `UPDATE site_deployments SET revision_id = ? WHERE id = ? AND project_id = ?`
	liveProjectsSQL    = `SELECT d.workspace_id, d.project_id, d.id AS deployment_id, COALESCE(p.status, '') AS project_status,
		EXISTS (SELECT 1 FROM site_abuse_cases a WHERE a.project_id = d.project_id AND a.status = 'suspended') AS abuse_suspended
		FROM site_deployments d LEFT JOIN site_projects p ON p.id = d.project_id
		WHERE d.status = 'active' AND d.project_id::text > ? ORDER BY d.project_id::text LIMIT ?`
	domainLockSQL         = `SELECT pg_advisory_xact_lock(hashtext(?))`
	customWorkspacesSQL   = `SELECT DISTINCT workspace_id::text FROM site_hostnames WHERE kind = 'custom' AND status <> 'removing' AND workspace_id::text > ? ORDER BY 1 LIMIT ?`
	domainLockKeyPrefix   = "site_domains:"
	customByWorkspaceSort = "created_at, id"
)

func (r *Deployments) RecordPublished(ctx context.Context, projectID, deploymentID, revisionID string) error {
	res := r.db.WithContext(ctx).Exec(recordPublishedSQL, revisionID, deploymentID, projectID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return sitepublish.ErrDeploymentNotFound
	}
	return nil
}

func (r *Deployments) LiveProjects(ctx context.Context, afterProjectID string, limit int) ([]sitepublish.LiveProject, error) {
	var rows []struct {
		WorkspaceID    string
		ProjectID      string
		DeploymentID   string
		ProjectStatus  string
		AbuseSuspended bool
	}
	if err := r.db.WithContext(ctx).Raw(liveProjectsSQL, afterProjectID, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]sitepublish.LiveProject, len(rows))
	for i, row := range rows {
		out[i] = sitepublish.LiveProject{WorkspaceID: row.WorkspaceID, ProjectID: row.ProjectID, DeploymentID: row.DeploymentID, ProjectStatus: row.ProjectStatus, AbuseSuspended: row.AbuseSuspended}
	}
	return out, nil
}

func (r *Hostnames) CreateCustom(ctx context.Context, rec sitepublish.HostnameRecord, allowed int) error {
	row, err := hostnameRow(rec)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(domainLockSQL, domainLockKeyPrefix+rec.WorkspaceID).Error; err != nil {
			return err
		}
		var used int64
		if err := tx.Raw(countCustomSQL, rec.WorkspaceID).Scan(&used).Error; err != nil {
			return err
		}
		if used >= int64(allowed) {
			return sitepublish.ErrDomainQuota
		}
		if err := tx.Create(&row).Error; err != nil {
			if database.IsUniqueViolation(err) {
				return fmt.Errorf("%w: %s", sitepublish.ErrHostnameTaken, rec.Hostname)
			}
			return err
		}
		return nil
	})
}

func (r *Hostnames) ListCustomByWorkspace(ctx context.Context, workspaceID string) ([]sitepublish.HostnameRecord, error) {
	var rows []schema.SiteHostname
	if err := r.db.WithContext(ctx).Where("workspace_id = ? AND kind = ?", workspaceID, string(sitepublish.HostnameCustom)).Order(customByWorkspaceSort).Find(&rows).Error; err != nil {
		return nil, err
	}
	return hostnameRecords(rows)
}

func (r *Hostnames) CustomWorkspaces(ctx context.Context, afterWorkspaceID string, limit int) ([]string, error) {
	var ids []string
	if err := r.db.WithContext(ctx).Raw(customWorkspacesSQL, afterWorkspaceID, limit).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}
