package siteforms

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"vozko/domain/emailconsent"
	"vozko/domain/notification"
	pfdomain "vozko/domain/publicform"
	"vozko/domain/site"
	"vozko/domain/sitesubmission"
	"vozko/domain/workspace"
	"vozko/usecases/formlead"
)

var visitor = Visitor{Origin: "https://loja.vozko.site", IP: "203.0.113.7", UserAgent: "Mozilla/5.0"}

func solve(t *testing.T, c pfdomain.Challenge) string {
	t.Helper()
	for n := 0; n < 1<<24; n++ {
		if pfdomain.SolvesChallenge(c.Token, strconv.Itoa(n), c.Bits) {
			return strconv.Itoa(n)
		}
	}
	t.Fatal("no solution")
	return ""
}

func (w *world) body(t *testing.T, key string, fields map[string]any, mutate ...func(map[string]any)) []byte {
	t.Helper()
	challenge, err := w.svc.Challenge(context.Background(), key, visitor)
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	w.clock = w.clock.Add(4 * time.Second)
	body := map[string]any{
		"utm":      map[string]string{"source": "meta", "medium": "cpc", "campaign": "natal", "term": "", "content": ""},
		"referrer": "https://l.facebook.com/l.php?u=x", "fbclid": "abc", "gclid": "", "fbp": "fb.1.1.2", "fbc": "fb.1.1.abc",
		"variant": "b", "idempotencyKey": "0f8e4b1a-1c2d-4e5f-8a9b-0c1d2e3f4a5b", "website": "", "route": "/oferta",
		"fields": fields, "challengeToken": challenge.Token, "nonce": solve(t, challenge),
	}
	for _, m := range mutate {
		m(body)
	}
	raw, _ := json.Marshal(body)
	return raw
}

func goodFields() map[string]any {
	return map[string]any{"lead.name": "Ana", "lead.phone": "+55 84 99999-0000", "lead.email": "ana@example.com", "field.plano": "pro", "consent": "on", "email_opt_in": "on"}
}

func TestASubmissionCreatesTheLeadAndStoresTheEvidence(t *testing.T) {
	w := newWorld(t)
	key := w.publish(t, site.FormSettings{Success: &site.Success{Kind: site.SuccessMessage, Message: "Obrigado!"}})
	result, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields()))
	if err != nil {
		t.Fatal(err)
	}
	if result.Success.Kind != site.SuccessMessage || result.Success.Message != "Obrigado!" || result.SubmissionID != "sub-1" {
		t.Fatalf("result %+v", result)
	}
	write := w.leads.writes[0]
	if write.Form != (formlead.Form{Kind: formlead.KindSiteForm, ID: key}) || write.Identity.Phone != "5584999990000" || write.Identity.Email != "ana@example.com" ||
		write.Profile.Name != "Ana" || write.Profile.CustomFields["plano"] != "pro" || write.Consent == nil || write.Consent.Text != "Aceito ser contatado" || write.ID != "sub-1" {
		t.Fatalf("lead write %+v", write)
	}
	row := w.subs.rows[0]
	if row.LeadID != "lead-1" || row.ProjectID != "p1" || row.DeploymentID != "d1" || row.FormID != "contato" || row.Route != "/oferta" || row.Variant != "b" ||
		row.UTM.Campaign != "natal" || row.ReferrerOrigin != "https://l.facebook.com" || row.Fbclid != "abc" || row.Fbp != "fb.1.1.2" || row.Fbc != "fb.1.1.abc" ||
		!row.Consent.Granted || row.Consent.Text != "Aceito ser contatado" || !row.Consent.EmailOptIn || row.UserAgent != "Mozilla/5.0" {
		t.Fatalf("row %+v", row)
	}
	if row.IPHash == "" || strings.Contains(row.IPHash, "203.0.113.7") {
		t.Fatalf("ip hash %q", row.IPHash)
	}
	if len(w.optIns.got) != 1 || w.optIns.got[0].Source != emailconsent.SourceSiteForm || w.optIns.got[0].IP != row.IPHash || w.optIns.got[0].ConsentedAt == nil || !w.optIns.got[0].ConsentedAt.Equal(row.CreatedAt) || w.optIns.got[0].LeadID != "lead-1" {
		t.Fatalf("opt ins %+v", w.optIns.got)
	}
	if len(w.conversions.sent) != 1 || w.conversions.sent[0].SubmissionID != "sub-1" || w.conversions.sent[0].SourceURL != "https://loja.vozko.site/oferta" || w.conversions.sent[0].Fbc != "fb.1.1.abc" {
		t.Fatalf("conversions %+v", w.conversions.sent)
	}
	if len(w.leads.triggers) != 0 || len(w.notices.drafts) != 0 || len(w.deals.created) != 0 || len(w.owners.set) != 0 {
		t.Fatal("nothing else was configured")
	}
}

