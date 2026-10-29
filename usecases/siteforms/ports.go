package siteforms

import (
	"context"
	"time"

	ads "vozko/domain/advertising"
	"vozko/domain/calendar"
	"vozko/domain/conversation"
	"vozko/domain/emailconsent"
	"vozko/domain/lead"
	"vozko/domain/opportunity"
	"vozko/domain/shortlink"
	"vozko/domain/site"
	"vozko/domain/siteform"
	"vozko/domain/sitesubmission"
	wo "vozko/domain/whatsapp_outreach"
	calendar_usecase "vozko/usecases/calendar"
	emailconsent_usecase "vozko/usecases/emailconsent"
	"vozko/usecases/emailinbox"
	"vozko/usecases/formlead"
	lead_usecase "vozko/usecases/lead"
	opportunity_usecase "vozko/usecases/opportunity"
	uwuc "vozko/usecases/unofficial_whatsapp"
)

type HostnameReader interface {
	ActiveHostnames(ctx context.Context, projectID string) ([]string, error)
}

type ProjectDirectory interface {
	WorkspaceOf(ctx context.Context, projectID string) (string, error)
}

type ActiveForm struct {
	WorkspaceID  string
	ProjectID    string
	DeploymentID string
	FormKey      string
	FormID       string
	Manifest     siteform.Manifest
	Settings     site.FormSettings
	SuccessURL   string
	Booking      *site.Booking
}

type Activation struct {
	WorkspaceID  string
	ProjectID    string
	DeploymentID string
	Forms        []ActiveForm
	Widgets      []string
}

type BookingCalendar interface {
	Busy(ctx context.Context, workspaceID string, from, to time.Time) ([]calendar.Slot, error)
	Reserve(ctx context.Context, req calendar_usecase.BookingRequest) (calendar.Reservation, error)
	Release(ctx context.Context, workspaceID, googleEventID string) error
}

type FormStore interface {
	EnsureKeys(ctx context.Context, workspaceID, projectID string, proposed map[string]string) (map[string]string, error)
	Activate(ctx context.Context, a Activation) error
	Deactivate(ctx context.Context, projectID string) error
	ActiveForm(ctx context.Context, formKey string) (*ActiveForm, error)
	ProjectsEmbedding(ctx context.Context, workspaceID, widgetID string) ([]string, error)
	FormDirectory
}

type SubmissionQuery struct {
	WorkspaceID string
	ProjectID   string
	FormID      string
	From        *time.Time
	To          *time.Time
	Page        int
	PageSize    int
}

type SubmissionPage struct {
	Items []sitesubmission.Submission
	Total int64
}

type SubmissionStore interface {
	ByIdempotency(ctx context.Context, formKey, key string) (*sitesubmission.Submission, error)
	Create(ctx context.Context, s sitesubmission.Submission, effects sitesubmission.EffectRun) (sitesubmission.Submission, bool, error)
	AttachConversation(ctx context.Context, id, conversationID, conversationType string) error
	List(ctx context.Context, q SubmissionQuery) ([]sitesubmission.Submission, int64, error)
}

type EffectStore interface {
	Claim(ctx context.Context, submissionID string, now time.Time, lease time.Duration) (*sitesubmission.EffectRun, error)
	Due(ctx context.Context, now time.Time, limit int) ([]string, error)
	Progress(ctx context.Context, run sitesubmission.EffectRun) error
	Settle(ctx context.Context, run sitesubmission.EffectRun) error
}

type LeadWriter interface {
	Write(ctx context.Context, sub formlead.Submission) (formlead.Outcome, error)
	Trigger(ctx context.Context, in formlead.Trigger) error
}

type EmailOptIns interface {
	Subscribe(ctx context.Context, workspaceID string, in emailconsent_usecase.SubscribeInput) (emailconsent.Subscription, error)
}

type Roulette interface {
	PickForLead(workspaceID, departmentID string) (string, error)
}

type LeadOwners interface {
	SetOwner(ctx context.Context, a lead_usecase.Actor, id, owner string) (*lead.Lead, error)
}

type Deals interface {
	InitialStage(workspaceID, pipelineID string) (string, error)
	Create(workspaceID string, in opportunity_usecase.CreateInput) (*opportunity.Opportunity, error)
}

type OfficialEntries interface {
	Open(ctx context.Context, in wo.OpenEntryInput) (*wo.OpenedEntry, error)
}

type UnofficialConversations interface {
	Execute(ctx context.Context, in uwuc.StartConversationInput) (*uwuc.StartedConversation, error)
}

type EmailConversations interface {
	OpenConversation(ctx context.Context, in emailinbox.OpenConversationInput) (string, error)
}

type Conversions interface {
	SendWebsiteLead(ctx context.Context, workspaceID string, lead ads.WebsiteLead) error
}

type ShortLinks interface {
	Execute(ctx context.Context, input shortlink.CreateShortLinkInput) (*shortlink.ShortLink, error)
}

type WhatsAppNumbers interface {
	NumberOf(ctx context.Context, workspaceID, numberRef string) (string, error)
}

type PageReferenceKey struct {
	ProjectID  string
	Route      string
	NumberRef  string
	TextDigest string
}

type PageReference struct {
	Key         PageReferenceKey
	Code        string
	WorkspaceID string
	ShortLinkID string
	URL         string
}

type PageReferenceStore interface {
	conversation.PageReferences
	Find(ctx context.Context, key PageReferenceKey) (*PageReference, error)
	Create(ctx context.Context, ref PageReference) (PageReference, bool, error)
}
