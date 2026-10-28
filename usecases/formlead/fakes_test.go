package formlead

import (
	"context"
	"strings"

	"vozko/domain/lead"
	lead_usecase "vozko/usecases/lead"
)

type phoneCall struct {
	workspaceID string
	number      string
	update      lead.LeadUpdate
}

type emailCall struct {
	workspaceID string
	inputs      []lead.BulkEmailInput
}

type recorder struct {
	phones     []phoneCall
	phoneErr   error
	emails     []emailCall
	emailErr   error
	emailLost  bool
	profiles   []lead_usecase.ProfileUpdate
	profileOut lead_usecase.ProfileResult
	profileErr error
	consents   []lead.ConsentChange
	consentOut lead_usecase.ConsentOutcome
	consentErr error
	triggers   []Trigger
	triggerErr error
	existing   bool
	owner      string
}

func (r *recorder) FindOrCreate(workspaceID, number string, update lead.LeadUpdate) (*lead.Lead, bool, error) {
	r.phones = append(r.phones, phoneCall{workspaceID: workspaceID, number: number, update: update})
	if r.phoneErr != nil {
		return nil, false, r.phoneErr
	}
	return &lead.Lead{ID: "lead-phone", WorkspaceID: workspaceID, Number: number, Owner: r.owner}, !r.existing, nil
}

func (r *recorder) FindOrCreateManyByEmail(_ context.Context, workspaceID string, inputs []lead.BulkEmailInput) (map[string]*lead.Lead, error) {
	r.emails = append(r.emails, emailCall{workspaceID: workspaceID, inputs: inputs})
	if r.emailErr != nil {
		return nil, r.emailErr
	}
	out := map[string]*lead.Lead{}
	if r.emailLost {
		return out, nil
	}
	for _, in := range inputs {
		address := strings.ToLower(strings.TrimSpace(in.Email))
		out[address] = &lead.Lead{ID: "lead-email", WorkspaceID: workspaceID, Email: address, Owner: r.owner}
	}
	return out, nil
}

func (r *recorder) Update(_ context.Context, in lead_usecase.ProfileUpdate) (lead_usecase.ProfileResult, error) {
	r.profiles = append(r.profiles, in)
	return r.profileOut, r.profileErr
}

func (r *recorder) Record(_ context.Context, _, _ string, change lead.ConsentChange) (lead_usecase.ConsentOutcome, error) {
	r.consents = append(r.consents, change)
	return r.consentOut, r.consentErr
}

func (r *recorder) FormSubmitted(_ context.Context, in Trigger) error {
	r.triggers = append(r.triggers, in)
	return r.triggerErr
}

func newService(r *recorder) *Service {
	svc, err := New(Deps{Phones: r, Emails: r, Profiles: r, Consents: r, Triggers: r})
	if err != nil {
		panic(err)
	}
	return svc
}
