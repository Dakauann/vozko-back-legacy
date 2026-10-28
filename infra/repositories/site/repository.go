package site_repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/infra/database"
	"vozko/infra/database/schema"
	site_usecase "vozko/usecases/site"
)

const (
	slugConstraint   = "ux_site_projects_slug"
	originConstraint = "ux_site_projects_origin"
	blobBatchSize    = 100
)

type Repository struct {
	db *gorm.DB
}

var _ site_usecase.Store = (*Repository)(nil)

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func validID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func projectRecord(p site_usecase.Project) schema.SiteProject {
	return schema.SiteProject{
		ID: p.ID, WorkspaceID: p.WorkspaceID, Name: p.Name, Slug: p.Slug, Stack: string(p.Stack), Status: string(p.Status),
		WorkingRevisionID: p.WorkingRevisionID, PublishedDeploymentID: optional(p.PublishedDeploymentID), DesignSystemID: optional(p.DesignSystemID), ThumbnailMediaID: optional(p.ThumbnailMediaID),
		ChatThreadID: optional(p.Origin.ThreadID), ScreenCommandID: optional(p.Origin.CommandID), Version: p.Version, CreatedBy: p.CreatedBy,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func toProject(r schema.SiteProject) site_usecase.Project {
	return site_usecase.Project{
		ID: r.ID, WorkspaceID: r.WorkspaceID, Name: r.Name, Slug: r.Slug, Stack: site.Stack(r.Stack), Status: site_usecase.Status(r.Status),
		WorkingRevisionID: r.WorkingRevisionID, PublishedDeploymentID: deref(r.PublishedDeploymentID), DesignSystemID: deref(r.DesignSystemID), ThumbnailMediaID: deref(r.ThumbnailMediaID),
		Origin: site_usecase.Origin{ThreadID: deref(r.ChatThreadID), CommandID: deref(r.ScreenCommandID)}, Version: r.Version, CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func revisionRecord(r site_usecase.Revision) (schema.SiteRevision, error) {
	manifest, err := json.Marshal(r.Manifest)
	if err != nil {
		return schema.SiteRevision{}, err
	}
	return schema.SiteRevision{
		ID: r.ID, WorkspaceID: r.WorkspaceID, ProjectID: r.ProjectID, ParentID: optional(r.ParentID), Manifest: manifest,
		Author: string(r.Author), Label: r.Label, Kind: string(r.Kind), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, nil
}

func toRevision(r schema.SiteRevision) (site_usecase.Revision, error) {
	out := site_usecase.Revision{
		ID: r.ID, WorkspaceID: r.WorkspaceID, ProjectID: r.ProjectID, ParentID: deref(r.ParentID), Author: site_usecase.Author(r.Author),
		Label: r.Label, Kind: site_usecase.RevisionKind(r.Kind), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if len(r.Manifest) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(r.Manifest, &out.Manifest); err != nil {
		return site_usecase.Revision{}, err
	}
	return out, nil
}

func insertBlobs(tx *gorm.DB, workspaceID string, blobs []site_usecase.Blob, now time.Time) error {
	if len(blobs) == 0 {
		return nil
	}
	records := make([]schema.SiteBlob, 0, len(blobs))
	for _, b := range blobs {
		records = append(records, schema.SiteBlob{WorkspaceID: workspaceID, SHA256: b.SHA256, Bytes: b.Bytes, Size: len(b.Bytes), CreatedAt: now})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&records, blobBatchSize).Error
}

func (r *Repository) CreateProject(ctx context.Context, in site_usecase.NewProject) (*site_usecase.Project, error) {
	if !validID(in.Project.WorkspaceID) {
		return nil, site_usecase.ErrWorkspaceRequired
	}
	if !in.Project.Origin.Empty() {
		existing, err := r.byOrigin(ctx, in.Project)
		if err != nil || existing != nil {
			return existing, err
		}
	}
	rev, err := revisionRecord(in.Revision)
	if err != nil {
		return nil, err
	}
	settings, err := json.Marshal(in.Settings.Settings)
	if err != nil {
		return nil, err
	}
	project := projectRecord(in.Project)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}
		if err := tx.Create(&rev).Error; err != nil {
			return err
		}
		if err := insertBlobs(tx, in.Project.WorkspaceID, in.Blobs, in.Project.CreatedAt); err != nil {
			return err
		}
		if err := insertFileRevs(tx, in.Project.WorkspaceID, in.Project.ID, in.FileRevs, in.Project.CreatedAt); err != nil {
			return err
		}
		return tx.Create(&schema.SiteSettings{ProjectID: in.Project.ID, WorkspaceID: in.Project.WorkspaceID, Settings: settings, Version: in.Settings.Version, UpdatedAt: in.Settings.UpdatedAt}).Error
	})
	switch {
	case database.IsUniqueViolationOf(err, slugConstraint):
		return nil, site_usecase.ErrSlugTaken
	case database.IsUniqueViolationOf(err, originConstraint):
		existing, lookupErr := r.byOrigin(ctx, in.Project)
		if lookupErr != nil || existing != nil {
			return existing, lookupErr
		}
		return nil, site_usecase.ErrOriginTaken
	case err != nil:
		return nil, err
	}
	out := toProject(project)
	return &out, nil
}

func (r *Repository) byOrigin(ctx context.Context, p site_usecase.Project) (*site_usecase.Project, error) {
	var records []schema.SiteProject
	err := r.db.WithContext(ctx).Where("chat_thread_id = ? AND screen_command_id = ?", p.Origin.ThreadID, p.Origin.CommandID).Limit(1).Find(&records).Error
	if err != nil || len(records) == 0 {
		return nil, err
	}
	existing := records[0]
	if existing.WorkspaceID != p.WorkspaceID || existing.CreatedBy != p.CreatedBy {
		return nil, site_usecase.ErrOriginTaken
	}
	out := toProject(existing)
	return &out, nil
}

func (r *Repository) GetProject(ctx context.Context, workspaceID, id string) (*site_usecase.Project, error) {
	if !validID(workspaceID) || !validID(id) {
		return nil, site_usecase.ErrProjectNotFound
	}
	var records []schema.SiteProject
	if err := r.db.WithContext(ctx).Where("workspace_id = ? AND id = ?", workspaceID, id).Limit(1).Find(&records).Error; err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, site_usecase.ErrProjectNotFound
	}
	out := toProject(records[0])
	return &out, nil
}

func likePattern(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(text) + "%"
}

func (r *Repository) ListProjects(ctx context.Context, q site_usecase.ListQuery) ([]site_usecase.Project, int64, error) {
	if !validID(q.WorkspaceID) {
		return nil, 0, site_usecase.ErrWorkspaceRequired
	}
	scope := r.db.WithContext(ctx).Model(&schema.SiteProject{}).Where("workspace_id = ?", q.WorkspaceID)
	if q.Status == "" {
		scope = scope.Where("status <> ?", string(site_usecase.StatusArchived))
	} else {
		scope = scope.Where("status = ?", string(q.Status))
	}
	if q.Search != "" {
		scope = scope.Where("name ILIKE ?", likePattern(q.Search))
	}
	var total int64
	if err := scope.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []schema.SiteProject
	if err := scope.Order("updated_at DESC, id DESC").Limit(q.Limit).Offset(q.Offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	out := make([]site_usecase.Project, 0, len(records))
	for _, rec := range records {
		out = append(out, toProject(rec))
	}
	return out, total, nil
}

func (r *Repository) TakenSlugs(ctx context.Context, workspaceID, prefix string) ([]string, error) {
	if !validID(workspaceID) {
		return nil, site_usecase.ErrWorkspaceRequired
	}
	var slugs []string
	err := r.db.WithContext(ctx).Model(&schema.SiteProject{}).Where("workspace_id = ? AND slug LIKE ?", workspaceID, strings.TrimSuffix(likePattern(prefix), "%")+"%").
		Pluck("slug", &slugs).Error
	return slugs, err
}

func (r *Repository) missingOrConflict(ctx context.Context, workspaceID, id string) error {
	p, err := r.GetProject(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if p.Status == site_usecase.StatusArchived {
		return site_usecase.ErrProjectArchived
	}
	return shared.ErrVersionConflict
}

func (r *Repository) UpdateProject(ctx context.Context, p *site_usecase.Project, expected int64) error {
	if !validID(p.WorkspaceID) || !validID(p.ID) {
		return site_usecase.ErrProjectNotFound
	}
	now := time.Now().UTC()
	var saved []schema.SiteProject
	err := r.db.WithContext(ctx).Raw(`UPDATE site_projects SET name = ?, slug = ?, status = CASE WHEN status IN ('published','suspended') THEN status ELSE ? END, design_system_id = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND workspace_id = ? AND version = ? AND NOT (status IN ('published','suspended') AND (? = 'archived' OR slug <> ?)) RETURNING *`,
		p.Name, p.Slug, string(p.Status), optional(p.DesignSystemID), now, p.ID, p.WorkspaceID, expected, string(p.Status), p.Slug).Scan(&saved).Error
	if database.IsUniqueViolationOf(err, slugConstraint) {
		return site_usecase.ErrSlugTaken
	}
	if err != nil {
		return err
	}
	if len(saved) == 0 {
		if _, err := r.GetProject(ctx, p.WorkspaceID, p.ID); err != nil {
			return err
		}
		return shared.ErrVersionConflict
	}
	*p = toProject(saved[0])
	return nil
}

func (r *Repository) DeleteProject(ctx context.Context, workspaceID, id string) error {
	if !validID(workspaceID) || !validID(id) {
		return site_usecase.ErrProjectNotFound
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`DELETE FROM site_projects WHERE id = ? AND workspace_id = ?`, id, workspaceID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return site_usecase.ErrProjectNotFound
		}
		if err := tx.Exec(`DELETE FROM site_revisions WHERE project_id = ? AND workspace_id = ?`, id, workspaceID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM site_file_revs WHERE project_id = ? AND workspace_id = ?`, id, workspaceID).Error; err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM site_settings WHERE project_id = ? AND workspace_id = ?`, id, workspaceID).Error
	})
}

var errStale = errors.New("site: the page changed while saving")

func (r *Repository) Commit(ctx context.Context, c site_usecase.Commit) (*site_usecase.Project, error) {
	if !validID(c.WorkspaceID) || !validID(c.ProjectID) {
		return nil, site_usecase.ErrProjectNotFound
	}
	rev, err := revisionRecord(c.Revision)
	if err != nil {
		return nil, err
	}
	var saved []schema.SiteProject
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Raw(`UPDATE site_projects SET working_revision_id = ?, version = version + 1, updated_at = ?
			WHERE id = ? AND workspace_id = ? AND version = ? AND status <> ? RETURNING *`,
			c.Revision.ID, c.Revision.UpdatedAt, c.ProjectID, c.WorkspaceID, c.ExpectedVersion, string(site_usecase.StatusArchived)).Scan(&saved).Error
		if err != nil {
			return err
		}
		if len(saved) == 0 {
			return errStale
		}
		if err := insertBlobs(tx, c.WorkspaceID, c.Blobs, c.Revision.UpdatedAt); err != nil {
			return err
		}
		if err := writeRevision(tx, c, rev); err != nil {
			return err
		}
		return insertFileRevs(tx, c.WorkspaceID, c.ProjectID, c.FileRevs, c.Revision.UpdatedAt)
	})
	if errors.Is(err, errStale) {
		return nil, r.missingOrConflict(ctx, c.WorkspaceID, c.ProjectID)
	}
	if err != nil {
		return nil, err
	}
	out := toProject(saved[0])
	return &out, nil
}

