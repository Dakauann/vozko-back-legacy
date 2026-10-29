package siteforms

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"vozko/domain/cache"
	"vozko/domain/conversation"
	"vozko/domain/notification"
	"vozko/domain/opportunity"
	"vozko/domain/shortlink"
	"vozko/domain/site"
	"vozko/domain/siteform"
	"vozko/domain/sitepublish"
	"vozko/domain/sitesubmission"
	"vozko/domain/whatsapplink"
	"vozko/usecases/publicform"
)

const (
	challengeKeyPrefix = "sites:form:challenge:"
	whatsAppLinkTitle  = "WhatsApp do site"
)

var (
	ErrIncomplete      = errors.New("siteforms: a required dependency is missing")
	ErrFormIDInvalid   = errors.New("siteforms: a form id is invalid")
	ErrKeysIncomplete  = errors.New("siteforms: the form keys could not all be kept")
	ErrWhatsAppText    = errors.New("siteforms: the WhatsApp message is too long")
	ErrPageInvalid     = errors.New("siteforms: the page of the submissions list is invalid")
	ErrWhatsAppNumber  = errors.New("siteforms: the WhatsApp number cannot be used by this page")
	ErrProjectNotFound = errors.New("siteforms: the page does not exist")
)

var formIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type Config struct {
	ChallengeKey  []byte
	IPHashKey     []byte
	Addresses     sitepublish.Addresses
	ShortLinkBase string
}

type Deps struct {
	Forms       FormStore
	Submissions SubmissionStore
	Effects     EffectStore
	Hostnames   HostnameReader
	Projects    ProjectDirectory
	OneShot     publicform.OneShot
	PerIP       cache.RateLimiter
	PerForm     cache.RateLimiter
	PerPreview  cache.RateLimiter
	Leads       LeadWriter
	EmailOptIns EmailOptIns
	Roulette    Roulette
	LeadOwners  LeadOwners
	Deals       Deals
	LeadDeals   opportunity.LeadDealReader
	Official    OfficialEntries
	Unofficial  UnofficialConversations
	Email       EmailConversations
	Notices     notification.Publisher
	Conversions Conversions
	Links       ShortLinks
	Numbers     WhatsAppNumbers
	References  PageReferenceStore
	Automations AutomationCatalog
	Calendar    BookingCalendar
	Slots       cache.Memo
	BookingLock cache.Locker
	LeadFields  LeadFieldDefinitions
	Config      Config
	Now         func() time.Time
	NewID       func() string
	Async       func(func())
	Random      io.Reader
}

type Service struct {
	d   Deps
	cfg Config
}

