package scheduled_message

import (
	"context"
	"errors"
	"strings"
	"time"

	"vozko/domain/actor"
	"vozko/domain/shared"
	"vozko/domain/whatsapp/template"
	wo "vozko/domain/whatsapp_outreach"
)

const MaxPendingFollowUps = 3

type CancelReason string

const (
	CancelByPerson            CancelReason = "person"
	CancelByAutomation        CancelReason = "automation"
	CancelContactReplied      CancelReason = "contact_replied"
	CancelAutomationStoodDown CancelReason = "automation_stood_down"
)

func (r CancelReason) IsKnown() bool {
	switch r {
	case CancelByPerson, CancelByAutomation, CancelContactReplied, CancelAutomationStoodDown:
		return true
	}
	return false
}

var (
	ErrFollowUpWhenRequired  = errors.New("follow-up: give in_minutes, or a time with days_from_today or a date")
	ErrFollowUpWhenAmbiguous = errors.New("follow-up: give either minutes from now or a day and a time, not both")
	ErrFollowUpWhenInvalid   = errors.New("follow-up: the time could not be read; use minutes above zero, a day from today of zero or more, a date as YYYY-MM-DD and a time as HH:MM")
	ErrFollowUpZoneRequired  = errors.New("follow-up: the workspace time zone is unknown")
	ErrFollowUpLimitReached  = errors.New("follow-up: this conversation already has the most pending follow-ups automation may keep")
	ErrFollowUpNotAllowed    = errors.New("follow-up: automation may not act on this conversation")

	ErrFollowUpNeedsConversation = errors.New("follow-up: this run is not on a conversation")
	ErrFollowUpVariableMissing   = errors.New("follow-up: a variable used in the node has no value")

	ErrAuthorAmbiguous         = errors.New("scheduled message: a message is scheduled either by a person or by automation, not both")
	ErrAutomationAuthorInvalid = errors.New("scheduled message: automation authors are ai:<agent id> or workflow:<workflow id>")
)

type FollowUpWhen struct {
	InMinutes     int
	DaysFromToday *int
	Date          string
	Time          string
}

func (w FollowUpWhen) Resolve(now time.Time, loc *time.Location) (time.Time, error) {
	if loc == nil {
		return time.Time{}, ErrFollowUpZoneRequired
	}
	date, clock := strings.TrimSpace(w.Date), strings.TrimSpace(w.Time)
	relative := w.InMinutes != 0
	onADay := w.DaysFromToday != nil || date != ""
	switch {
	case relative && (clock != "" || onADay):
		return time.Time{}, ErrFollowUpWhenAmbiguous
	case relative && w.InMinutes < 0:
		return time.Time{}, ErrFollowUpWhenInvalid
	case relative:
		return now.Add(time.Duration(w.InMinutes) * time.Minute), nil
	case clock == "":
		return time.Time{}, ErrFollowUpWhenRequired
	case w.DaysFromToday != nil && date != "":
		return time.Time{}, ErrFollowUpWhenAmbiguous
	}
	day, err := w.day(now, loc, date)
	if err != nil {
		return time.Time{}, err
	}
	hour, minute, err := clockOf(clock)
	if err != nil {
		return time.Time{}, err
	}
	at := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc)
	if date != "" && at.Before(now) {
		return time.Time{}, ErrFollowUpWhenPast
	}
	return at, nil
}

func (w FollowUpWhen) day(now time.Time, loc *time.Location, date string) (time.Time, error) {
	if date != "" {
		parsed, err := time.ParseInLocation("2006-01-02", date, loc)
		if err != nil {
			return time.Time{}, ErrFollowUpWhenInvalid
		}
		return parsed, nil
	}
	offset := 0
	if w.DaysFromToday != nil {
		offset = *w.DaysFromToday
	}
	if offset < 0 {
		return time.Time{}, ErrFollowUpWhenInvalid
	}
	return now.In(loc).AddDate(0, 0, offset), nil
}

func clockOf(raw string) (int, int, error) {
	for _, layout := range []string{"15:04", "15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.Hour(), parsed.Minute(), nil
		}
	}
	return 0, 0, ErrFollowUpWhenInvalid
}

func FollowUpKind(at time.Time, open bool, expiresAt *time.Time, now time.Time, entryType shared.EntryType, hasFallback bool) (Kind, error) {
	textErr := KindText.ValidateScheduledAt(at, open, expiresAt, now)
	if textErr == nil {
		return KindText, nil
	}
	outsideWindow := errors.Is(textErr, ErrWindowClosed) || errors.Is(textErr, ErrScheduledAtPastWindow)
	if !outsideWindow || !hasFallback || !entryType.SupportsTemplates() {
		return "", textErr
	}
	if err := KindTemplate.ValidateScheduledAt(at, open, expiresAt, now); err != nil {
		return "", err
	}
	return KindTemplate, nil
}