func TestTheSameSubmissionTwiceAnswersTheOriginalSuccess(t *testing.T) {
	w := newWorld(t)
	key := w.publish(t, site.FormSettings{Notify: true})
	first, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields()))
	if err != nil {
		t.Fatal(err)
	}
	again, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields()))
	if err != nil || again.SubmissionID != first.SubmissionID {
		t.Fatalf("again %+v err %v", again, err)
	}
	if len(w.subs.rows) != 1 || len(w.leads.writes) != 1 || len(w.notices.drafts) != 1 {
		t.Fatalf("rows %d writes %d notices %d", len(w.subs.rows), len(w.leads.writes), len(w.notices.drafts))
	}
}

func TestOwnerFunnelAndNotificationFollowTheFormSettings(t *testing.T) {
	w := newWorld(t)
	key := w.publish(t, site.FormSettings{FunnelID: "pipe1", Owner: &site.Owner{Kind: site.OwnerRoulette, ID: "dept-1"}, Notify: true})
	if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields())); err != nil {
		t.Fatal(err)
	}
	if len(w.roulette.asks) != 1 || w.roulette.asks[0] != "dept-1" || len(w.owners.set) != 1 || w.owners.set[0] != "system|lead-1|user-7" {
		t.Fatalf("roulette %v owners %v", w.roulette.asks, w.owners.set)
	}
	deal := w.deals.created[0]
	if deal.LeadID != "lead-1" || deal.PipelineID != "pipe1" || deal.StageID != "pipe1-first" || deal.OwnerID != "user-7" || deal.Actor != "system" || deal.Source != "site_form" {
		t.Fatalf("deal %+v", deal)
	}
	draft := w.notices.drafts[0]
	if draft.Category != notification.CategoryLeads || draft.WorkspaceID != "w1" || draft.DedupKey == "" || draft.Audience.Permission == nil || draft.Audience.Permission.Resource != workspace.ResourceSiteSubmissions {
		t.Fatalf("draft %+v", draft)
	}
}

func TestADealWithoutAnyOwnerIsNotCreated(t *testing.T) {
	w := newWorld(t)
	key := w.publish(t, site.FormSettings{FunnelID: "pipe1", StageID: "stage-2"})
	if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields())); err != nil {
		t.Fatal(err)
	}
	if len(w.deals.created) != 0 {
		t.Fatalf("deals %+v", w.deals.created)
	}
	w2 := newWorld(t)
	user := w2.publish(t, site.FormSettings{FunnelID: "pipe1", StageID: "stage-2", Owner: &site.Owner{Kind: site.OwnerUser, ID: "user-3"}})
	if _, err := w2.svc.Submit(context.Background(), user, visitor, w2.body(t, user, goodFields())); err != nil {
		t.Fatal(err)
	}
	if len(w2.deals.created) != 1 || w2.deals.created[0].StageID != "stage-2" || w2.deals.created[0].OwnerID != "user-3" || len(w2.roulette.asks) != 0 {
		t.Fatalf("deals %+v", w2.deals.created)
	}
}