func New(d Deps) (*Service, error) {
	required := map[string]bool{
		"forms": d.Forms == nil, "submissions": d.Submissions == nil, "effects": d.Effects == nil, "hostnames": d.Hostnames == nil, "projects": d.Projects == nil,
		"one shot": d.OneShot == nil, "ip limiter": d.PerIP == nil, "form limiter": d.PerForm == nil, "preview limiter": d.PerPreview == nil, "leads": d.Leads == nil,
		"email opt ins": d.EmailOptIns == nil, "roulette": d.Roulette == nil, "lead owners": d.LeadOwners == nil, "deals": d.Deals == nil, "lead deals": d.LeadDeals == nil,
		"official whatsapp": d.Official == nil, "unofficial whatsapp": d.Unofficial == nil, "email": d.Email == nil,
		"notices": d.Notices == nil, "conversions": d.Conversions == nil, "short links": d.Links == nil, "numbers": d.Numbers == nil,
		"references": d.References == nil, "automations": d.Automations == nil,
		"booking calendar": d.Calendar == nil, "booking slots": d.Slots == nil, "booking lock": d.BookingLock == nil, "lead fields": d.LeadFields == nil, "challenge key": len(d.Config.ChallengeKey) == 0, "ip hash key": len(d.Config.IPHashKey) == 0,
		"platform domain": strings.TrimSpace(d.Config.Addresses.Domain) == "", "short link base": strings.TrimSpace(d.Config.ShortLinkBase) == "",
	}
	for name, missing := range required {
		if missing {
			return nil, fmt.Errorf("%w: %s", ErrIncomplete, name)
		}
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.NewID == nil {
		d.NewID = uuid.NewString
	}
	if d.Async == nil {
		d.Async = func(fn func()) { go fn() }
	}
	if d.Random == nil {
		d.Random = rand.Reader
	}
	return &Service{d: d, cfg: d.Config}, nil
}

func (s *Service) now() time.Time {
	return s.d.Now().UTC()
}

func (s *Service) gate() publicform.ChallengeGate {
	return publicform.ChallengeGate{Key: s.cfg.ChallengeKey, Spent: s.d.OneShot, KeyPrefix: challengeKeyPrefix, Bits: sitesubmission.ChallengeBits, Random: s.d.Random}
}

func (s *Service) KeysFor(ctx context.Context, workspaceID, projectID string, formIDs []string) (map[string]string, error) {
	proposed := make(map[string]string, len(formIDs))
	for _, id := range formIDs {
		if !formIDPattern.MatchString(id) {
			return nil, fmt.Errorf("%w: %q", ErrFormIDInvalid, id)
		}
		key, err := sitesubmission.NewFormKey(s.d.Random)
		if err != nil {
			return nil, err
		}
		proposed[id] = key
	}
	kept, err := s.d.Forms.EnsureKeys(ctx, workspaceID, projectID, proposed)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(formIDs))
	for _, id := range formIDs {
		key := kept[id]
		if !sitesubmission.ValidFormKey(key) {
			return nil, fmt.Errorf("%w: %s", ErrKeysIncomplete, id)
		}
		out[id] = key
	}
	return out, nil
}

type ActivateInput struct {
	WorkspaceID  string
	ProjectID    string
	DeploymentID string
	Manifests    []siteform.Manifest
	Settings     map[string]site.FormSettings
	Widgets      []string
	Bookings     map[string]site.Booking
}

func (s *Service) Activate(ctx context.Context, in ActivateInput) error {
	ids := make([]string, 0, len(in.Manifests))
	for _, m := range in.Manifests {
		if err := sitesubmission.CheckForm(m, in.Settings[m.ID]); err != nil {
			return err
		}
		ids = append(ids, m.ID)
	}
	keys, err := s.KeysFor(ctx, in.WorkspaceID, in.ProjectID, ids)
	if err != nil {
		return err
	}
	forms := make([]ActiveForm, 0, len(in.Manifests))
	for _, m := range in.Manifests {
		settings := in.Settings[m.ID]
		form := ActiveForm{WorkspaceID: in.WorkspaceID, ProjectID: in.ProjectID, DeploymentID: in.DeploymentID, FormKey: keys[m.ID], FormID: m.ID, Manifest: m, Settings: settings}
		if booking, ok := in.Bookings[m.ID]; ok {
			form.Booking = &booking
		}
		if settings.Success != nil && settings.Success.Kind == site.SuccessWhatsApp {
			link, err := s.WhatsAppLink(ctx, in.ProjectID, settings.Success.Channel, "", "")
			if err != nil {
				return err
			}
			form.SuccessURL = link.URL
		}
		forms = append(forms, form)
	}
	widgets := slices.Compact(slices.Sorted(slices.Values(in.Widgets)))
	return s.d.Forms.Activate(ctx, Activation{WorkspaceID: in.WorkspaceID, ProjectID: in.ProjectID, DeploymentID: in.DeploymentID, Forms: forms, Widgets: widgets})
}

func (s *Service) Deactivate(ctx context.Context, projectID string) error {
	return s.d.Forms.Deactivate(ctx, projectID)
}

type WhatsAppLinkResult struct {
	URL       string
	Reference string
}

