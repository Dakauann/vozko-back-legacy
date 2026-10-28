package formlead

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"vozko/domain/actor"
	"vozko/domain/lead"
	"vozko/domain/workflow"
	lead_usecase "vozko/usecases/lead"
)

var (
	ctx         = context.Background()
	submittedAt = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
)

func submission(mutate func(*Submission)) Submission {
	s := Submission{
		WorkspaceID: "ws-1",
		Form:        Form{Kind: KindSiteForm, ID: "form-1"},
		ID:          "sub-1",
		Identity:    Identity{Phone: "5511987654321"},
		Profile:     lead.Profile{Name: "Ana Souza", Email: "ana@example.com"},
		SubmittedAt: submittedAt,
	}
	if mutate != nil {
		mutate(&s)
	}
	return s
}

func TestNewRefusesMissingDependencies(t *testing.T) {
	r := &recorder{}
	cases := map[string]Deps{
		"no phones":   {Emails: r, Profiles: r, Consents: r, Triggers: r},
		"no emails":   {Phones: r, Profiles: r, Consents: r, Triggers: r},
		"no profiles": {Phones: r, Emails: r, Consents: r, Triggers: r},
		"no consents": {Phones: r, Emails: r, Profiles: r, Triggers: r},
		"no triggers": {Phones: r, Emails: r, Profiles: r, Consents: r},
	}
	for name, deps := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := New(deps); err == nil {
				t.Fatal("a missing dependency must refuse the service")
			}
		})
	}
}

func TestTheLeadIsFoundOrCreatedFromTheIdentity(t *testing.T) {
	cases := []struct {
		name       string
		identity   Identity
		wantLead   string
		wantPhones int
		wantEmails int
	}{
		{"a known lead is used as is", Identity{LeadID: " lead-1 ", Phone: "5511987654321", Email: "ana@example.com"}, "lead-1", 0, 0},
		{"a phone finds or creates", Identity{Phone: "5511987654321"}, "lead-phone", 1, 0},
		{"the phone wins over the email", Identity{Phone: "5511987654321", Email: "ana@example.com"}, "lead-phone", 1, 0},
		{"an email alone finds or creates", Identity{Email: " Ana@Example.com "}, "lead-email", 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &recorder{}
			out, err := newService(r).Write(ctx, submission(func(s *Submission) { s.Identity = tc.identity }))
			if err != nil {
				t.Fatal(err)
			}
			if out.LeadID != tc.wantLead || len(r.phones) != tc.wantPhones || len(r.emails) != tc.wantEmails {
				t.Fatalf("lead = %q phones = %d emails = %d", out.LeadID, len(r.phones), len(r.emails))
			}
			if len(r.profiles) != 1 || r.profiles[0].LeadID != tc.wantLead {
				t.Fatalf("profile updates = %+v", r.profiles)
			}
		})
	}
}

func TestLeadsCreatedByAFormComeFromTheForm(t *testing.T) {
	r := &recorder{}
	svc := newService(r)
	if _, err := svc.Write(ctx, submission(nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Write(ctx, submission(func(s *Submission) { s.Identity = Identity{Email: " Ana@Example.com "} })); err != nil {
		t.Fatal(err)
	}
	wantPhone := phoneCall{workspaceID: "ws-1", number: "5511987654321", update: lead.LeadUpdate{Source: lead.SourceForm}}
	if len(r.phones) != 1 || !reflect.DeepEqual(r.phones[0], wantPhone) {
		t.Fatalf("phone lookups = %+v", r.phones)
	}
	wantEmail := emailCall{workspaceID: "ws-1", inputs: []lead.BulkEmailInput{{Source: lead.SourceForm, Email: "ana@example.com"}}}
	if len(r.emails) != 1 || !reflect.DeepEqual(r.emails[0], wantEmail) {
		t.Fatalf("email lookups = %+v", r.emails)
	}
}

func TestWriteRefusesWhatCannotBecomeALead(t *testing.T) {
	cases := []struct {
		name  string
		shape func(*Submission)
		setup func(*recorder)
		want  error
	}{
		{name: "no identity", shape: func(s *Submission) { s.Identity = Identity{Phone: " ", Email: " "} }, want: lead.ErrLeadIdentityRequired},
		{name: "an invalid email", shape: func(s *Submission) { s.Identity = Identity{Email: "not-an-email"} }, want: lead.ErrLeadEmailInvalid},
		{name: "an invalid phone", setup: func(r *recorder) { r.phoneErr = lead.ErrLeadInvalid }, want: lead.ErrLeadInvalid},
		{name: "no workspace", shape: func(s *Submission) { s.WorkspaceID = " " }, want: ErrSubmissionIncomplete},
		{name: "no form", shape: func(s *Submission) { s.Form.ID = "" }, want: ErrSubmissionIncomplete},
		{name: "an unknown form kind", shape: func(s *Submission) { s.Form.Kind = "fax" }, want: ErrSubmissionIncomplete},
		{name: "no submission id", shape: func(s *Submission) { s.ID = " " }, want: ErrSubmissionIncomplete},
		{name: "no submission time", shape: func(s *Submission) { s.SubmittedAt = time.Time{} }, want: ErrSubmissionIncomplete},
		{name: "the email store fails", shape: func(s *Submission) { s.Identity = Identity{Email: "ana@example.com"} }, setup: func(r *recorder) { r.emailErr = errDatabase }, want: errDatabase},
		{name: "the email store returns no lead", shape: func(s *Submission) { s.Identity = Identity{Email: "ana@example.com"} }, setup: func(r *recorder) { r.emailLost = true }, want: ErrLeadNotResolved},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &recorder{}
			if tc.setup != nil {
				tc.setup(r)
			}
			_, err := newService(r).Write(ctx, submission(tc.shape))
			if !errors.Is(err, tc.want) {
				t.Fatalf("Write = %v, want %v", err, tc.want)
			}
			if len(r.profiles)+len(r.consents)+len(r.triggers) != 0 {
				t.Fatal("a refused submission writes nothing to the lead")
			}
		})
	}
}

