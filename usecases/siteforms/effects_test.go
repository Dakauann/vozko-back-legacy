package siteforms

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"vozko/domain/opportunity"
	pfdomain "vozko/domain/publicform"
	"vozko/domain/site"
	"vozko/domain/siteform"
	"vozko/domain/sitesubmission"
)

func (w *world) publishManifest(t *testing.T, m siteform.Manifest, settings site.FormSettings) string {
	t.Helper()
	err := w.svc.Activate(context.Background(), ActivateInput{
		WorkspaceID: "w1", ProjectID: "p1", DeploymentID: "d1", Manifests: []siteform.Manifest{m}, Settings: map[string]site.FormSettings{m.ID: settings},
	})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := w.svc.KeysFor(context.Background(), "w1", "p1", []string{m.ID})
	if err != nil {
		t.Fatal(err)
	}
	return keys[m.ID]
}

func (w *world) sweepAt(t *testing.T, at time.Time) {
	t.Helper()
	w.clock = at
	if err := w.svc.RunPendingEffects(context.Background()); err != nil {
		t.Logf("sweep: %v", err)
	}
}

func TestEffectsNeverBlockTheVisitorAndSurviveAClosedTab(t *testing.T) {
	w := newWorld(t)
	w.defer_ = true
	key := w.publish(t, site.FormSettings{FunnelID: "pipe1", Owner: &site.Owner{Kind: site.OwnerUser, ID: "user-3"}, Notify: true})
	ctx, cancel := context.WithCancel(context.Background())
	result, err := w.svc.Submit(ctx, key, visitor, w.body(t, key, goodFields()))
	if err != nil || result.SubmissionID != "sub-1" {
		t.Fatalf("result %+v err %v", result, err)
	}
	if len(w.owners.set) != 0 || len(w.deals.created) != 0 || len(w.notices.drafts) != 0 || len(w.deferred) != 1 {
		t.Fatal("the visitor is answered before any effect runs")
	}
	if run := w.effects.run("sub-1"); len(run.Pending) != 5 || run.FinishedAt != nil {
		t.Fatalf("the pending effects are stored with the submission: %+v", run)
	}
	cancel()
	w.deferred[0]()
	if len(w.owners.set) != 1 || len(w.deals.created) != 1 || len(w.notices.drafts) != 1 {
		t.Fatalf("a closed tab still gets its effects: owners %v deals %d notices %d", w.owners.set, len(w.deals.created), len(w.notices.drafts))
	}
	if run := w.effects.run("sub-1"); run.FinishedAt == nil || len(run.Pending) != 0 || run.Abandoned() {
		t.Fatalf("run %+v", run)
	}
}

func TestAFailedEffectIsRetriedWithBackoffAndTheOthersAreNotRepeated(t *testing.T) {
	w := newWorld(t)
	w.notices.fail = 1
	key := w.publish(t, site.FormSettings{Owner: &site.Owner{Kind: site.OwnerRoulette, ID: "dept-1"}, Notify: true})
	if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields())); err != nil {
		t.Fatal(err)
	}
	run := w.effects.run("sub-1")
	if len(w.owners.set) != 1 || len(w.notices.drafts) != 0 || run.Attempts != 1 || !run.Has(sitesubmission.EffectNotice) || run.Has(sitesubmission.EffectOwner) || run.OwnerID != "user-7" {
		t.Fatalf("after the first attempt owners %v notices %d run %+v", w.owners.set, len(w.notices.drafts), run)
	}
	w.sweepAt(t, w.clock)
	if len(w.notices.drafts) != 0 {
		t.Fatal("a retry waits for its backoff")
	}
	w.sweepAt(t, w.clock.Add(time.Minute))
	if len(w.notices.drafts) != 1 || len(w.owners.set) != 1 || len(w.roulette.asks) != 1 {
		t.Fatalf("notices %d owners %v roulette %v", len(w.notices.drafts), w.owners.set, w.roulette.asks)
	}
	if run := w.effects.run("sub-1"); run.FinishedAt == nil || run.Abandoned() {
		t.Fatalf("run %+v", run)
	}
}

