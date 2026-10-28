package siteforms

import (
	"context"
	"errors"
	"strings"

	"vozko/domain/lead"
	pfdomain "vozko/domain/publicform"
	"vozko/domain/siteform"
	"vozko/domain/sitesubmission"
	"vozko/usecases/formlead"
	"vozko/usecases/publicform"
)

type Visitor struct {
	Origin    string
	IP        string
	UserAgent string
}

type SubmitResult struct {
	SubmissionID string
	Success      sitesubmission.SuccessAction
}

func (s *Service) publishedOrigins(ctx context.Context, projectID string) (pfdomain.OriginAllowList, error) {
	hosts, err := s.d.Hostnames.ActiveHostnames(ctx, projectID)
	if err != nil {
		return pfdomain.OriginAllowList{}, err
	}
	origins := make([]string, 0, len(hosts))
	for _, host := range hosts {
		origins = append(origins, s.cfg.Addresses.Origins.Of(host))
	}
	return pfdomain.AllowExactOrigins(origins), nil
}

func (s *Service) admitted(ctx context.Context, formKey, origin string) (*ActiveForm, error) {
	if !sitesubmission.ValidFormKey(formKey) {
		return nil, sitesubmission.ErrFormUnknown
	}
	if formKey == sitesubmission.PreviewFormKey {
		if !s.cfg.Addresses.PreviewOrigin(origin) {
			return nil, sitesubmission.ErrOriginRefused
		}
		return nil, nil
	}
	form, err := s.d.Forms.ActiveForm(ctx, formKey)
	if err != nil {
		return nil, err
	}
	if form == nil {
		return nil, sitesubmission.ErrFormUnknown
	}
	allowed, err := s.publishedOrigins(ctx, form.ProjectID)
	if err != nil {
		return nil, err
	}
	if !allowed.Allows(origin) {
		return nil, sitesubmission.ErrOriginRefused
	}
	return form, nil
}

func (s *Service) AllowedOrigin(ctx context.Context, formKey, origin string) (bool, error) {
	_, err := s.admitted(ctx, formKey, origin)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, sitesubmission.ErrOriginRefused), errors.Is(err, sitesubmission.ErrFormUnknown):
		return false, nil
	}
	return false, err
}

func (s *Service) limitVisitor(scope, formKey string, v Visitor) error {
	ip := sitesubmission.HashIP(s.cfg.IPHashKey, v.IP)
	if formKey == sitesubmission.PreviewFormKey {
		return publicform.Allow(s.d.PerPreview, "preview:"+scope+":"+ip)
	}
	return publicform.Allow(s.d.PerIP, scope+":"+ip)
}

func (s *Service) Challenge(ctx context.Context, formKey string, v Visitor) (pfdomain.Challenge, error) {
	if _, err := s.admitted(ctx, formKey, v.Origin); err != nil {
		return pfdomain.Challenge{}, err
	}
	if err := s.limitVisitor("challenge", formKey, v); err != nil {
		return pfdomain.Challenge{}, err
	}
	return s.gate().Issue(formKey, s.now())
}

func (s *Service) Submit(ctx context.Context, formKey string, v Visitor, raw []byte) (SubmitResult, error) {
	form, err := s.admitted(ctx, formKey, v.Origin)
	if err != nil {
		return SubmitResult{}, err
	}
	if err := s.limitVisitor("submit", formKey, v); err != nil {
		return SubmitResult{}, err
	}
	body, err := sitesubmission.DecodeBody(raw)
	if err != nil {
		return SubmitResult{}, err
	}
	if pfdomain.CheckHoneypot(body.Website) != nil {
		return SubmitResult{}, sitesubmission.ErrBotSuspected
	}
	now := s.now()
	proof, err := s.gate().Admit(body.ChallengeToken, body.Nonce, formKey, now)
	if err != nil {
		return SubmitResult{}, err
	}
	if pfdomain.CheckFillTime(proof.IssuedAt, now, pfdomain.DefaultMinimumFillTime) != nil {
		return SubmitResult{}, sitesubmission.ErrBotSuspected
	}
	if form == nil {
		return SubmitResult{Success: sitesubmission.SuccessFor(nil, "")}, nil
	}
	if err := publicform.Allow(s.d.PerForm, formKey); err != nil {
		return SubmitResult{}, err
	}
	values, err := validate(form.Manifest, body)
	if err != nil {
		return SubmitResult{}, err
	}
	success := sitesubmission.SuccessFor(form.Settings.Success, form.SuccessURL)
	existing, err := s.d.Submissions.ByIdempotency(ctx, formKey, body.IdempotencyKey)
	if err != nil {
		return SubmitResult{}, err
	}
	if existing != nil {
		return SubmitResult{SubmissionID: existing.ID, Success: success}, nil
	}
	record := s.record(*form, body, values, v)
	outcome, err := s.writeLead(ctx, *form, record, values)
	if err != nil {
		return SubmitResult{}, err
	}
	record.LeadID = outcome.LeadID
	effects := effectsOf(*form, record, outcome, values, v)
	stored, created, err := s.d.Submissions.Create(ctx, record, effects)
	if err != nil {
		return SubmitResult{}, err
	}
	if created && !effects.Finished() {
		s.startEffects(ctx, stored.ID)
	}
	return SubmitResult{SubmissionID: stored.ID, Success: success}, nil
}