var errDatabase = errors.New("database down")

func TestTheProfileIsUpdatedFromTheForm(t *testing.T) {
	r := &recorder{profileOut: lead_usecase.ProfileResult{Changed: []string{"name"}, Conflicts: []string{"email"}}}
	out, err := newService(r).Write(ctx, submission(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := lead_usecase.ProfileUpdate{
		WorkspaceID: "ws-1", LeadID: "lead-phone", Actor: actor.SystemID, Source: lead.ProfileFromForm,
		Profile: lead.Profile{Name: "Ana Souza", Email: "ana@example.com"},
	}
	if len(r.profiles) != 1 || !reflect.DeepEqual(r.profiles[0], want) {
		t.Fatalf("profile updates = %+v", r.profiles)
	}
	if !reflect.DeepEqual(out.Changed, []string{"name"}) || !reflect.DeepEqual(out.Conflicts, []string{"email"}) || len(out.Refused) != 0 {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestAnEmptyProfileIsNotWritten(t *testing.T) {
	r := &recorder{}
	out, err := newService(r).Write(ctx, submission(func(s *Submission) { s.Profile = lead.Profile{} }))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.profiles) != 0 || out.LeadID != "lead-phone" {
		t.Fatalf("profiles = %+v outcome = %+v", r.profiles, out)
	}
}

func TestConsentIsGrantedWithTheSubmissionAsEvidence(t *testing.T) {
	cases := []struct {
		name        string
		consentOut  lead_usecase.ConsentOutcome
		wantConsent string
	}{
		{"a new grant", lead_usecase.ConsentOutcome{Changed: true}, ConsentGranted},
		{"an existing grant", lead_usecase.ConsentOutcome{Changed: false}, ConsentUnchanged},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &recorder{consentOut: tc.consentOut}
			out, err := newService(r).Write(ctx, submission(func(s *Submission) { s.Consent = &Consent{Text: "  Aceito receber mensagens "} }))
			if err != nil {
				t.Fatal(err)
			}
			want := lead.ConsentChange{
				Kind: lead.ConsentGrant, Source: lead.ConsentForm, Actor: lead.ConsentActor{Type: lead.ConsentActorLead},
				Purpose:  "Aceito receber mensagens",
				Evidence: lead.ConsentEvidence{Type: lead.EvidenceFormSubmission, Ref: "sub-1", Excerpt: "Aceito receber mensagens"},
				At:       submittedAt,
			}
			if len(r.consents) != 1 || !reflect.DeepEqual(r.consents[0], want) {
				t.Fatalf("consents = %+v", r.consents)
			}
			if err := lead.ConsentRules(r.consents[0]); err != nil {
				t.Fatalf("the grant must satisfy the lead consent rules: %v", err)
			}
			if out.Consent != tc.wantConsent {
				t.Fatalf("consent = %q, want %q", out.Consent, tc.wantConsent)
			}
		})
	}
}

func TestWithoutAGrantNoConsentIsRecorded(t *testing.T) {
	r := &recorder{}
	out, err := newService(r).Write(ctx, submission(nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.consents) != 0 || out.Consent != "" {
		t.Fatalf("consents = %+v outcome = %+v", r.consents, out)
	}
}

func TestInputRefusalsAreReportedAndInfrastructureFailuresReturned(t *testing.T) {
	cases := []struct {
		name        string
		setup       func(*recorder)
		wantRefused []string
		wantErr     error
	}{
		{"an invalid profile value", func(r *recorder) { r.profileErr = lead.ErrLeadEmailInvalid }, []string{"lead_email_invalid"}, nil},
		{"an invalid consent", func(r *recorder) { r.consentErr = lead.ErrConsentEvidenceRequired }, []string{lead.ErrorCode(lead.ErrConsentEvidenceRequired)}, nil},
		{"both refused in order", func(r *recorder) {
			r.profileErr, r.consentErr = lead.ErrLeadEmailInvalid, lead.ErrConsentEvidenceRequired
		}, []string{"lead_email_invalid", lead.ErrorCode(lead.ErrConsentEvidenceRequired)}, nil},
		{"the profile store fails", func(r *recorder) { r.profileErr = errDatabase }, nil, errDatabase},
		{"the consent store fails", func(r *recorder) { r.consentErr = errDatabase }, nil, errDatabase},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &recorder{}
			tc.setup(r)
			out, err := newService(r).Write(ctx, submission(func(s *Submission) { s.Consent = &Consent{Text: "Aceito"} }))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Write = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			if strings.Join(out.Refused, ",") != strings.Join(tc.wantRefused, ",") {
				t.Fatalf("refused = %v, want %v", out.Refused, tc.wantRefused)
			}
		})
	}
}