func TestAWorkflowFormOpensTheConversationOnTheChosenChannelAndFires(t *testing.T) {
	cases := map[string]struct {
		followUp  site.FollowUp
		entryID   string
		entryType string
	}{
		"official whatsapp":   {site.FollowUp{Kind: site.FollowUpWhatsApp, ID: "bp-1"}, "entry-official", "whatsapp"},
		"unofficial whatsapp": {site.FollowUp{Kind: site.FollowUpUnofficialWhatsApp, ID: "inst-1"}, "conv-unofficial", "unofficial_whatsapp"},
		"email inbox":         {site.FollowUp{Kind: site.FollowUpEmail, ID: "sender-1"}, "conv-email", "email"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			followUp := tc.followUp
			key := w.publish(t, site.FormSettings{WorkflowID: "wf-1", FollowUp: &followUp})
			if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields())); err != nil {
				t.Fatal(err)
			}
			if len(w.leads.triggers) != 1 {
				t.Fatalf("triggers %+v", w.leads.triggers)
			}
			trigger := w.leads.triggers[0]
			if trigger.EntryID != tc.entryID || trigger.EntryType != tc.entryType || trigger.Form.Kind != formlead.KindSiteForm || trigger.Form.ID != key || trigger.SubmissionID != "sub-1" || trigger.WorkspaceID != "w1" {
				t.Fatalf("trigger %+v", trigger)
			}
			if trigger.Data["route"] != "/oferta" || trigger.Data["variant"] != "b" || trigger.Data["form_id"] != "contato" || trigger.Data["project_id"] != "p1" {
				t.Fatalf("trigger data %+v", trigger.Data)
			}
			if utm, _ := trigger.Data["utm"].(map[string]string); utm["campaign"] != "natal" {
				t.Fatalf("trigger utm %+v", trigger.Data["utm"])
			}
			if w.subs.attached["sub-1"] != tc.entryType+":"+tc.entryID {
				t.Fatalf("attached %v", w.subs.attached)
			}
		})
	}
	w := newWorld(t)
	email := site.FollowUp{Kind: site.FollowUpEmail, ID: "sender-1"}
	key := w.publish(t, site.FormSettings{WorkflowID: "wf-1", FollowUp: &email})
	_, _ = w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields()))
	if got := w.email.got[0]; got.SenderID != "sender-1" || got.Address != "ana@example.com" || got.LeadID != "lead-1" || got.WorkspaceID != "w1" {
		t.Fatalf("email open %+v", got)
	}
	if w.official.got != nil || w.unofficial.got != nil {
		t.Fatal("only the chosen channel is opened")
	}
}

func TestWithoutAWorkflowNoConversationIsOpened(t *testing.T) {
	w := newWorld(t)
	followUp := site.FollowUp{Kind: site.FollowUpWhatsApp, ID: "bp-1"}
	key := w.publish(t, site.FormSettings{FollowUp: &followUp})
	if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields())); err != nil {
		t.Fatal(err)
	}
	if len(w.official.got) != 0 || len(w.leads.triggers) != 0 {
		t.Fatal("no workflow, no conversation")
	}
}

func TestInvalidFieldsAnswerWithTheFieldsToFix(t *testing.T) {
	w := newWorld(t)
	key := w.publish(t, site.FormSettings{})
	fields := map[string]any{"lead.name": "", "lead.phone": "123", "field.plano": []string{"a", "b"}, "extra": "x"}
	_, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, fields))
	var fieldsErr *sitesubmission.FieldsError
	if !errors.As(err, &fieldsErr) {
		t.Fatalf("err = %v", err)
	}
	for _, name := range []string{"lead.name", "lead.phone", "field.plano", "extra", "consent"} {
		if fieldsErr.Details[name] == "" {
			t.Fatalf("details %v miss %s", fieldsErr.Details, name)
		}
	}
	if len(w.subs.rows) != 0 || len(w.leads.writes) != 0 {
		t.Fatal("nothing is written for an invalid submission")
	}
}

