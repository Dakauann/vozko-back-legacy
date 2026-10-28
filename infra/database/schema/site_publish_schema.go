package schema

import (
	"time"

	"gorm.io/datatypes"
)

type SiteDeployment struct {
	ID          string         `gorm:"type:uuid;primaryKey"`
	WorkspaceID string         `gorm:"type:uuid;not null;index"`
	ProjectID   string         `gorm:"type:uuid;not null;index:idx_site_deployments_project_created,priority:1;uniqueIndex:ux_site_deployments_active_project,where:status = 'active'"`
	RevisionID  string         `gorm:"type:text;not null;default:''"`
	Status      string         `gorm:"type:varchar(16);not null;index"`
	Manifest    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	Checks      datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	Gaps        datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	Forms       datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	Redirects   datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	Settings    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	Outbound    datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`
	PublishedBy string         `gorm:"type:text;not null;default:''"`
	PublishedAt *time.Time
	CreatedAt   time.Time `gorm:"not null;index:idx_site_deployments_project_created,priority:2"`
}

func (SiteDeployment) TableName() string { return "site_deployments" }

type SiteHostname struct {
	ID            string         `gorm:"type:uuid;primaryKey"`
	WorkspaceID   string         `gorm:"type:uuid;not null;index"`
	ProjectID     string         `gorm:"type:uuid;not null;index"`
	Hostname      string         `gorm:"type:varchar(253);not null;uniqueIndex:idx_site_hostnames_hostname"`
	Kind          string         `gorm:"type:varchar(16);not null"`
	Status        string         `gorm:"type:varchar(16);not null;index"`
	CloudflareRef string         `gorm:"type:text;not null;default:''"`
	Verification  datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt     time.Time      `gorm:"not null"`
	UpdatedAt     time.Time      `gorm:"not null"`
}

func (SiteHostname) TableName() string { return "site_hostnames" }

type SiteAnalyticsDaily struct {
	ProjectID   string    `gorm:"type:uuid;primaryKey"`
	Day         time.Time `gorm:"type:date;primaryKey"`
	Route       string    `gorm:"type:varchar(200);primaryKey"`
	Variant     string    `gorm:"type:varchar(16);primaryKey"`
	Views       int64     `gorm:"not null;default:0"`
	Visitors    int64     `gorm:"not null;default:0"`
	CTA         int64     `gorm:"column:cta;not null;default:0"`
	FormViews   int64     `gorm:"not null;default:0"`
	FormStarts  int64     `gorm:"not null;default:0"`
	FormSubmits int64     `gorm:"not null;default:0"`
	WhatsApp    int64     `gorm:"column:whatsapp;not null;default:0"`
}

func (SiteAnalyticsDaily) TableName() string { return "site_analytics_daily" }

type SiteAbuseCase struct {
	ID           string `gorm:"type:uuid;primaryKey"`
	WorkspaceID  string `gorm:"type:uuid;not null;index"`
	ProjectID    string `gorm:"type:uuid;not null;index"`
	DeploymentID string `gorm:"type:text;not null;default:''"`
	Source       string `gorm:"type:varchar(16);not null"`
	Status       string `gorm:"type:varchar(16);not null;index"`
	Reason       string `gorm:"type:text;not null;default:''"`
	DecidedBy    string `gorm:"type:text;not null;default:''"`
	DecidedAt    *time.Time
	CreatedAt    time.Time `gorm:"not null"`
}

func (SiteAbuseCase) TableName() string { return "site_abuse_cases" }

func SitePublishModels() []any {
	return []any{&SiteDeployment{}, &SiteHostname{}, &SiteAnalyticsDaily{}, &SiteAnalyticsSource{}, &SiteAbuseCase{}, &SiteServingKey{}}
}

type SiteAnalyticsSource struct {
	ProjectID string    `gorm:"type:uuid;primaryKey"`
	Day       time.Time `gorm:"type:date;primaryKey"`
	Kind      string    `gorm:"type:varchar(16);primaryKey"`
	Value     string    `gorm:"type:varchar(200);primaryKey"`
	Views     int64     `gorm:"not null;default:0"`
	Submits   int64     `gorm:"not null;default:0"`
}

func (SiteAnalyticsSource) TableName() string { return "site_analytics_sources" }

type SiteServingKey struct {
	Key       string    `gorm:"type:varchar(320);primaryKey"`
	Value     string    `gorm:"type:text;not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (SiteServingKey) TableName() string { return "site_serving_keys" }
