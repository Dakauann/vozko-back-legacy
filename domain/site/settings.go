package site

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"vozko/domain/siteform"
	"vozko/domain/strictjson"
)

var ErrSettingsInvalid = errors.New("site: the settings document is invalid")

type IntegrationKind string

const (
	IntegrationMetaPixel   IntegrationKind = "meta_pixel"
	IntegrationGA4         IntegrationKind = "ga4"
	IntegrationGTM         IntegrationKind = "gtm"
	IntegrationGoogleAds   IntegrationKind = "google_ads"
	IntegrationTikTokPixel IntegrationKind = "tiktok_pixel"
	IntegrationClarity     IntegrationKind = "clarity"
)

func IntegrationKinds() []IntegrationKind {
	return []IntegrationKind{IntegrationMetaPixel, IntegrationGA4, IntegrationGTM, IntegrationGoogleAds, IntegrationTikTokPixel, IntegrationClarity}
}

var integrationIDs = map[IntegrationKind]*regexp.Regexp{
	IntegrationMetaPixel:   regexp.MustCompile(`^\d{10,20}$`),
	IntegrationGA4:         regexp.MustCompile(`^G-[A-Z0-9]{4,16}$`),
	IntegrationGTM:         regexp.MustCompile(`^GTM-[A-Z0-9]{4,12}$`),
	IntegrationGoogleAds:   regexp.MustCompile(`^AW-\d{6,14}(/[A-Za-z0-9_-]{1,64})?$`),
	IntegrationTikTokPixel: regexp.MustCompile(`^[A-Z0-9]{10,32}$`),
	IntegrationClarity:     regexp.MustCompile(`^[a-z0-9]{6,16}$`),
}

func ValidIntegrationID(kind IntegrationKind, id string) bool {
	pattern, ok := integrationIDs[kind]
	return ok && pattern.MatchString(id)
}

type Integrations struct {
	MetaPixel   string `json:"meta_pixel,omitempty"`
	GA4         string `json:"ga4,omitempty"`
	GTM         string `json:"gtm,omitempty"`
	GoogleAds   string `json:"google_ads,omitempty"`
	TikTokPixel string `json:"tiktok_pixel,omitempty"`
	Clarity     string `json:"clarity,omitempty"`
}

func (i Integrations) ID(kind IntegrationKind) (string, bool) {
	values := map[IntegrationKind]string{
		IntegrationMetaPixel: i.MetaPixel, IntegrationGA4: i.GA4, IntegrationGTM: i.GTM,
		IntegrationGoogleAds: i.GoogleAds, IntegrationTikTokPixel: i.TikTokPixel, IntegrationClarity: i.Clarity,
	}
	id := values[kind]
	return id, id != ""
}

func (i Integrations) Enabled() []IntegrationKind {
	var out []IntegrationKind
	for _, kind := range IntegrationKinds() {
		if _, ok := i.ID(kind); ok {
			out = append(out, kind)
		}
	}
	return out
}

type PageMeta struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	OGMediaID   string `json:"og_media_id,omitempty"`
	Canonical   string `json:"canonical,omitempty"`
	NoIndex     bool   `json:"noindex,omitempty"`
}

type OwnerKind string

const (
	OwnerUnassigned OwnerKind = "unassigned"
	OwnerUser       OwnerKind = "user"
	OwnerRoulette   OwnerKind = "roulette"
)

type Owner struct {
	Kind OwnerKind `json:"kind"`
	ID   string    `json:"id,omitempty"`
}

type SuccessKind string

const (
	SuccessMessage  SuccessKind = "message"
	SuccessRedirect SuccessKind = "redirect"
	SuccessWhatsApp SuccessKind = "whatsapp"
)

type Success struct {
	Kind    SuccessKind `json:"kind"`
	Message string      `json:"message,omitempty"`
	Route   string      `json:"route,omitempty"`
	Channel string      `json:"channel,omitempty"`
}

type FollowUpKind string

const (
	FollowUpWhatsApp           FollowUpKind = "whatsapp"
	FollowUpUnofficialWhatsApp FollowUpKind = "unofficial_whatsapp"
	FollowUpEmail              FollowUpKind = "email"
)

func (k FollowUpKind) Valid() bool {
	return k == FollowUpWhatsApp || k == FollowUpUnofficialWhatsApp || k == FollowUpEmail
}