func TestPublicFormGuardsRefuseBeforeAnythingIsWritten(t *testing.T) {
	w := newWorld(t)
	key := w.publish(t, site.FormSettings{})
	ctx := context.Background()
	if _, err := w.svc.Challenge(ctx, key, Visitor{Origin: "https://evil.example", IP: "1.1.1.1"}); !errors.Is(err, sitesubmission.ErrOriginRefused) {
		t.Fatalf("foreign origin challenge = %v", err)
	}
	if _, err := w.svc.Challenge(ctx, key, Visitor{Origin: "https://sub.loja.vozko.site", IP: "1.1.1.1"}); !errors.Is(err, sitesubmission.ErrOriginRefused) {
		t.Fatalf("subdomain challenge = %v", err)
	}
	if _, err := w.svc.Challenge(ctx, "unknown-key", visitor); !errors.Is(err, sitesubmission.ErrFormUnknown) {
		t.Fatalf("unknown form = %v", err)
	}
	if _, err := w.svc.Challenge(ctx, "broken", visitor); err == nil || errors.Is(err, sitesubmission.ErrFormUnknown) {
		t.Fatalf("an unreadable store must fail, not answer unknown: %v", err)
	}
	cases := map[string]func(*world) error{
		"honeypot": func(w *world) error {
			_, err := w.svc.Submit(ctx, key, visitor, w.body(t, key, goodFields(), func(b map[string]any) { b["website"] = "spam" }))
			return expect(err, sitesubmission.ErrBotSuspected)
		},
		"too fast": func(w *world) error {
			challenge, _ := w.svc.Challenge(ctx, key, visitor)
			raw, _ := json.Marshal(map[string]any{"utm": map[string]string{}, "idempotencyKey": "k-12345678", "route": "/", "fields": goodFields(), "challengeToken": challenge.Token, "nonce": solve(t, challenge)})
			_, err := w.svc.Submit(ctx, key, visitor, raw)
			return expect(err, sitesubmission.ErrBotSuspected)
		},
		"reused challenge": func(w *world) error {
			raw := w.body(t, key, goodFields())
			if _, err := w.svc.Submit(ctx, key, visitor, raw); err != nil {
				return err
			}
			_, err := w.svc.Submit(ctx, key, visitor, raw)
			return expect(err, pfdomain.ErrChallengeReused)
		},
		"challenge of another form": func(w *world) error {
			other := w.svc
			challenge, _ := other.gate().Issue("other-form", w.clock)
			w.clock = w.clock.Add(4 * time.Second)
			raw, _ := json.Marshal(map[string]any{"utm": map[string]string{}, "idempotencyKey": "k-12345678", "route": "/", "fields": goodFields(), "challengeToken": challenge.Token, "nonce": solve(t, challenge)})
			_, err := w.svc.Submit(ctx, key, visitor, raw)
			return expect(err, pfdomain.ErrChallengeInvalid)
		},
		"strict body": func(w *world) error {
			_, err := w.svc.Submit(ctx, key, visitor, []byte(`{"fields":{}}`))
			return expect(err, sitesubmission.ErrBodyInvalid)
		},
		"foreign origin": func(w *world) error {
			_, err := w.svc.Submit(ctx, key, Visitor{Origin: "https://evil.example", IP: "1.1.1.1"}, w.body(t, key, goodFields()))
			return expect(err, sitesubmission.ErrOriginRefused)
		},
		"limited ip": func(w *world) error {
			raw := w.body(t, key, goodFields())
			w.ipLimit.deny = map[string]bool{"submit:" + sitesubmission.HashIP(w.svc.cfg.IPHashKey, visitor.IP): true}
			_, err := w.svc.Submit(ctx, key, visitor, raw)
			return expect(err, pfdomain.ErrRateLimited)
		},
		"limited form": func(w *world) error {
			raw := w.body(t, key, goodFields())
			w.formLimit.deny = map[string]bool{key: true}
			_, err := w.svc.Submit(ctx, key, visitor, raw)
			return expect(err, pfdomain.ErrRateLimited)
		},
		"limiter down": func(w *world) error {
			raw := w.body(t, key, goodFields())
			w.formLimit.err = errors.New("redis down")
			_, err := w.svc.Submit(ctx, key, visitor, raw)
			return expect(err, pfdomain.ErrRateLimited)
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			key = w.publish(t, site.FormSettings{})
			if err := run(w); err != nil {
				t.Fatal(err)
			}
			if name != "reused challenge" && (len(w.subs.rows) != 0 || len(w.leads.writes) != 0) {
				t.Fatal("a refused submission wrote something")
			}
		})
	}
}