func writeRevision(tx *gorm.DB, c site_usecase.Commit, rev schema.SiteRevision) error {
	if !c.Replace {
		return tx.Create(&rev).Error
	}
	result := tx.Exec(`UPDATE site_revisions SET manifest = ?, updated_at = ? WHERE id = ? AND project_id = ? AND workspace_id = ? AND kind = ?`,
		rev.Manifest, rev.UpdatedAt, rev.ID, c.ProjectID, c.WorkspaceID, string(site_usecase.RevisionAutosave))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return site_usecase.ErrRevisionNotFound
	}
	return nil
}

func (r *Repository) GetRevision(ctx context.Context, workspaceID, projectID, revisionID string) (*site_usecase.Revision, error) {
	if !validID(workspaceID) || !validID(projectID) || !validID(revisionID) {
		return nil, site_usecase.ErrRevisionNotFound
	}
	var records []schema.SiteRevision
	err := r.db.WithContext(ctx).Where("workspace_id = ? AND project_id = ? AND id = ?", workspaceID, projectID, revisionID).Limit(1).Find(&records).Error
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, site_usecase.ErrRevisionNotFound
	}
	out, err := toRevision(records[0])
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *Repository) ListRevisions(ctx context.Context, workspaceID, projectID string, limit, offset int) ([]site_usecase.Revision, int64, error) {
	if !validID(workspaceID) || !validID(projectID) {
		return nil, 0, site_usecase.ErrProjectNotFound
	}
	scope := r.db.WithContext(ctx).Model(&schema.SiteRevision{}).Where("workspace_id = ? AND project_id = ?", workspaceID, projectID)
	var total int64
	if err := scope.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []schema.SiteRevision
	err := scope.Select("id", "workspace_id", "project_id", "parent_id", "author", "label", "kind", "created_at", "updated_at").
		Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&records).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]site_usecase.Revision, 0, len(records))
	for _, rec := range records {
		rev, err := toRevision(rec)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, rev)
	}
	return out, total, nil
}

