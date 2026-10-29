package sitepublish_usecase

import (
	"context"
	"time"

	"vozko/domain/dnsrecord"
	"vozko/domain/media"
	"vozko/domain/notification"
	"vozko/domain/site"
	"vozko/domain/siteform"
	"vozko/domain/sitepublish"
	"vozko/domain/webchat"
	"vozko/domain/workspace"
	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

type Actor struct {
	UserID      string
	WorkspaceID string
}

func (a Actor) Ref() string {
	return "person:" + a.UserID
}

type ProjectSnapshot struct {
	ID          string
	WorkspaceID string
	Name        string
	Slug        string
	Project     site.Project
	Settings    site.Settings
	RevisionID  string
	Version     int64
}

type Publication string

const (
	PublicationLive      Publication = "published"
	PublicationSuspended Publication = "suspended"
	PublicationOffline   Publication = "draft"
)

func (p Publication) Valid() bool {
	return p == PublicationLive || p == PublicationSuspended || p == PublicationOffline
}

type PublishRecorded struct {
	RevisionID string
	Version    int64
	Current    bool
}

type PublishRecord struct {
	WorkspaceID      string
	ProjectID        string
	SourceRevisionID string
	SourceVersion    int64
	DeploymentID     string
	Project          site.Project
	Author           string
}

type Projects interface {
	Describe(ctx context.Context, workspaceID, projectID string) (ProjectSnapshot, error)
	Working(ctx context.Context, workspaceID, projectID string) (ProjectSnapshot, error)
	CurrentSettings(ctx context.Context, workspaceID, projectID string) (site.Settings, error)
	RecordPublish(ctx context.Context, in PublishRecord) (PublishRecorded, error)
	SetPublication(ctx context.Context, workspaceID, projectID string, state Publication, deploymentID string) error
}

type Workspaces interface {
	WorkspaceName(ctx context.Context, workspaceID string) (string, error)
}

type BuildAsset struct {
	Path        string
	ContentType string
	Body        []byte
}

type BuildRoute struct {
	Route       string
	HTMLPath    string
	Interactive bool
}

type BuildError struct {
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

type CandidateRequest struct {
	WorkspaceID   string
	ProjectID     string
	Project       site.Project
	Settings      site.Settings
	FormKeys      map[string]string
	PrimaryOrigin string
	OGImages      map[string]string
}

type Candidate struct {
	URL           string
	Assets        []BuildAsset
	Imports       []site.Import
	Routes        []BuildRoute
	Errors        []BuildError
	PrerenderPath string
}

type CandidateBuilder interface {
	Candidate(ctx context.Context, req CandidateRequest) (Candidate, error)
}

type Prerenderer interface {
	Prerender(ctx context.Context, url string, routes []string) (map[string]string, error)
	LiveDOM(ctx context.Context, url string, routes []string) (map[string]string, error)
	OpenGraph(ctx context.Context, url string) ([]byte, error)
}

type Thumbnails interface {
	Published(ctx context.Context, workspaceID, projectID, candidateURL string)
}

type ActivateForms struct {
	WorkspaceID  string
	ProjectID    string
	DeploymentID string
	Manifests    []siteform.Manifest
	Settings     map[string]site.FormSettings
	Widgets      []string
	Bookings     map[string]site.Booking
}

type BookingCalendars interface {
	Connected(ctx context.Context, workspaceID string) (bool, error)
}

type Forms interface {
	KeysFor(ctx context.Context, workspaceID, projectID string, formIDs []string) (map[string]string, error)
	Activate(ctx context.Context, in ActivateForms) error
	Deactivate(ctx context.Context, projectID string) error
	WhatsAppLink(ctx context.Context, projectID, numberRef, text, route string) (string, error)
}

type WebchatWidgets interface {
	FindByPublicKey(ctx context.Context, publicKey string) (*webchat.Widget, error)
	FindByID(ctx context.Context, workspaceID, id string) (*webchat.Widget, error)
}

type Hosting interface {
	HasBlob(ctx context.Context, sha string) (bool, error)
	PutBlob(ctx context.Context, sha, contentType string, body []byte) error
	PutManifest(ctx context.Context, deploymentID string, body []byte) error
	PutHost(ctx context.Context, hostname string, value []byte) error
	DeleteHost(ctx context.Context, hostname string) error
	PutDeployment(ctx context.Context, deploymentID string, value []byte) error
	DeleteDeployment(ctx context.Context, deploymentID string) error
}

type MediaLibrary interface {
	Owns(rawURL string) bool
	Lookup(ctx context.Context, workspaceID, rawURL string) (*media.Media, error)
	Fetch(ctx context.Context, rawURL string) ([]byte, string, error)
}

type CustomHostnames interface {
	Create(ctx context.Context, hostname string) (sitepublish.CustomHostnameState, error)
	Get(ctx context.Context, ref string) (sitepublish.CustomHostnameState, error)
	Delete(ctx context.Context, ref string) error
}

type Reversals interface {
	HasOpenReversal(ctx context.Context, workspaceID string) (bool, error)
}

type AccessChecker interface {
	Execute(userID, workspaceID string, resource workspace.Resource, action workspace.Action) error
}

type Subscriptions interface {
	Execute(workspaceID string) (*workspace_plan.WorkspaceSubscription, error)
}

type Entitlements interface {
	Resolve(workspaceID string, kind workspace_addon.EntitlementKind) (int, error)
}

type DeploymentStore interface {
	Create(ctx context.Context, rec sitepublish.DeploymentRecord) error
	Update(ctx context.Context, rec sitepublish.DeploymentRecord) error
	Get(ctx context.Context, workspaceID, id string) (sitepublish.DeploymentRecord, error)
	Active(ctx context.Context, projectID string) (sitepublish.DeploymentRecord, bool, error)
	Latest(ctx context.Context, projectID string) (sitepublish.DeploymentRecord, error)
	ListByProject(ctx context.Context, workspaceID, projectID string, limit int) ([]sitepublish.DeploymentRecord, error)
	ListActiveByWorkspace(ctx context.Context, workspaceID string) ([]sitepublish.DeploymentRecord, error)
	ListServable(ctx context.Context, projectID string) ([]sitepublish.DeploymentRecord, error)
	Activate(ctx context.Context, projectID, deploymentID, publishedBy string, at time.Time) error
	RecordPublished(ctx context.Context, projectID, deploymentID, revisionID string) error
	LiveProjects(ctx context.Context, afterProjectID string, limit int) ([]sitepublish.LiveProject, error)
}

type HostnameStore interface {
	Create(ctx context.Context, rec sitepublish.HostnameRecord) error
	Update(ctx context.Context, rec sitepublish.HostnameRecord) error
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, workspaceID, id string) (sitepublish.HostnameRecord, error)
	ListByProject(ctx context.Context, workspaceID, projectID string) ([]sitepublish.HostnameRecord, error)
	ListByStatus(ctx context.Context, statuses []sitepublish.HostnameStatus, kind sitepublish.HostnameKind, limit int) ([]sitepublish.HostnameRecord, error)
	ListPage(ctx context.Context, page sitepublish.HostnamePage) ([]sitepublish.HostnameRecord, error)
	ActiveHostnames(ctx context.Context, projectID string) ([]string, error)
	CountCustom(ctx context.Context, workspaceID string) (int, error)
	CreateCustom(ctx context.Context, rec sitepublish.HostnameRecord, allowed int) error
	ListCustomByWorkspace(ctx context.Context, workspaceID string) ([]sitepublish.HostnameRecord, error)
	CustomWorkspaces(ctx context.Context, afterWorkspaceID string, limit int) ([]string, error)
}

type AbuseStore interface {
	Create(ctx context.Context, c sitepublish.AbuseCase) error
	Update(ctx context.Context, c sitepublish.AbuseCase) error
	List(ctx context.Context, statuses []sitepublish.AbuseStatus, limit int) ([]sitepublish.AbuseCase, error)
	OpenForProject(ctx context.Context, projectID string) ([]sitepublish.AbuseCase, error)
	Suspended(ctx context.Context, projectID string) (bool, error)
}

type Alerts interface {
	Publish(ctx context.Context, d notification.Draft) (notification.Record, error)
	Resolve(ctx context.Context, workspaceID string, keys ...string) error
}

type Locks interface {
	SetNX(key string, value string, ttl time.Duration) (bool, error)
	Del(keys ...string) error
}

type Counters interface {
	IncrWithTTL(key string, ttl time.Duration) (int64, error)
	Del(keys ...string) error
}

type Config struct {
	Addresses          sitepublish.Addresses
	APIOrigin          string
	CNAMETarget        string
	ApplicationDomains []string
	BrandWords         []string
}

type Deps struct {
	Projects        Projects
	Workspaces      Workspaces
	Builder         CandidateBuilder
	Renderer        Prerenderer
	Forms           Forms
	Hosting         Hosting
	Media           MediaLibrary
	CustomHostnames CustomHostnames
	DNS             dnsrecord.TXTLookup
	Reversals       Reversals
	Access          AccessChecker
	Subscriptions   Subscriptions
	Entitlements    Entitlements
	Widgets         WebchatWidgets
	Deployments     DeploymentStore
	Hostnames       HostnameStore
	Abuse           AbuseStore
	Alerts          Alerts
	Thumbnails      Thumbnails
	Locks           Locks
	Counters        Counters
	Calendars       BookingCalendars
	Now             func() time.Time
	NewID           func() string
}