func TestARoulettePickIsKeptAcrossRetries(t *testing.T) {
	w := newWorld(t)
	key := w.publish(t, site.FormSettings{Owner: &site.Owner{Kind: site.OwnerRoulette, ID: "dept-1"}})
	w.defer_ = true
	if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields())); err != nil {
		t.Fatal(err)
	}
	w.effects.mu.Lock()
	w.effects.runs["sub-1"].OwnerID = "user-9"
	w.effects.mu.Unlock()
	w.deferred[0]()
	if len(w.roulette.asks) != 0 || len(w.owners.set) != 1 || w.owners.set[0] != "system|lead-1|user-9" {
		t.Fatalf("a stored pick is reused: roulette %v owners %v", w.roulette.asks, w.owners.set)
	}
}

func TestTheWorkflowFiresExactlyOnce(t *testing.T) {
	w := newWorld(t)
	w.official.fail = 1
	w.leads.triggerErr = errors.New("queue down")
	followUp := site.FollowUp{Kind: site.FollowUpWhatsApp, ID: "bp-1"}
	key := w.publish(t, site.FormSettings{WorkflowID: "wf-1", FollowUp: &followUp})
	if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields())); err != nil {
		t.Fatal(err)
	}
	if len(w.leads.triggers) != 0 || len(w.subs.attached) != 0 {
		t.Fatal("no conversation, no trigger")
	}
	w.sweepAt(t, w.clock.Add(time.Minute))
	if len(w.official.got) != 2 || w.subs.attached["sub-1"] != "whatsapp:entry-official" || len(w.leads.triggers) != 0 {
		t.Fatalf("opens %d attached %v triggers %d", len(w.official.got), w.subs.attached, len(w.leads.triggers))
	}
	if run := w.effects.run("sub-1"); !run.Has(sitesubmission.EffectTrigger) || run.Has(sitesubmission.EffectConversation) {
		t.Fatalf("a failed trigger stays pending: %+v", run)
	}
	w.leads.triggerErr = nil
	w.sweepAt(t, w.clock.Add(2*time.Minute))
	w.sweepAt(t, w.clock.Add(time.Hour))
	if len(w.leads.triggers) != 1 || len(w.official.got) != 2 || w.leads.triggers[0].EntryID != "entry-official" {
		t.Fatalf("triggers %+v opens %d", w.leads.triggers, len(w.official.got))
	}
}

func TestATriggerThatCannotBeClaimedIsNotFired(t *testing.T) {
	w := newWorld(t)
	followUp := site.FollowUp{Kind: site.FollowUpWhatsApp, ID: "bp-1"}
	key := w.publish(t, site.FormSettings{WorkflowID: "wf-1", FollowUp: &followUp})
	w.defer_ = true
	if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields())); err != nil {
		t.Fatal(err)
	}
	w.effects.failSpend = errors.New("db down")
	w.deferred[0]()
	if len(w.leads.triggers) != 0 || w.subs.attached["sub-1"] == "" || !w.effects.run("sub-1").Has(sitesubmission.EffectTrigger) {
		t.Fatal("the trigger is marked spent before it fires")
	}
	w.effects.failSpend = nil
	w.sweepAt(t, w.clock.Add(time.Minute))
	if len(w.leads.triggers) != 1 {
		t.Fatalf("triggers %d", len(w.leads.triggers))
	}
}

func TestARepeatedSubmissionKeepsTheOriginalPendingEffects(t *testing.T) {
	w := newWorld(t)
	w.notices.fail = 1
	key := w.publish(t, site.FormSettings{Notify: true})
	first, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields()))
	if err != nil {
		t.Fatal(err)
	}
	again, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields()))
	if err != nil || again.SubmissionID != first.SubmissionID || len(w.effects.runs) != 1 {
		t.Fatalf("again %+v err %v runs %d", again, err, len(w.effects.runs))
	}
	w.sweepAt(t, w.clock.Add(time.Minute))
	if len(w.notices.drafts) != 1 {
		t.Fatalf("the pending notice is still delivered: %d", len(w.notices.drafts))
	}
}

