package formlead

import (
	"errors"
	"strings"
	"time"

	"vozko/domain/lead"
)

var (
	ErrSubmissionIncomplete = errors.New("formlead: a form submission needs its workspace, a known form kind, the form, its id and when it was sent")
	ErrLeadNotResolved      = errors.New("formlead: the lead of the form submission could not be found or created")
)

type Kind string

const (
	KindWhatsAppForm Kind = "whatsapp_form"
	KindSiteForm     Kind = "site_form"
)

func (k Kind) Valid() bool {
	return k == KindWhatsAppForm || k == KindSiteForm
}

type Form struct {
	Kind Kind
	ID   string
}

func (f Form) valid() bool {
	return f.Kind.Valid() && strings.TrimSpace(f.ID) != ""
}

type Identity struct {
	LeadID string
	Phone  string
	Email  string
}

type Consent struct {
	Text string
}

func (c Consent) grant(submissionID string, at time.Time) lead.ConsentChange {
	text := strings.TrimSpace(c.Text)
	return lead.ConsentChange{
		Kind:     lead.ConsentGrant,
		Source:   lead.ConsentForm,
		Actor:    lead.ConsentActor{Type: lead.ConsentActorLead},
		Purpose:  text,
		Evidence: lead.ConsentEvidence{Type: lead.EvidenceFormSubmission, Ref: submissionID, Excerpt: text},
		At:       at,
	}
}

type Submission struct {
	WorkspaceID string
	Form        Form
	ID          string
	Identity    Identity
	Profile     lead.Profile
	Consent     *Consent
	SubmittedAt time.Time
}

func (s Submission) validate() error {
	if strings.TrimSpace(s.WorkspaceID) == "" || !s.Form.valid() || strings.TrimSpace(s.ID) == "" || s.SubmittedAt.IsZero() {
		return ErrSubmissionIncomplete
	}
	return nil
}

const (
	ConsentGranted   = "granted"
	ConsentUnchanged = "unchanged"
)

type Outcome struct {
	LeadID    string
	Created   bool
	Owner     string
	Changed   []string
	Conflicts []string
	Refused   []string
	Consent   string
}

type Trigger struct {
	WorkspaceID  string
	EntryID      string
	EntryType    string
	Form         Form
	SubmissionID string
	Data         map[string]any
}