func effectsOf(form ActiveForm, record sitesubmission.Submission, outcome formlead.Outcome, values siteform.Values, v Visitor) sitesubmission.EffectRun {
	effectValues := sitesubmission.EffectValues{Name: values.Name, Phone: values.Phone, Email: values.Email, Consent: values.Consent, EmailOptIn: values.EmailOptIn}
	plan := sitesubmission.EffectPlan{Settings: form.Settings, Values: effectValues, LeadOwner: outcome.Owner, SourceURL: strings.TrimRight(v.Origin, "/") + record.Route}
	pending := sitesubmission.PlanEffects(sitesubmission.EffectPlanInput{
		LeadID: outcome.LeadID, LeadCreated: outcome.Created, LeadOwner: outcome.Owner, Settings: form.Settings, Values: effectValues,
	})
	return sitesubmission.NewEffectRun(record, plan, pending)
}

func validate(m siteform.Manifest, body sitesubmission.Body) (siteform.Values, error) {
	raw, problems := body.Values()
	for _, p := range problems {
		delete(raw, p.Field)
	}
	values, err := m.Validate(raw)
	var invalid *siteform.SubmissionError
	switch {
	case errors.As(err, &invalid):
		problems = append(problems, invalid.Problems...)
	case err != nil:
		return siteform.Values{}, err
	}
	if len(problems) > 0 {
		return siteform.Values{}, &sitesubmission.FieldsError{Details: sitesubmission.Details(problems, m)}
	}
	return values, nil
}

func (s *Service) record(form ActiveForm, body sitesubmission.Body, values siteform.Values, v Visitor) sitesubmission.Submission {
	fields, _ := body.Values()
	return sitesubmission.Submission{
		ID: s.d.NewID(), WorkspaceID: form.WorkspaceID, ProjectID: form.ProjectID, DeploymentID: form.DeploymentID,
		FormKey: form.FormKey, FormID: form.FormID, Route: body.Route, Variant: body.Variant, Fields: fields, UTM: body.UTM,
		ReferrerOrigin: sitesubmission.ReferrerOrigin(body.Referrer), Fbclid: body.Fbclid, Gclid: body.Gclid, Fbp: body.Fbp, Fbc: body.Fbc,
		Consent: sitesubmission.ConsentOf(form.Manifest, values), IPHash: sitesubmission.HashIP(s.cfg.IPHashKey, v.IP),
		UserAgent: sitesubmission.TruncateUserAgent(v.UserAgent), IdempotencyKey: body.IdempotencyKey, CreatedAt: s.now(),
	}
}

func (s *Service) writeLead(ctx context.Context, form ActiveForm, record sitesubmission.Submission, values siteform.Values) (formlead.Outcome, error) {
	sub := formlead.Submission{
		WorkspaceID: form.WorkspaceID, Form: formlead.Form{Kind: formlead.KindSiteForm, ID: form.FormKey}, ID: record.ID,
		Identity: formlead.Identity{Phone: values.Phone, Email: values.Email},
		Profile:  lead.Profile{Name: values.Name, Email: values.Email}, SubmittedAt: record.CreatedAt,
	}
	if len(values.Fields) > 0 {
		sub.Profile.CustomFields = values.Fields
	}
	if values.Consent {
		sub.Consent = &formlead.Consent{Text: form.Manifest.ConsentText}
	}
	out, err := s.d.Leads.Write(ctx, sub)
	if errors.Is(err, lead.ErrLeadIdentityRequired) {
		return formlead.Outcome{}, nil
	}
	return out, err
}
