package site_usecase

import (
	"context"
	"time"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

type FileRev struct {
	Path   string
	Rev    uint64
	SHA256 string
}

type Commit struct {
	WorkspaceID     string
	ProjectID       string
	ExpectedVersion int64
	Blobs           []Blob
	Revision        Revision
	Replace         bool
	FileRevs        []FileRev
}

type NewProject struct {
	Project  Project
	Revision Revision
	Blobs    []Blob
	Settings StoredSettings
	FileRevs []FileRev
}

type ProjectStore interface {
	CreateProject(ctx context.Context, in NewProject) (*Project, error)
	GetProject(ctx context.Context, workspaceID, id string) (*Project, error)
	ListProjects(ctx context.Context, q ListQuery) ([]Project, int64, error)
	TakenSlugs(ctx context.Context, workspaceID, prefix string) ([]string, error)
	UpdateProject(ctx context.Context, p *Project, expectedVersion int64) error
	DeleteProject(ctx context.Context, workspaceID, id string) error
	SetPublication(ctx context.Context, workspaceID, id string, status Status, deploymentID string) (*Project, error)
}

type RevisionStore interface {
	Commit(ctx context.Context, c Commit) (*Project, error)
	GetRevision(ctx context.Context, workspaceID, projectID, revisionID string) (*Revision, error)
	ListRevisions(ctx context.Context, workspaceID, projectID string, limit, offset int) ([]Revision, int64, error)
	Blobs(ctx context.Context, workspaceID string, shas []string) (map[string][]byte, error)
	FileRevs(ctx context.Context, workspaceID, projectID string, wants map[string][]uint64) (map[string]map[uint64]string, error)
}

type SettingsStore interface {
	GetSettings(ctx context.Context, workspaceID, projectID string) (*StoredSettings, error)
	SaveSettings(ctx context.Context, s *StoredSettings, expectedVersion int64) error
}

type DesignSystemStore interface {
	CreateDesignSystem(ctx context.Context, d *DesignSystemRecord) error
	GetDesignSystem(ctx context.Context, workspaceID, id string) (*DesignSystemRecord, error)
	ListDesignSystems(ctx context.Context, workspaceID string) ([]DesignSystemRecord, error)
	UpdateDesignSystem(ctx context.Context, d *DesignSystemRecord, expectedVersion int64) error
	DeleteDesignSystem(ctx context.Context, workspaceID, id string) error
}

type Store interface {
	ProjectStore
	RevisionStore
	SettingsStore
	DesignSystemStore
}

type ExecuteRequest struct {
	WorkspaceID string
	ProjectID   string
	Project     site.Project
	Batch       site.Batch
	Bases       map[string][]site.File
}

type ExecuteResult struct {
	Steps    []site.StepPatch
	Failures []site.Failure
	Skipped  []site.Skip
}

type ExtractRequest struct {
	WorkspaceID string
	ProjectID   string
	Project     site.Project
	Node        string
	Rev         uint64
	Bases       map[string][]site.File
}

type ExtractResult struct {
	Source  string
	Failure *site.Failure
}

type Executor interface {
	Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error)
	Extract(ctx context.Context, req ExtractRequest) (ExtractResult, error)
}

type MediaLocator interface {
	Locate(ctx context.Context, workspaceID string, mediaIDs []string) (map[string]string, error)
}

type PreviewRequest struct {
	WorkspaceID string
	ProjectID   string
	RevisionID  string
	Project     site.Project
	Settings    site.Settings
}

type PreviewResult struct {
	URL        string
	RevisionID string
	Build      string
}

type Previewer interface {
	Preview(ctx context.Context, req PreviewRequest) (PreviewResult, error)
}

type PublishedRevisions interface {
	PublishedRevision(ctx context.Context, workspaceID, deploymentID string) (string, error)
}

type FormsRetirement interface {
	Deactivate(ctx context.Context, projectID string) error
}

type PublicationRetirement interface {
	Retire(ctx context.Context, workspaceID, projectID string) error
}

type WhatsAppNumbers interface {
	NumberOf(ctx context.Context, workspaceID, ref string) (string, error)
}

type HostnameLister interface {
	ListByProjects(ctx context.Context, workspaceID string, projectIDs []string) ([]sitepublish.HostnameRecord, error)
}

type PreviewCache interface {
	GetString(key string) (string, error)
	SetString(key string, value string, ttl time.Duration) error
}

type FormReferences interface {
	Check(ctx context.Context, workspaceID, formID string, form site.FormSettings) error
}