func TestTriggerStartsFormSubmittedWorkflowsWithTheFormIdentity(t *testing.T) {
	r := &recorder{}
	in := Trigger{
		WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: "whatsapp",
		Form: Form{Kind: KindWhatsAppForm, ID: "form-1"}, SubmissionID: "resp-1",
		Data: workflow.FormSubmittedRunData("form-1", "resp-1", map[string]any{"f_nome": "Ana"}),
	}
	if err := newService(r).Trigger(ctx, in); err != nil {
		t.Fatal(err)
	}
	if len(r.triggers) != 1 || !reflect.DeepEqual(r.triggers[0], in) {
		t.Fatalf("triggers = %+v", r.triggers)
	}
}

func TestTriggerRefusesAnUnknownFormKindAndReturnsFailures(t *testing.T) {
	cases := []struct {
		name  string
		kind  Kind
		fails error
		want  error
	}{
		{"unknown kind", "fax", nil, ErrSubmissionIncomplete},
		{"no kind", "", nil, ErrSubmissionIncomplete},
		{"firer failure", KindSiteForm, errDatabase, errDatabase},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &recorder{triggerErr: tc.fails}
			err := newService(r).Trigger(ctx, Trigger{WorkspaceID: "ws-1", EntryID: "e", EntryType: "whatsapp", Form: Form{Kind: tc.kind, ID: "form-1"}, SubmissionID: "s"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("Trigger = %v, want %v", err, tc.want)
			}
			if tc.fails == nil && len(r.triggers) != 0 {
				t.Fatal("an unknown form kind never starts workflows")
			}
		})
	}
}

func TestFormKindsAreKnown(t *testing.T) {
	cases := map[Kind]bool{KindWhatsAppForm: true, KindSiteForm: true, "": false, "fax": false}
	for kind, want := range cases {
		if got := kind.Valid(); got != want {
			t.Errorf("%q.Valid() = %v, want %v", kind, got, want)
		}
	}
}

func TestTheOutcomeTellsWhetherTheFormCreatedTheLeadAndWhoOwnsIt(t *testing.T) {
	cases := map[string]struct {
		recorder    recorder
		identity    Identity
		wantCreated bool
		wantOwner   string
	}{
		"a new phone lead":      {recorder{}, Identity{Phone: "5511987654321"}, true, ""},
		"an owned phone lead":   {recorder{existing: true, owner: "user-2"}, Identity{Phone: "5511987654321"}, false, "user-2"},
		"an unowned phone lead": {recorder{existing: true}, Identity{Phone: "5511987654321"}, false, ""},
		"an owned email lead":   {recorder{owner: "user-3"}, Identity{Email: "ana@example.com"}, false, "user-3"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := tc.recorder
			out, err := newService(&r).Write(ctx, submission(func(s *Submission) { s.Identity = tc.identity }))
			if err != nil {
				t.Fatal(err)
			}
			if out.Created != tc.wantCreated || out.Owner != tc.wantOwner {
				t.Fatalf("outcome %+v", out)
			}
		})
	}
}
