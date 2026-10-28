package sitepublish

import (
	"errors"
	"time"

	"vozko/domain/dnsrecord"

	"vozko/domain/site"
	"vozko/domain/siteform"
)

var (
	ErrDeploymentNotFound = errors.New("sitepublish: the deployment was not found")
	ErrHostnameNotFound   = errors.New("sitepublish: the hostname was not found")
	ErrHostnameTaken      = errors.New("sitepublish: the hostname is already in use")
	ErrAbuseCaseNotFound  = errors.New("sitepublish: the abuse case was not found")
	ErrDomainQuota        = errors.New("sitepublish: the workspace has no custom domain left")
)

type FormsSnapshot struct {
	Manifests []siteform.Manifest          `json:"manifests,omitempty"`
	Settings  map[string]site.FormSettings `json:"settings,omitempty"`
	Widgets   []string                     `json:"widgets,omitempty"`
}

type DeploymentRecord struct {
	ID          string
	WorkspaceID string
	ProjectID   string
	RevisionID  string
	Status      DeploymentStatus
	Files       map[string]FileEntry
	Checks      []site.Check
	Gaps        []site.Issue
	Forms       FormsSnapshot
	Redirects   []site.Redirect
	Settings    site.Settings
	Outbound    []string
	PublishedBy string
	PublishedAt *time.Time
	CreatedAt   time.Time
}

type HostnamePage struct {
	Statuses              []HostnameStatus
	Kind                  HostnameKind
	HoldingProviderRecord bool
	AfterID               string
	Limit                 int
}

type DNSRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Verification struct {
	CNAMETarget  string      `json:"cname_target,omitempty"`
	Records      []DNSRecord `json:"records,omitempty"`
	SSLStatus    string      `json:"ssl_status,omitempty"`
	Errors       []string    `json:"errors,omitempty"`
	FailingSince *time.Time  `json:"failing_since,omitempty"`
	ActivatedAt  *time.Time  `json:"activated_at,omitempty"`

	OwnershipToken      string           `json:"ownership_token,omitempty"`
	OwnershipStatus     dnsrecord.Status `json:"ownership_status,omitempty"`
	OwnershipVerifiedAt *time.Time       `json:"ownership_verified_at,omitempty"`
	PendingSince        *time.Time       `json:"pending_since,omitempty"`
	Offline             OfflineReason    `json:"offline,omitempty"`
}

type HostnameRecord struct {
	ID            string
	WorkspaceID   string
	ProjectID     string
	Hostname      string
	Kind          HostnameKind
	Status        HostnameStatus
	CloudflareRef string
	Verification  Verification
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type LiveProject struct {
	WorkspaceID    string
	ProjectID      string
	DeploymentID   string
	ProjectStatus  string
	AbuseSuspended bool
}

type AbuseSource string

const (
	AbuseReport    AbuseSource = "report"
	AbuseScan      AbuseSource = "scan"
	AbuseHeuristic AbuseSource = "heuristic"
	AbuseAdmin     AbuseSource = "admin"
)

type AbuseStatus string

const (
	AbuseHeld      AbuseStatus = "held"
	AbuseSuspended AbuseStatus = "suspended"
	AbuseReleased  AbuseStatus = "released"
	AbuseRestored  AbuseStatus = "restored"
	AbuseDismissed AbuseStatus = "dismissed"
)

func (s AbuseStatus) Open() bool {
	return s == AbuseHeld || s == AbuseSuspended
}

type AbuseCase struct {
	ID           string
	WorkspaceID  string
	ProjectID    string
	DeploymentID string
	Source       AbuseSource
	Status       AbuseStatus
	Reason       string
	DecidedBy    string
	DecidedAt    *time.Time
	CreatedAt    time.Time
}

type DailyStat struct {
	ProjectID   string    `json:"-"`
	Day         time.Time `json:"day"`
	Route       string    `json:"route"`
	Variant     string    `json:"variant"`
	Views       int64     `json:"views"`
	Visitors    int64     `json:"visitors"`
	CTA         int64     `json:"cta"`
	FormViews   int64     `json:"formViews"`
	FormStarts  int64     `json:"formStarts"`
	FormSubmits int64     `json:"formSubmits"`
	WhatsApp    int64     `json:"whatsapp"`
}

type CustomHostnameState struct {
	Ref        string
	Hostname   string
	Phase      HostnameStatus
	SSLStatus  string
	SSLFailing bool
	Records    []DNSRecord
	Errors     []string
}

func (h *HostnameRecord) Transition(to HostnameStatus) error {
	if err := CheckHostnameTransition(h.Kind, h.Status, to); err != nil {
		return err
	}
	h.Status = to
	return nil
}