func (k FollowUpKind) Field() string {
	switch k {
	case FollowUpWhatsApp, FollowUpUnofficialWhatsApp:
		return siteform.TargetPhone
	case FollowUpEmail:
		return siteform.TargetEmail
	}
	return ""
}

const CodeFormFollowUpFieldMissing = "form_follow_up_field_missing"

func followUpReachable(m siteform.Manifest, form FormSettings) bool {
	if form.FollowUp == nil {
		return true
	}
	field := form.FollowUp.Kind.Field()
	if field == "" {
		return false
	}
	_, ok := m.Field(field)
	return ok
}

type FollowUp struct {
	Kind FollowUpKind `json:"kind"`
	ID   string       `json:"id"`
}

const CodeFormChannelRequired = "form_channel_required"

var ErrFormChannelRequired = errors.New("site: a form that starts a workflow needs a follow up channel")

const CodeFormDealOwnerRequired = "form_deal_owner_required"

var ErrFormDealOwnerRequired = errors.New("site: a form that opens deals needs an owner or a workflow to own them")

func (f FormSettings) CheckDealOwner(id string) error {
	if f.FunnelID == "" || f.WorkflowID != "" || (f.Owner != nil && f.Owner.Kind != OwnerUnassigned) {
		return nil
	}
	return fmt.Errorf("%w: %w: form %s", ErrSettingsInvalid, ErrFormDealOwnerRequired, id)
}

type FormSettings struct {
	Name       string    `json:"name,omitempty"`
	FunnelID   string    `json:"funnel_id,omitempty"`
	StageID    string    `json:"stage_id,omitempty"`
	Owner      *Owner    `json:"owner,omitempty"`
	WorkflowID string    `json:"workflow_id,omitempty"`
	FollowUp   *FollowUp `json:"follow_up,omitempty"`
	Success    *Success  `json:"success,omitempty"`
	Notify     bool      `json:"notify,omitempty"`
}

type Redirect struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Status int    `json:"status"`
}

type ConsentCategory string

const (
	ConsentAnalytics ConsentCategory = "analytics"
	ConsentMarketing ConsentCategory = "marketing"
)

type Consent struct {
	Banner     bool              `json:"banner,omitempty"`
	Text       string            `json:"text,omitempty"`
	Categories []ConsentCategory `json:"categories,omitempty"`
}

type Settings struct {
	Pages        map[string]PageMeta     `json:"pages,omitempty"`
	Integrations Integrations            `json:"integrations,omitzero"`
	Forms        map[string]FormSettings `json:"forms,omitempty"`
	Bookings     map[string]Booking      `json:"bookings,omitempty"`
	Redirects    []Redirect              `json:"redirects,omitempty"`
	Consent      Consent                 `json:"consent,omitzero"`
	Site         Identity                `json:"site,omitzero"`
}

const (
	maxTitleRunes       = 70
	maxDescriptionRunes = 320
	maxMessageRunes     = 500
	maxConsentRunes     = 2000
)