func ValidAutomationAuthor(id string) bool {
	switch {
	case actor.IsAI(id):
		return actor.ParseAI(id) != ""
	case actor.IsWorkflow(id):
		return actor.ParseWorkflow(id) != ""
	}
	return false
}

func (m *ScheduledMessage) ByAutomation() bool {
	return m.CreatedByAutomation != ""
}

func (m *ScheduledMessage) Author() string {
	if m.ByAutomation() {
		return m.CreatedByAutomation
	}
	return m.CreatedByUserID
}

func (m *ScheduledMessage) PendingLimit() (int, bool) {
	if m.ByAutomation() {
		return MaxPendingFollowUps, true
	}
	return 0, false
}

func (m *ScheduledMessage) validateAuthor() error {
	person := strings.TrimSpace(m.CreatedByUserID) != ""
	switch {
	case person && m.ByAutomation():
		return ErrAuthorAmbiguous
	case m.ByAutomation() && !ValidAutomationAuthor(m.CreatedByAutomation):
		return ErrAutomationAuthorInvalid
	case !person && !m.ByAutomation():
		return ErrSenderRequired
	}
	return nil
}

type FollowUpInput struct {
	WorkspaceID string
	EntryID     string
	EntryType   string
	Author      string

	Text     string
	When     FollowUpWhen
	Fallback *TemplateContent

	CancelOnReply  bool
	IdempotencyKey string
}

type FollowUpScheduled struct {
	Message  *ScheduledMessage
	Local    time.Time
	Replayed bool
}

type FollowUpCancel struct {
	WorkspaceID string
	EntryID     string
	EntryType   string
	Author      string
	ID          string
}

type FollowUpUseCase interface {
	Schedule(ctx context.Context, in FollowUpInput) (*FollowUpScheduled, error)
	Cancel(ctx context.Context, in FollowUpCancel) ([]*ScheduledMessage, error)
}

type ReplyCancellation interface {
	ContactReplied(entryID, entryType string, at time.Time) error
}

type followUpCode struct {
	code string
	errs []error
}

var followUpCodes = []followUpCode{
	{"follow_up_when_required", []error{ErrFollowUpWhenRequired}},
	{"follow_up_when_ambiguous", []error{ErrFollowUpWhenAmbiguous}},
	{"follow_up_when_invalid", []error{ErrFollowUpWhenInvalid}},
	{"follow_up_field_invalid", []error{ErrFollowUpFieldInvalid}},
	{"follow_up_when_past", []error{ErrFollowUpWhenPast}},
	{"follow_up_too_soon", []error{ErrScheduledAtTooSoon}},
	{"follow_up_too_far", []error{ErrScheduledAtTooFar}},
	{"follow_up_past_window", []error{ErrScheduledAtPastWindow}},
	{"follow_up_window_closed", []error{ErrWindowClosed}},
	{"follow_up_limit_reached", []error{ErrFollowUpLimitReached}},
	{"follow_up_not_allowed", []error{ErrFollowUpNotAllowed}},
	{"follow_up_needs_conversation", []error{ErrFollowUpNeedsConversation}},
	{"follow_up_variable_missing", []error{ErrFollowUpVariableMissing}},
	{"follow_up_not_found", []error{ErrNotFound, wo.ErrConversationNotFound}},
	{"follow_up_not_pending", []error{ErrNotPending}},
	{"follow_up_text_required", []error{ErrContentRequired}},
	{"follow_up_contact_ineligible", []error{wo.ErrLeadBlocked, wo.ErrLeadOptedOut, wo.ErrWithinSpamWindow}},
	{"follow_up_template_unavailable", []error{
		ErrTemplatesUnsupported, ErrTemplateRequired, wo.ErrTemplateNotFound, wo.ErrTemplateForbidden,
		wo.ErrPhoneNotConnected, wo.ErrBusinessPhoneNotFound,
		template.ErrTemplateNotSendable, template.ErrTemplatePhoneMismatch, template.ErrTemplateParamsMismatch,
	}},
}

const followUpUnavailable = "follow_up_unavailable"

func FollowUpErrorCode(err error) string {
	for _, entry := range followUpCodes {
		for _, known := range entry.errs {
			if errors.Is(err, known) {
				return entry.code
			}
		}
	}
	return followUpUnavailable
}

func FollowUpErrorCodes() []string {
	codes := make([]string, 0, len(followUpCodes)+1)
	for _, entry := range followUpCodes {
		codes = append(codes, entry.code)
	}
	return append(codes, followUpUnavailable)
}