func TestAnOwnedLeadKeepsItsOwnerAndSpendsNoRoulettePick(t *testing.T) {
	w := newWorld(t)
	w.leads.owner = "user-2"
	key := w.publish(t, site.FormSettings{FunnelID: "pipe1", Owner: &site.Owner{Kind: site.OwnerRoulette, ID: "dept-1"}})
	if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields())); err != nil {
		t.Fatal(err)
	}
	if len(w.roulette.asks) != 0 || len(w.owners.set) != 0 {
		t.Fatalf("roulette %v owners %v", w.roulette.asks, w.owners.set)
	}
	if len(w.deals.created) != 1 || w.deals.created[0].OwnerID != "user-2" {
		t.Fatalf("the deal follows the lead owner: %+v", w.deals.created)
	}
	created := newWorld(t)
	created.leads.owner, created.leads.created = "user-2", true
	fresh := created.publish(t, site.FormSettings{Owner: &site.Owner{Kind: site.OwnerRoulette, ID: "dept-1"}})
	if _, err := created.svc.Submit(context.Background(), fresh, visitor, created.body(t, fresh, goodFields())); err != nil {
		t.Fatal(err)
	}
	if len(created.roulette.asks) != 1 || len(created.owners.set) != 1 {
		t.Fatalf("a lead the form created is placed: roulette %v owners %v", created.roulette.asks, created.owners.set)
	}
}

func TestAnOpenDealInTheFunnelIsNotDuplicated(t *testing.T) {
	filler := func(n int, pipeline string, status opportunity.Status) []*opportunity.Opportunity {
		out := make([]*opportunity.Opportunity, 0, n)
		for i := range n {
			out = append(out, &opportunity.Opportunity{ID: "o" + strconv.Itoa(i) + pipeline + string(status), PipelineID: pipeline, Status: status, CreatedAt: testNow.Add(-time.Duration(i) * time.Minute)})
		}
		return out
	}
	cases := map[string]struct {
		deals []*opportunity.Opportunity
		want  int
	}{
		"no deal":                     {nil, 1},
		"open deal in the funnel":     {filler(1, "pipe1", opportunity.StatusOpen), 0},
		"won deal in the funnel":      {filler(1, "pipe1", opportunity.StatusWon), 1},
		"open deal in another funnel": {filler(1, "pipe2", opportunity.StatusOpen), 1},
		"open deal on a later page":   {append(filler(dealsPageSize, "pipe2", opportunity.StatusOpen), filler(1, "pipe1", opportunity.StatusOpen)...), 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			w.leadDeals.deals = tc.deals
			key := w.publish(t, site.FormSettings{FunnelID: "pipe1", Owner: &site.Owner{Kind: site.OwnerUser, ID: "user-3"}})
			if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields())); err != nil {
				t.Fatal(err)
			}
			if len(w.deals.created) != tc.want {
				t.Fatalf("deals %d, want %d", len(w.deals.created), tc.want)
			}
			if q := w.leadDeals.asked[0]; q.WorkspaceID != "w1" || q.LeadID != "lead-1" {
				t.Fatalf("query %+v", q)
			}
		})
	}
	w := newWorld(t)
	w.leadDeals.err = errors.New("db down")
	key := w.publish(t, site.FormSettings{FunnelID: "pipe1", Owner: &site.Owner{Kind: site.OwnerUser, ID: "user-3"}})
	if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields())); err != nil {
		t.Fatal(err)
	}
	if len(w.deals.created) != 0 || !w.effects.run("sub-1").Has(sitesubmission.EffectDeal) {
		t.Fatal("an unreadable deal list never creates a deal and retries")
	}
}