func (r *Repository) Blobs(ctx context.Context, workspaceID string, shas []string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(shas))
	if len(shas) == 0 {
		return out, nil
	}
	if !validID(workspaceID) {
		return nil, site_usecase.ErrWorkspaceRequired
	}
	var records []schema.SiteBlob
	if err := r.db.WithContext(ctx).Select("sha256", "bytes").Where("workspace_id = ? AND sha256 IN ?", workspaceID, shas).Find(&records).Error; err != nil {
		return nil, err
	}
	for _, rec := range records {
		out[rec.SHA256] = rec.Bytes
	}
	return out, nil
}

func (r *Repository) GetSettings(ctx context.Context, workspaceID, projectID string) (*site_usecase.StoredSettings, error) {
	if !validID(workspaceID) || !validID(projectID) {
		return nil, site_usecase.ErrProjectNotFound
	}
	var records []schema.SiteSettings
	if err := r.db.WithContext(ctx).Where("workspace_id = ? AND project_id = ?", workspaceID, projectID).Limit(1).Find(&records).Error; err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, site_usecase.ErrProjectNotFound
	}
	settings, err := site.DecodeSettings(records[0].Settings)
	if err != nil {
		return nil, err
	}
	return &site_usecase.StoredSettings{ProjectID: projectID, WorkspaceID: workspaceID, Settings: settings, Version: records[0].Version, UpdatedAt: records[0].UpdatedAt}, nil
}

