package formlead

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"vozko/domain/actor"
	"vozko/domain/lead"
	lead_usecase "vozko/usecases/lead"
)

var errIncomplete = errors.New("formlead: a required dependency is missing")

type PhoneLeads interface {
	FindOrCreate(workspaceID, number string, update lead.LeadUpdate) (*lead.Lead, bool, error)
}

type EmailLeads interface {
	FindOrCreateManyByEmail(ctx context.Context, workspaceID string, inputs []lead.BulkEmailInput) (map[string]*lead.Lead, error)
}

type ProfileWriter interface {
	Update(ctx context.Context, in lead_usecase.ProfileUpdate) (lead_usecase.ProfileResult, error)
}

type ConsentWriter interface {
	Record(ctx context.Context, workspaceID, leadID string, change lead.ConsentChange) (lead_usecase.ConsentOutcome, error)
}

type TriggerFirer interface {
	FormSubmitted(ctx context.Context, in Trigger) error
}

type Deps struct {
	Phones   PhoneLeads
	Emails   EmailLeads
	Profiles ProfileWriter
	Consents ConsentWriter
	Triggers TriggerFirer
}

type Service struct {
	deps Deps
}

func New(deps Deps) (*Service, error) {
	required := []struct {
		name   string
		absent bool
	}{
		{"phones", deps.Phones == nil}, {"emails", deps.Emails == nil}, {"profiles", deps.Profiles == nil},
		{"consents", deps.Consents == nil}, {"triggers", deps.Triggers == nil},
	}
	for _, r := range required {
		if r.absent {
			return nil, fmt.Errorf("%w: %s", errIncomplete, r.name)
		}
	}
	return &Service{deps: deps}, nil
}

func (s *Service) Write(ctx context.Context, sub Submission) (Outcome, error) {
	if err := sub.validate(); err != nil {
		return Outcome{}, err
	}
	resolved, err := s.resolveLead(ctx, sub)
	if err != nil {
		return Outcome{}, err
	}
	leadID := resolved.id
	out := Outcome{LeadID: leadID, Created: resolved.created, Owner: resolved.owner}
	if !sub.Profile.Empty() {
		result, err := s.deps.Profiles.Update(ctx, lead_usecase.ProfileUpdate{
			WorkspaceID: sub.WorkspaceID, LeadID: leadID, Actor: actor.SystemID, Source: lead.ProfileFromForm, Profile: sub.Profile,
		})
		switch {
		case lead.IsInputRefusal(err):
			out.Refused = append(out.Refused, refusalCode(err))
		case err != nil:
			return Outcome{}, fmt.Errorf("lead %s from form submission %s: %w", leadID, sub.ID, err)
		default:
			out.Changed, out.Conflicts = result.Changed, result.Conflicts
		}
	}
	if sub.Consent == nil {
		return out, nil
	}
	recorded, err := s.deps.Consents.Record(ctx, sub.WorkspaceID, leadID, sub.Consent.grant(sub.ID, sub.SubmittedAt))
	switch {
	case lead.IsInputRefusal(err):
		out.Refused = append(out.Refused, refusalCode(err))
	case err != nil:
		return Outcome{}, fmt.Errorf("consent of lead %s from form submission %s: %w", leadID, sub.ID, err)
	case recorded.Changed:
		out.Consent = ConsentGranted
	default:
		out.Consent = ConsentUnchanged
	}
	return out, nil
}

func (s *Service) Trigger(ctx context.Context, in Trigger) error {
	if !in.Form.Kind.Valid() {
		return ErrSubmissionIncomplete
	}
	return s.deps.Triggers.FormSubmitted(ctx, in)
}

type resolvedLead struct {
	id      string
	created bool
	owner   string
}

func (s *Service) resolveLead(ctx context.Context, sub Submission) (resolvedLead, error) {
	identity := sub.Identity
	if id := strings.TrimSpace(identity.LeadID); id != "" {
		return resolvedLead{id: id}, nil
	}
	if phone := strings.TrimSpace(identity.Phone); phone != "" {
		found, created, err := s.deps.Phones.FindOrCreate(sub.WorkspaceID, phone, lead.LeadUpdate{Source: lead.SourceForm})
		if err != nil {
			return resolvedLead{}, err
		}
		return resolvedOf(found, created)
	}
	if email := strings.ToLower(strings.TrimSpace(identity.Email)); email != "" {
		if err := lead.ValidateEmail(email); err != nil {
			return resolvedLead{}, err
		}
		found, err := s.deps.Emails.FindOrCreateManyByEmail(ctx, sub.WorkspaceID, []lead.BulkEmailInput{{Source: lead.SourceForm, Email: email}})
		if err != nil {
			return resolvedLead{}, err
		}
		return resolvedOf(found[email], false)
	}
	return resolvedLead{}, lead.ErrLeadIdentityRequired
}

func resolvedOf(found *lead.Lead, created bool) (resolvedLead, error) {
	if found == nil || found.ID == "" {
		return resolvedLead{}, ErrLeadNotResolved
	}
	return resolvedLead{id: found.ID, created: created, owner: strings.TrimSpace(found.Owner)}, nil
}

func refusalCode(err error) string {
	if code := lead.ErrorCode(err); code != "" {
		return code
	}
	return "lead_refused"
}
