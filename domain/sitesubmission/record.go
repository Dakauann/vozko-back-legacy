package sitesubmission

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"vozko/domain/site"
	"vozko/domain/siteform"
)

const (
	PreviewFormKey = "vz-preview"
	formKeyBytes   = 18
	ipHashLabel    = "vozko/sites/ip-hash/v1"
	ChallengeBits  = 16
)

var formKey = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

const (
	CodeBodyInvalid           = "submission_invalid"
	CodeFieldsInvalid         = "submission_fields_invalid"
	CodeFormUnknown           = "form_unknown"
	CodeOriginRefused         = "submission_origin_refused"
	CodeBotSuspected          = "submission_refused"
	CodeSuccessChannelMissing = "form_success_channel_required"
)

var (
	ErrBodyInvalid            = errors.New("sitesubmission: the submission body is not what the page runtime sends")
	ErrFieldsInvalid          = errors.New("sitesubmission: some fields do not match the published form")
	ErrFormUnknown            = errors.New("sitesubmission: the form is not published")
	ErrOriginRefused          = errors.New("sitesubmission: the page origin is not a published address of this form")
	ErrBotSuspected           = errors.New("sitesubmission: the submission looks automated")
	ErrFormChannelRequired    = site.ErrFormChannelRequired
	ErrSuccessChannelRequired = errors.New("sitesubmission: opening WhatsApp after the form needs a number")
	ErrRandomnessMissing      = errors.New("sitesubmission: no source of randomness was given")
)

type FieldsError struct {
	Details map[string]string
}

func (e *FieldsError) Error() string {
	return ErrFieldsInvalid.Error()
}

func (e *FieldsError) Is(target error) bool {
	return target == ErrFieldsInvalid
}

func CheckForm(m siteform.Manifest, s site.FormSettings) error {
	if s.Success != nil && s.Success.Kind == site.SuccessWhatsApp && strings.TrimSpace(s.Success.Channel) == "" {
		return fmt.Errorf("%w (form %s)", ErrSuccessChannelRequired, m.ID)
	}
	if s.WorkflowID == "" {
		return nil
	}
	refuse := fmt.Errorf("%w (form %s)", ErrFormChannelRequired, m.ID)
	if s.FollowUp == nil {
		return refuse
	}
	_, phone := m.Field(siteform.TargetPhone)
	_, email := m.Field(siteform.TargetEmail)
	switch s.FollowUp.Kind {
	case site.FollowUpWhatsApp, site.FollowUpUnofficialWhatsApp:
		if phone {
			return nil
		}
	case site.FollowUpEmail:
		if email {
			return nil
		}
	}
	return refuse
}

type SuccessAction struct {
	Kind    site.SuccessKind `json:"kind"`
	Message string           `json:"message,omitempty"`
	Route   string           `json:"route,omitempty"`
	URL     string           `json:"url,omitempty"`
}

func SuccessFor(s *site.Success, whatsAppURL string) SuccessAction {
	fallback := SuccessAction{Kind: site.SuccessMessage}
	if s == nil {
		return fallback
	}
	switch s.Kind {
	case site.SuccessMessage:
		return SuccessAction{Kind: site.SuccessMessage, Message: s.Message}
	case site.SuccessRedirect:
		return SuccessAction{Kind: site.SuccessRedirect, Route: s.Route}
	case site.SuccessWhatsApp:
		if strings.HasPrefix(whatsAppURL, "https://") {
			return SuccessAction{Kind: site.SuccessWhatsApp, URL: whatsAppURL}
		}
	}
	return fallback
}

func IPHashKey(secret string) []byte {
	if strings.TrimSpace(secret) == "" {
		return nil
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ipHashLabel))
	return mac.Sum(nil)
}

func HashIP(key []byte, ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" || len(key) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(ip))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func NewFormKey(random io.Reader) (string, error) {
	if random == nil {
		return "", ErrRandomnessMissing
	}
	raw := make([]byte, formKeyBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func ValidFormKey(key string) bool {
	return formKey.MatchString(key)
}

type Consent struct {
	Granted        bool   `json:"granted"`
	Text           string `json:"text,omitempty"`
	EmailOptIn     bool   `json:"email_opt_in"`
	EmailOptInText string `json:"email_opt_in_text,omitempty"`
}

type Submission struct {
	ID               string
	WorkspaceID      string
	ProjectID        string
	DeploymentID     string
	FormKey          string
	FormID           string
	Route            string
	Variant          string
	Fields           map[string]string
	UTM              UTM
	ReferrerOrigin   string
	Fbclid           string
	Gclid            string
	Fbp              string
	Fbc              string
	Consent          Consent
	IPHash           string
	UserAgent        string
	LeadID           string
	ConversationID   string
	ConversationType string
	IdempotencyKey   string
	CreatedAt        time.Time
}

func ConsentOf(m siteform.Manifest, v siteform.Values) Consent {
	c := Consent{Granted: v.Consent, EmailOptIn: v.EmailOptIn}
	if v.Consent {
		c.Text = m.ConsentText
	}
	if v.EmailOptIn {
		c.EmailOptInText = m.EmailOptInText
	}
	return c
}

func Codes() []string {
	return []string{CodeBodyInvalid, CodeFieldsInvalid, CodeFormUnknown, CodeOriginRefused, CodeBotSuspected, CodeSuccessChannelMissing, site.CodeFormChannelRequired}
}
