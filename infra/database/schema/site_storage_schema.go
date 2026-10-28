package schema

import (
	"time"

	"gorm.io/datatypes"
)

type SiteProject struct {
	ID                    string    `gorm:"primaryKey;type:uuid"`
	WorkspaceID           string    `gorm:"type:uuid;not null;uniqueIndex:ux_site_projects_slug,priority:1;index:idx_site_projects_listing,priority:1"`
	Name                  string    `gorm:"type:varchar(120);not null"`
	Slug                  string    `gorm:"type:varchar(63);not null;uniqueIndex:ux_site_projects_slug,priority:2"`
	Stack                 string    `gorm:"type:varchar(8);not null"`
	Status                string    `gorm:"type:varchar(16);not null;default:'draft';index:idx_site_projects_listing,priority:2"`
	WorkingRevisionID     string    `gorm:"type:uuid;not null"`
	PublishedDeploymentID *string   `gorm:"type:uuid"`
	DesignSystemID        *string   `gorm:"type:uuid;index"`
	ThumbnailMediaID      *string   `gorm:"type:varchar(64)"`
	ChatThreadID          *string   `gorm:"type:varchar(64);uniqueIndex:ux_site_projects_origin,priority:1,where:chat_thread_id IS NOT NULL AND screen_command_id IS NOT NULL"`
	ScreenCommandID       *string   `gorm:"type:uuid;uniqueIndex:ux_site_projects_origin,priority:2,where:chat_thread_id IS NOT NULL AND screen_command_id IS NOT NULL"`
	Version               int64     `gorm:"not null;default:1"`
	CreatedBy             string    `gorm:"type:uuid;not null"`
	CreatedAt             time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt             time.Time `gorm:"type:timestamptz;not null;index:idx_site_projects_listing,priority:3,sort:desc"`
}

const SiteProjectOriginPredicate = "chat_thread_id IS NOT NULL AND screen_command_id IS NOT NULL"

func (SiteProject) TableName() string { return "site_projects" }

type SiteBlob struct {
	WorkspaceID string    `gorm:"primaryKey;type:uuid"`
	SHA256      string    `gorm:"primaryKey;column:sha256;type:varchar(64)"`
	Bytes       []byte    `gorm:"type:bytea;not null"`
	Size        int       `gorm:"not null"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null"`
}

func (SiteBlob) TableName() string { return "site_blobs" }

type SiteRevision struct {
	ID          string         `gorm:"primaryKey;type:uuid"`
	WorkspaceID string         `gorm:"type:uuid;not null"`
	ProjectID   string         `gorm:"type:uuid;not null;index:idx_site_revisions_project,priority:1"`
	ParentID    *string        `gorm:"type:uuid"`
	Manifest    datatypes.JSON `gorm:"type:jsonb;not null"`
	Author      string         `gorm:"type:varchar(80);not null"`
	Label       string         `gorm:"type:varchar(120);not null;default:''"`
	Kind        string         `gorm:"type:varchar(16);not null"`
	CreatedAt   time.Time      `gorm:"type:timestamptz;not null;index:idx_site_revisions_project,priority:2,sort:desc"`
	UpdatedAt   time.Time      `gorm:"type:timestamptz;not null"`
}

func (SiteRevision) TableName() string { return "site_revisions" }

type SiteSettings struct {
	ProjectID   string         `gorm:"primaryKey;type:uuid"`
	WorkspaceID string         `gorm:"type:uuid;not null"`
	Settings    datatypes.JSON `gorm:"type:jsonb;not null"`
	Version     int64          `gorm:"not null;default:1"`
	UpdatedAt   time.Time      `gorm:"type:timestamptz;not null"`
}

func (SiteSettings) TableName() string { return "site_settings" }

type SiteDesignSystem struct {
	ID          string         `gorm:"primaryKey;type:uuid"`
	WorkspaceID string         `gorm:"type:uuid;not null;index:idx_site_design_systems_listing,priority:1;uniqueIndex:ux_site_design_systems_default,where:is_default"`
	Name        string         `gorm:"type:varchar(120);not null"`
	Tokens      datatypes.JSON `gorm:"type:jsonb;not null"`
	Fonts       datatypes.JSON `gorm:"type:jsonb;not null"`
	LogoMediaID string         `gorm:"type:varchar(64);not null;default:''"`
	Voice       string         `gorm:"type:text;not null;default:''"`
	Snippets    datatypes.JSON `gorm:"type:jsonb;not null"`
	IsDefault   bool           `gorm:"not null;default:false"`
	Version     int64          `gorm:"not null;default:1"`
	CreatedAt   time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt   time.Time      `gorm:"type:timestamptz;not null;index:idx_site_design_systems_listing,priority:2,sort:desc"`
}

const SiteDesignSystemDefaultPredicate = "is_default"

func (SiteDesignSystem) TableName() string { return "site_design_systems" }

type SiteFileRev struct {
	WorkspaceID string    `gorm:"type:uuid;not null"`
	ProjectID   string    `gorm:"primaryKey;type:uuid"`
	Path        string    `gorm:"primaryKey;type:varchar(200)"`
	Rev         int64     `gorm:"primaryKey;autoIncrement:false"`
	SHA256      string    `gorm:"column:sha256;type:varchar(64);not null"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null"`
}

func (SiteFileRev) TableName() string { return "site_file_revs" }

func SiteStorageModels() []any {
	return []any{&SiteProject{}, &SiteBlob{}, &SiteRevision{}, &SiteSettings{}, &SiteDesignSystem{}, &SiteFileRev{}}
}