func TestTheMetaLeadNeedsTheVisitorsConsent(t *testing.T) {
	w := newWorld(t)
	m := siteform.Manifest{ID: "semconsent", Fields: []siteform.Field{{Name: "lead.name", Kind: siteform.KindText}, {Name: "lead.phone", Kind: siteform.KindTel}}}
	key := w.publishManifest(t, m, site.FormSettings{})
	fields := map[string]any{"lead.name": "Ana", "lead.phone": "+55 84 99999-0000"}
	if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, fields)); err != nil {
		t.Fatal(err)
	}
	if len(w.conversions.sent) != 0 {
		t.Fatalf("no consent, no Meta lead: %+v", w.conversions.sent)
	}
}

func TestAnInvalidProofNeverSpendsTheFormQuota(t *testing.T) {
	w := newWorld(t)
	key := w.publish(t, site.FormSettings{})
	raw := w.body(t, key, goodFields(), func(b map[string]any) { b["nonce"] = "not-a-solution" })
	if _, err := w.svc.Submit(context.Background(), key, visitor, raw); !errors.Is(err, pfdomain.ErrChallengeUnsolved) {
		t.Fatalf("err = %v", err)
	}
	if len(w.formLimit.keys) != 0 {
		t.Fatalf("the form quota was spent by a challenge or a bad proof: %v", w.formLimit.keys)
	}
	if _, err := w.svc.Submit(context.Background(), key, visitor, w.body(t, key, goodFields())); err != nil {
		t.Fatal(err)
	}
	if len(w.formLimit.keys) != 1 || w.formLimit.keys[0] != key {
		t.Fatalf("a valid proof counts once against the form: %v", w.formLimit.keys)
	}
}

func TestThePreviewSinkHasItsOwnPerIPBucket(t *testing.T) {
	w := newWorld(t)
	preview := Visitor{Origin: "https://preview--abc.paginas.vozko.dev", IP: "203.0.113.7", UserAgent: "x"}
	challenge, err := w.svc.Challenge(context.Background(), sitesubmission.PreviewFormKey, preview)
	if err != nil {
		t.Fatal(err)
	}
	hash := sitesubmission.HashIP(w.svc.cfg.IPHashKey, preview.IP)
	if len(w.previewLim.keys) != 1 || w.previewLim.keys[0] != "preview:challenge:"+hash || len(w.ipLimit.keys) != 0 || len(w.formLimit.keys) != 0 {
		t.Fatalf("preview %v ip %v form %v", w.previewLim.keys, w.ipLimit.keys, w.formLimit.keys)
	}
	w.clock = w.clock.Add(4 * time.Second)
	raw, _ := json.Marshal(map[string]any{"utm": map[string]string{}, "idempotencyKey": "k-12345678", "route": "/", "fields": goodFields(), "challengeToken": challenge.Token, "nonce": solve(t, challenge)})
	w.previewLim.deny = map[string]bool{"preview:submit:" + hash: true}
	if _, err := w.svc.Submit(context.Background(), sitesubmission.PreviewFormKey, preview, raw); !errors.Is(err, pfdomain.ErrRateLimited) {
		t.Fatalf("a limited preview = %v", err)
	}
	w.previewLim.err = errors.New("redis down")
	if _, err := w.svc.Challenge(context.Background(), sitesubmission.PreviewFormKey, preview); !errors.Is(err, pfdomain.ErrRateLimited) {
		t.Fatalf("a preview limiter failure refuses: %v", err)
	}
}

func TestTheSweepReportsAStoreFailure(t *testing.T) {
	w := newWorld(t)
	w.effects.runs["sub-9"] = &sitesubmission.EffectRun{SubmissionID: "sub-9", Pending: []sitesubmission.Effect{sitesubmission.EffectNotice}, NextAttemptAt: testNow}
	w.effects.failClaim = errors.New("db down")
	if err := w.svc.RunPendingEffects(context.Background()); err == nil {
		t.Fatal("a claim failure is reported")
	}
}