var (
	formSettingsID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	referenceID    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

func settingsError(reason string) error {
	return fmt.Errorf("%w: %s", ErrSettingsInvalid, reason)
}

func DecodeSettings(raw []byte) (Settings, error) {
	var s Settings
	if err := strictjson.Decode(raw, &s); err != nil {
		return Settings{}, fmt.Errorf("%w: %w", ErrSettingsInvalid, err)
	}
	if err := s.Validate(); err != nil {
		return Settings{}, err
	}
	return s, nil
}

func (s Settings) Validate() error {
	for route, meta := range s.Pages {
		if !ValidRoutePath(route) {
			return settingsError("seo route " + route + " is not a route path")
		}
		if utf8.RuneCountInString(meta.Title) > maxTitleRunes || utf8.RuneCountInString(meta.Description) > maxDescriptionRunes {
			return settingsError("seo text for " + route + " is too long")
		}
		if meta.OGMediaID != "" && !referenceID.MatchString(meta.OGMediaID) {
			return settingsError("the social image of " + route + " is not a media id")
		}
		if meta.Canonical != "" && !httpsURL(meta.Canonical) {
			return settingsError("the canonical of " + route + " must be an https address")
		}
	}
	for _, kind := range IntegrationKinds() {
		if id, ok := s.Integrations.ID(kind); ok && !ValidIntegrationID(kind, id) {
			return settingsError(string(kind) + " id is malformed")
		}
	}
	for id, form := range s.Forms {
		if !formSettingsID.MatchString(id) {
			return settingsError("form " + id + " is not a form id")
		}
		if err := form.validate(id); err != nil {
			return err
		}
	}
	for id, booking := range s.Bookings {
		if !formSettingsID.MatchString(id) {
			return settingsError("booking " + id + " is not a booking id")
		}
		if err := booking.validate(id); err != nil {
			return err
		}
	}
	if err := ValidateRedirects(s.Redirects); err != nil {
		return fmt.Errorf("%w: %w", ErrSettingsInvalid, err)
	}
	if err := s.Site.validate(); err != nil {
		return err
	}
	return s.Consent.validate()
}

const MaxFormNameRunes = 80

func validFormName(name string) bool {
	return name == "" || (name == strings.TrimSpace(name) && utf8.RuneCountInString(name) <= MaxFormNameRunes && !strings.ContainsFunc(name, unicode.IsControl))
}

func (f FormSettings) validate(id string) error {
	if !validFormName(f.Name) {
		return settingsError("form " + id + " name is not a short single line")
	}
	for _, ref := range []string{f.FunnelID, f.StageID, f.WorkflowID} {
		if ref != "" && !referenceID.MatchString(ref) {
			return settingsError("form " + id + " has a malformed reference")
		}
	}
	if f.Owner != nil {
		switch f.Owner.Kind {
		case OwnerUnassigned:
			if f.Owner.ID != "" {
				return settingsError("form " + id + " unassigned owner takes no id")
			}
		case OwnerUser, OwnerRoulette:
			if !referenceID.MatchString(f.Owner.ID) {
				return settingsError("form " + id + " owner needs an id")
			}
		default:
			return settingsError("form " + id + " owner kind is unknown")
		}
	}
	if f.FollowUp != nil && (!f.FollowUp.Kind.Valid() || !referenceID.MatchString(f.FollowUp.ID)) {
		return settingsError("form " + id + " follow up channel is not a known channel")
	}
	if f.WorkflowID != "" && f.FollowUp == nil {
		return fmt.Errorf("%w: %w: form %s", ErrSettingsInvalid, ErrFormChannelRequired, id)
	}
	if f.Success == nil {
		return nil
	}
	switch f.Success.Kind {
	case SuccessMessage:
		if strings.TrimSpace(f.Success.Message) == "" || utf8.RuneCountInString(f.Success.Message) > maxMessageRunes {
			return settingsError("form " + id + " success message is required")
		}
	case SuccessRedirect:
		if !ValidRoutePath(f.Success.Route) {
			return settingsError("form " + id + " success route is not a route")
		}
	case SuccessWhatsApp:
		if !referenceID.MatchString(f.Success.Channel) {
			return settingsError("form " + id + " success channel is required")
		}
	default:
		return settingsError("form " + id + " success kind is unknown")
	}
	return nil
}

func (c Consent) validate() error {
	if utf8.RuneCountInString(c.Text) > maxConsentRunes {
		return settingsError("consent text is too long")
	}
	for _, category := range c.Categories {
		if category != ConsentAnalytics && category != ConsentMarketing {
			return settingsError("consent category " + string(category) + " is unknown")
		}
	}
	return nil
}

var ErrRedirectInvalid = errors.New("site: a redirect is invalid")

func ValidateRedirects(redirects []Redirect) error {
	seen := map[string]bool{}
	for _, r := range redirects {
		if r.From == HomeRoute || !ValidRoutePath(r.From) {
			return fmt.Errorf("%w: %s is not a redirectable path", ErrRedirectInvalid, r.From)
		}
		if seen[r.From] {
			return fmt.Errorf("%w: %s repeats", ErrRedirectInvalid, r.From)
		}
		seen[r.From] = true
		if r.Status != 301 && r.Status != 302 {
			return fmt.Errorf("%w: status must be 301 or 302", ErrRedirectInvalid)
		}
		if r.To == r.From || (!ValidRoutePath(r.To) && !httpsURL(r.To)) {
			return fmt.Errorf("%w: %s must point to a route or an https address", ErrRedirectInvalid, r.From)
		}
	}
	return nil
}

func httpsURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && len(raw) <= 2048
}