func (s *Service) WhatsAppLink(ctx context.Context, projectID, numberRef, text, route string) (WhatsAppLinkResult, error) {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > site.MaxWhatsAppTextRunes {
		return WhatsAppLinkResult{}, ErrWhatsAppText
	}
	workspaceID, err := s.d.Projects.WorkspaceOf(ctx, projectID)
	if err != nil {
		return WhatsAppLinkResult{}, err
	}
	if workspaceID == "" {
		return WhatsAppLinkResult{}, ErrProjectNotFound
	}
	digest := sha256.Sum256([]byte(text))
	key := PageReferenceKey{ProjectID: projectID, Route: route, NumberRef: numberRef, TextDigest: hex.EncodeToString(digest[:])}
	if found, err := s.d.References.Find(ctx, key); err != nil {
		return WhatsAppLinkResult{}, err
	} else if found != nil {
		return WhatsAppLinkResult{URL: found.URL, Reference: found.Code}, nil
	}
	number, err := s.numberOf(ctx, workspaceID, numberRef)
	if err != nil {
		return WhatsAppLinkResult{}, err
	}
	code, err := conversation.NewPageReference()
	if err != nil {
		return WhatsAppLinkResult{}, err
	}
	target := whatsapplink.URL(number, conversation.PrefilledText(text, code))
	link, err := s.d.Links.Execute(ctx, shortlink.CreateShortLinkInput{WorkspaceID: workspaceID, TargetURL: target, Title: whatsAppLinkTitle})
	if err != nil {
		return WhatsAppLinkResult{}, err
	}
	ref, _, err := s.d.References.Create(ctx, PageReference{Key: key, Code: code, WorkspaceID: workspaceID, ShortLinkID: link.ID, URL: shortlink.ShortURL(s.cfg.ShortLinkBase, link.Code)})
	if err != nil {
		return WhatsAppLinkResult{}, err
	}
	return WhatsAppLinkResult{URL: ref.URL, Reference: ref.Code}, nil
}

func (s *Service) LiveForm(ctx context.Context, workspaceID, formKey string) (ActiveForm, error) {
	form, err := s.d.Forms.ActiveForm(ctx, formKey)
	if err != nil {
		return ActiveForm{}, err
	}
	if form == nil || form.WorkspaceID != workspaceID {
		return ActiveForm{}, fmt.Errorf("%w: %s", sitesubmission.ErrFormUnknown, formKey)
	}
	return *form, nil
}

func (s *Service) DirectLink(ctx context.Context, workspaceID, numberRef, text string) (string, error) {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > site.MaxWhatsAppTextRunes {
		return "", ErrWhatsAppText
	}
	number, err := s.numberOf(ctx, workspaceID, numberRef)
	if err != nil {
		return "", err
	}
	return whatsapplink.URL(number, text), nil
}

func (s *Service) numberOf(ctx context.Context, workspaceID, numberRef string) (string, error) {
	number, err := s.d.Numbers.NumberOf(ctx, workspaceID, numberRef)
	if err != nil {
		return "", err
	}
	if number == "" {
		return "", ErrWhatsAppNumber
	}
	return number, nil
}

type EmbedOrigins struct {
	Forms     FormStore
	Hostnames HostnameReader
	Origins   sitepublish.Origins
}

func (s *Service) WidgetOrigins(ctx context.Context, workspaceID, widgetID string) ([]string, error) {
	return EmbedOrigins{Forms: s.d.Forms, Hostnames: s.d.Hostnames, Origins: s.cfg.Addresses.Origins}.WidgetOrigins(ctx, workspaceID, widgetID)
}

func (e EmbedOrigins) WidgetOrigins(ctx context.Context, workspaceID, widgetID string) ([]string, error) {
	projects, err := e.Forms.ProjectsEmbedding(ctx, workspaceID, widgetID)
	if err != nil {
		return nil, err
	}
	var origins []string
	for _, project := range projects {
		hosts, err := e.Hostnames.ActiveHostnames(ctx, project)
		if err != nil {
			return nil, err
		}
		for _, host := range hosts {
			origins = append(origins, e.Origins.Of(host))
		}
	}
	return slices.Compact(slices.Sorted(slices.Values(origins))), nil
}

func (s *Service) Submissions(ctx context.Context, q SubmissionQuery) (SubmissionPage, error) {
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 100 {
		return SubmissionPage{}, ErrPageInvalid
	}
	items, total, err := s.d.Submissions.List(ctx, q)
	if err != nil {
		return SubmissionPage{}, err
	}
	return SubmissionPage{Items: items, Total: total}, nil
}