func expect(err, want error) error {
	if errors.Is(err, want) {
		return nil
	}
	return errors.New("got " + errString(err) + ", want " + want.Error())
}

func errString(err error) string {
	if err == nil {
		return "nil"
	}
	return err.Error()
}

func TestAPreviewFormValidatesAndWritesNothing(t *testing.T) {
	w := newWorld(t)
	preview := Visitor{Origin: "https://preview--abc.paginas.vozko.dev", IP: "203.0.113.7", UserAgent: "x"}
	challenge, err := w.svc.Challenge(context.Background(), sitesubmission.PreviewFormKey, preview)
	if err != nil {
		t.Fatal(err)
	}
	w.clock = w.clock.Add(4 * time.Second)
	raw, _ := json.Marshal(map[string]any{"utm": map[string]string{}, "idempotencyKey": "k-12345678", "route": "/", "fields": goodFields(), "challengeToken": challenge.Token, "nonce": solve(t, challenge)})
	result, err := w.svc.Submit(context.Background(), sitesubmission.PreviewFormKey, preview, raw)
	if err != nil || result.Success.Kind != site.SuccessMessage {
		t.Fatalf("result %+v err %v", result, err)
	}
	if len(w.subs.rows) != 0 || len(w.leads.writes) != 0 || len(w.conversions.sent) != 0 {
		t.Fatal("a preview wrote something")
	}
	if _, err := w.svc.Challenge(context.Background(), sitesubmission.PreviewFormKey, visitor); !errors.Is(err, sitesubmission.ErrOriginRefused) {
		t.Fatalf("a published page cannot use the preview sink: %v", err)
	}
	for _, origin := range []string{"https://loja.paginas.vozko.dev", "https://deploy--abc.paginas.vozko.dev", "http://preview--abc.paginas.vozko.dev", "https://preview--abc.evil.dev"} {
		if _, err := w.svc.Challenge(context.Background(), sitesubmission.PreviewFormKey, Visitor{Origin: origin, IP: "203.0.113.7"}); !errors.Is(err, sitesubmission.ErrOriginRefused) {
			t.Fatalf("%s cannot use the preview sink: %v", origin, err)
		}
	}
}

func TestAllowedOriginAnswersPreflights(t *testing.T) {
	w := newWorld(t)
	key := w.publish(t, site.FormSettings{})
	if ok, err := w.svc.AllowedOrigin(context.Background(), key, "https://www.loja.com.br"); !ok || err != nil {
		t.Fatalf("published origin = %v %v", ok, err)
	}
	if ok, _ := w.svc.AllowedOrigin(context.Background(), key, "https://evil.example"); ok {
		t.Fatal("foreign origin allowed")
	}
	if ok, err := w.svc.AllowedOrigin(context.Background(), "broken", "https://www.loja.com.br"); ok || err == nil {
		t.Fatalf("broken store = %v %v", ok, err)
	}
}

func TestSubmissionsAreListedPerProject(t *testing.T) {
	w := newWorld(t)
	key := w.publish(t, site.FormSettings{})
	if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields())); err != nil {
		t.Fatal(err)
	}
	page, err := w.svc.Submissions(context.Background(), SubmissionQuery{WorkspaceID: "w1", ProjectID: "p1", Page: 1, PageSize: 20})
	if err != nil || page.Total != 1 || page.Items[0].FormID != "contato" {
		t.Fatalf("page %+v err %v", page, err)
	}
	if _, err := w.svc.Submissions(context.Background(), SubmissionQuery{WorkspaceID: "w1", ProjectID: "p1", Page: 1, PageSize: 500}); err == nil {
		t.Fatal("an oversized page must refuse")
	}
}
