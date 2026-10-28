package schema

import (
	"time"

	"gorm.io/datatypes"
)

type SiteFormKey struct {
	FormKey     string    `gorm:"primaryKey;type:varchar(128)"`
	WorkspaceID string    `gorm:"type:uuid;not null"`
	ProjectID   string    `gorm:"type:uuid;not null;uniqueIndex:ux_site_form_keys_project_form,priority:1"`
	FormID      string    `gorm:"type:varchar(64);not null;uniqueIndex:ux_site_form_keys_project_form,priority:2"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null"`
}

func (SiteFormKey) TableName() string { return "site_form_keys" }

type SiteFormManifest struct {
	ID           string         `gorm:"primaryKey;type:uuid"`
	WorkspaceID  string         `gorm:"type:uuid;not null"`
	ProjectID    string         `gorm:"type:uuid;not null;index:idx_site_form_manifests_project_active,priority:1"`
	DeploymentID string         `gorm:"type:uuid;not null;index"`
	FormKey      string         `gorm:"type:varchar(128);not null;uniqueIndex:ux_site_form_manifests_active_key,where:active"`
	FormID       string         `gorm:"type:varchar(64);not null"`
	Manifest     datatypes.JSON `gorm:"type:jsonb;not null"`
	Settings     datatypes.JSON `gorm:"type:jsonb;not null"`
	SuccessURL   string         `gorm:"type:text;not null;default:''"`
	Active       bool           `gorm:"not null;default:false;index:idx_site_form_manifests_project_active,priority:2"`
	CreatedAt    time.Time      `gorm:"type:timestamptz;not null"`
}

func (SiteFormManifest) TableName() string { return "site_form_manifests" }

type SiteWidgetEmbed struct {
	DeploymentID string    `gorm:"primaryKey;type:uuid"`
	WidgetID     string    `gorm:"primaryKey;type:uuid;index:idx_site_widget_embeds_widget,priority:2"`
	WorkspaceID  string    `gorm:"type:uuid;not null;index:idx_site_widget_embeds_widget,priority:1"`
	ProjectID    string    `gorm:"type:uuid;not null;index"`
	Active       bool      `gorm:"not null;default:false"`
	CreatedAt    time.Time `gorm:"type:timestamptz;not null"`
}

func (SiteWidgetEmbed) TableName() string { return "site_widget_embeds" }

type SiteSubmission struct {
	ID               string         `gorm:"primaryKey;type:uuid"`
	WorkspaceID      string         `gorm:"type:uuid;not null;index:idx_site_submissions_listing,priority:1"`
	ProjectID        string         `gorm:"type:uuid;not null;index:idx_site_submissions_listing,priority:2"`
	DeploymentID     string         `gorm:"type:uuid;not null"`
	FormKey          string         `gorm:"type:varchar(128);not null;uniqueIndex:ux_site_submissions_idempotency,priority:1"`
	FormID           string         `gorm:"type:varchar(64);not null"`
	Route            string         `gorm:"type:varchar(512);not null;default:''"`
	Variant          string         `gorm:"type:varchar(64);not null;default:''"`
	Fields           datatypes.JSON `gorm:"type:jsonb;not null"`
	UTM              datatypes.JSON `gorm:"type:jsonb;not null"`
	ReferrerOrigin   string         `gorm:"type:varchar(2048);not null;default:''"`
	Fbclid           string         `gorm:"type:varchar(512);not null;default:''"`
	Gclid            string         `gorm:"type:varchar(512);not null;default:''"`
	Fbp              string         `gorm:"type:varchar(512);not null;default:''"`
	Fbc              string         `gorm:"type:varchar(512);not null;default:''"`
	Consent          datatypes.JSON `gorm:"type:jsonb;not null"`
	IPHash           string         `gorm:"type:varchar(64);not null;default:''"`
	UserAgent        string         `gorm:"type:varchar(512);not null;default:''"`
	LeadID           *string        `gorm:"type:uuid;index"`
	ConversationID   *string        `gorm:"type:uuid"`
	ConversationType string         `gorm:"type:varchar(32);not null;default:''"`
	IdempotencyKey   string         `gorm:"type:varchar(128);not null;uniqueIndex:ux_site_submissions_idempotency,priority:2"`
	CreatedAt        time.Time      `gorm:"type:timestamptz;not null;index:idx_site_submissions_listing,priority:3,sort:desc"`
}

func (SiteSubmission) TableName() string { return "site_submissions" }

type SiteSubmissionEffect struct {
	SubmissionID  string         `gorm:"primaryKey;type:uuid"`
	WorkspaceID   string         `gorm:"type:uuid;not null"`
	Plan          datatypes.JSON `gorm:"type:jsonb;not null"`
	Pending       datatypes.JSON `gorm:"type:jsonb;not null"`
	OwnerID       string         `gorm:"type:varchar(128);not null;default:"`
	EntryID       string         `gorm:"type:varchar(64);not null;default:"`
	EntryType     string         `gorm:"type:varchar(32);not null;default:"`
	Attempts      int            `gorm:"not null;default:0"`
	NextAttemptAt time.Time      `gorm:"type:timestamptz;not null;index:idx_site_submission_effects_due,where:finished_at IS NULL"`
	LockedUntil   *time.Time     `gorm:"type:timestamptz"`
	FinishedAt    *time.Time     `gorm:"type:timestamptz"`
	LastError     string         `gorm:"type:text;not null;default:"`
	CreatedAt     time.Time      `gorm:"type:timestamptz;not null"`
}

func (SiteSubmissionEffect) TableName() string { return "site_submission_effects" }

type SiteWhatsAppReference struct {
	Code        string    `gorm:"primaryKey;type:varchar(8)"`
	WorkspaceID string    `gorm:"type:uuid;not null"`
	ProjectID   string    `gorm:"type:uuid;not null;uniqueIndex:ux_site_whatsapp_references_key,priority:1"`
	Route       string    `gorm:"type:varchar(512);not null;default:'';uniqueIndex:ux_site_whatsapp_references_key,priority:2"`
	NumberRef   string    `gorm:"type:varchar(64);not null;uniqueIndex:ux_site_whatsapp_references_key,priority:3"`
	TextDigest  string    `gorm:"type:char(64);not null;uniqueIndex:ux_site_whatsapp_references_key,priority:4"`
	ShortLinkID string    `gorm:"type:uuid;not null"`
	URL         string    `gorm:"type:text;not null"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null"`
}

func (SiteWhatsAppReference) TableName() string { return "site_whatsapp_references" }

type ConversationPageOrigin struct {
	EntryID     string    `gorm:"type:uuid;primaryKey"`
	EntryType   string    `gorm:"type:varchar(32);primaryKey"`
	WorkspaceID string    `gorm:"type:uuid;not null"`
	ProjectID   string    `gorm:"type:uuid;not null;index"`
	Route       string    `gorm:"type:varchar(512);not null;default:''"`
	Reference   string    `gorm:"type:varchar(8);not null"`
	ArrivedAt   time.Time `gorm:"not null"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}

func (ConversationPageOrigin) TableName() string { return "conversation_page_origins" }

func SiteFormModels() []any {
	return []any{&SiteFormKey{}, &SiteFormManifest{}, &SiteWidgetEmbed{}, &SiteSubmission{}, &SiteSubmissionEffect{}, &SiteWhatsAppReference{}, &ConversationPageOrigin{}}
}
