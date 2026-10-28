package sitepublish

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"vozko/domain/site"
)

const VisitorHashLength = 32

var ErrVisitInvalid = errors.New("sitepublish: the visit is invalid")

var (
	botAgent    = regexp.MustCompile(`(?i)bot|crawl|spider|slurp|facebookexternalhit|preview|monitor|headless|lighthouse|curl|wget|python|go-http`)
	visitorHash = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

type VisitEvent string

const (
	VisitView       VisitEvent = "view"
	VisitCTA        VisitEvent = "cta"
	VisitFormView   VisitEvent = "form_view"
	VisitFormStart  VisitEvent = "form_start"
	VisitFormSubmit VisitEvent = "form_submit"
	VisitWhatsApp   VisitEvent = "whatsapp"
)

func ParseVisitEvent(raw string) (VisitEvent, bool) {
	switch e := VisitEvent(raw); e {
	case VisitView, VisitCTA, VisitFormView, VisitFormStart, VisitFormSubmit, VisitWhatsApp:
		return e, true
	}
	return "", false
}

type Visit struct {
	ProjectID string
	Day       time.Time
	Route     string
	Variant   string
	Event     VisitEvent
	Referrer  string
	UTMSource string
	Visitor   string
}

func visitError(reason string) error {
	return fmt.Errorf("%w: %s", ErrVisitInvalid, reason)
}

func (v Visit) Validate() error {
	if !deploymentID.MatchString(v.ProjectID) {
		return visitError("the page id is malformed")
	}
	if v.Day.IsZero() {
		return visitError("the day is missing")
	}
	if !site.ValidRoutePath(v.Route) || !variantKey.MatchString(v.Variant) {
		return visitError("the route or variant is malformed")
	}
	if _, ok := ParseVisitEvent(string(v.Event)); !ok {
		return visitError("the event is unknown")
	}
	if v.Referrer != "" && !ValidSource(SourceReferrer, v.Referrer) {
		return visitError("the referrer is not an origin")
	}
	if v.UTMSource != "" && !ValidSource(SourceUTM, v.UTMSource) {
		return visitError("the utm source is unreadable")
	}
	if !visitorHash.MatchString(v.Visitor) {
		return visitError("the visitor hash is malformed")
	}
	return nil
}

func (v Visit) Source(kind SourceKind) string {
	switch kind {
	case SourceUTM:
		return v.UTMSource
	case SourceReferrer:
		return v.Referrer
	}
	return ""
}

func (v Visit) CountsForSources() bool {
	return v.Event == VisitView || v.Event == VisitFormSubmit
}

func SourceKinds() []SourceKind {
	return []SourceKind{SourceUTM, SourceReferrer}
}

func (s *DailyStat) Count(event VisitEvent, n int64) {
	switch event {
	case VisitView:
		s.Views += n
	case VisitCTA:
		s.CTA += n
	case VisitFormView:
		s.FormViews += n
	case VisitFormStart:
		s.FormStarts += n
	case VisitFormSubmit:
		s.FormSubmits += n
	case VisitWhatsApp:
		s.WhatsApp += n
	}
}

func (s *SourceStat) Count(event VisitEvent, n int64) {
	switch event {
	case VisitView:
		s.Views += n
	case VisitFormSubmit:
		s.Submits += n
	}
}

func IsBot(userAgent string) bool {
	return userAgent == "" || botAgent.MatchString(userAgent)
}

func VisitorHash(secret []byte, day time.Time, clientIP, userAgent, projectID string) string {
	salt := sign(secret, "visitor|"+day.Format(time.DateOnly))
	return hex.EncodeToString(sign(salt, clientIP+"|"+userAgent+"|"+projectID))[:VisitorHashLength]
}

func sign(secret []byte, message string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(message))
	return mac.Sum(nil)
}

func ReferrerOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ""
	}
	origin := u.Scheme + "://" + u.Host
	if !ValidSource(SourceReferrer, origin) {
		return ""
	}
	return origin
}

const maxUTMLength = 100

func UTMValue(raw string) string {
	value := cleanText(raw, maxUTMLength)
	if !ValidSource(SourceUTM, value) {
		return ""
	}
	return value
}