func (r *Repository) SaveSettings(ctx context.Context, s *site_usecase.StoredSettings, expected int64) error {
	if !validID(s.WorkspaceID) || !validID(s.ProjectID) {
		return site_usecase.ErrProjectNotFound
	}
	raw, err := json.Marshal(s.Settings)
	if err != nil {
		return err
	}
	var saved []schema.SiteSettings
	err = r.db.WithContext(ctx).Raw(`UPDATE site_settings SET settings = ?, version = version + 1, updated_at = ?
		WHERE project_id = ? AND workspace_id = ? AND version = ? RETURNING *`, raw, s.UpdatedAt, s.ProjectID, s.WorkspaceID, expected).Scan(&saved).Error
	if err != nil {
		return err
	}
	if len(saved) == 0 {
		if _, err := r.GetSettings(ctx, s.WorkspaceID, s.ProjectID); err != nil {
			return err
		}
		return shared.ErrVersionConflict
	}
	s.Version, s.UpdatedAt = saved[0].Version, saved[0].UpdatedAt
	return nil
}

func (r *Repository) SetPublication(ctx context.Context, workspaceID, id string, status site_usecase.Status, deploymentID string) (*site_usecase.Project, error) {
	if !validID(workspaceID) || !validID(id) {
		return nil, site_usecase.ErrProjectNotFound
	}
	var saved []schema.SiteProject
	err := r.db.WithContext(ctx).Raw(`UPDATE site_projects SET status = ?, published_deployment_id = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ? AND status <> ? RETURNING *`,
		string(status), optional(deploymentID), time.Now().UTC(), id, workspaceID, string(site_usecase.StatusArchived)).Scan(&saved).Error
	if err != nil {
		return nil, err
	}
	if len(saved) == 0 {
		if _, err := r.GetProject(ctx, workspaceID, id); err != nil {
			return nil, err
		}
		return nil, site_usecase.ErrProjectArchived
	}
	out := toProject(saved[0])
	return &out, nil
}
