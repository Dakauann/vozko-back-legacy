package siteforms

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"vozko/domain/conversation"
	"vozko/domain/site"
	"vozko/domain/siteform"
	"vozko/domain/sitepublish"
	"vozko/domain/sitesubmission"
)

type world struct {
	forms       *formShelf
	subs        *submissionShelf
	effects     *effectShelf
	hosts       hostnames
	ipLimit     *limiter
	formLimit   *limiter
	previewLim  *limiter
	leads       *leadDesk
	optIns      *optIns
	roulette    *roulette
	owners      *owners
	deals       *deals
	leadDeals   *leadDeals
	official    *officialEntries
	unofficial  *unofficialStarts
	email       *emailOpens
	notices     *notices
	conversions *conversions
	links       *links
	refs        *references
	svc         *Service
	ids         int
	clock       time.Time
	deferred    []func()
	defer_      bool
}

func newWorld(t *testing.T) *world {
	t.Helper()
	effects := newEffectShelf()
	w := &world{
		forms: newFormShelf(), subs: &submissionShelf{effects: effects}, effects: effects, hosts: hostnames{"p1": {"loja.vozko.site", "www.loja.com.br"}},
		ipLimit: &limiter{}, formLimit: &limiter{}, previewLim: &limiter{}, leads: &leadDesk{}, optIns: &optIns{}, roulette: &roulette{pick: "user-7"},
		owners: &owners{}, deals: &deals{}, leadDeals: &leadDeals{}, official: &officialEntries{}, unofficial: &unofficialStarts{}, email: &emailOpens{},
		notices: &notices{}, conversions: &conversions{}, links: &links{}, refs: &references{}, clock: testNow,
	}
	svc, err := New(Deps{
		Forms: w.forms, Submissions: w.subs, Effects: w.effects, Hostnames: w.hosts, Projects: projects{"p1": "w1"}, OneShot: &spent{},
		PerIP: w.ipLimit, PerForm: w.formLimit, PerPreview: w.previewLim, Leads: w.leads, EmailOptIns: w.optIns, Roulette: w.roulette, LeadOwners: w.owners,
		Deals: w.deals, LeadDeals: w.leadDeals, Official: w.official, Unofficial: w.unofficial, Email: w.email, Notices: w.notices, Conversions: w.conversions,
		Links: w.links, Numbers: numbers{"n1": "5584999990000"}, References: w.refs,
		Config: Config{ChallengeKey: []byte("challenge-key-for-tests-0123456789"), IPHashKey: sitesubmission.IPHashKey("ip-secret-for-tests-0123456789abc"), Addresses: sitepublish.Addresses{Domain: "paginas.vozko.dev"}, ShortLinkBase: "https://vz.co"},
		Now:    func() time.Time { return w.clock },
		NewID:  func() string { w.ids++; return "sub-" + strconv.Itoa(w.ids) },
		Async: func(fn func()) {
			if w.defer_ {
				w.deferred = append(w.deferred, fn)
				return
			}
			fn()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	w.svc = svc
	return w
}

func contactForm() siteform.Manifest {
	return siteform.Manifest{ID: "contato", ConsentText: "Aceito ser contatado", EmailOptInText: "Quero receber emails", Fields: []siteform.Field{
		{Name: "lead.name", Kind: siteform.KindText, Required: true},
		{Name: "lead.phone", Kind: siteform.KindTel},
		{Name: "lead.email", Kind: siteform.KindEmail},
		{Name: "field.plano", Kind: siteform.KindSelect, Options: []string{"basico", "pro"}},
		{Name: "consent", Kind: siteform.KindCheckbox, Required: true},
		{Name: "email_opt_in", Kind: siteform.KindCheckbox},
	}}
}

func (w *world) publish(t *testing.T, settings site.FormSettings) string {
	t.Helper()
	err := w.svc.Activate(context.Background(), ActivateInput{
		WorkspaceID: "w1", ProjectID: "p1", DeploymentID: "d1", Manifests: []siteform.Manifest{contactForm()},
		Settings: map[string]site.FormSettings{"contato": settings}, Widgets: []string{"widget-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := w.svc.KeysFor(context.Background(), "w1", "p1", []string{"contato"})
	if err != nil {
		t.Fatal(err)
	}
	return keys["contato"]
}

func TestKeysAreStablePerProjectForm(t *testing.T) {
	w := newWorld(t)
	first, err := w.svc.KeysFor(context.Background(), "w1", "p1", []string{"contato", "newsletter"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := w.svc.KeysFor(context.Background(), "w1", "p1", []string{"contato"})
	if err != nil {
		t.Fatal(err)
	}
	if first["contato"] == "" || first["newsletter"] == "" || first["contato"] == first["newsletter"] || again["contato"] != first["contato"] {
		t.Fatalf("first %v again %v", first, again)
	}
	if !sitesubmission.ValidFormKey(first["contato"]) {
		t.Fatalf("key %q is not url safe", first["contato"])
	}
	if _, err := w.svc.KeysFor(context.Background(), "w1", "p1", []string{"Not A Form"}); err == nil {
		t.Fatal("an invalid form id must refuse")
	}
	w.forms.failKeys = errors.New("db down")
	if _, err := w.svc.KeysFor(context.Background(), "w1", "p1", []string{"contato"}); err == nil {
		t.Fatal("a store failure must refuse")
	}
}

func TestActivatingADeploymentReplacesTheProjectsForms(t *testing.T) {
	w := newWorld(t)
	key := w.publish(t, site.FormSettings{Notify: true})
	form := w.forms.active[key]
	if form.FormID != "contato" || form.DeploymentID != "d1" || form.WorkspaceID != "w1" || !form.Settings.Notify || form.Manifest.ConsentText == "" {
		t.Fatalf("active %+v", form)
	}
	if len(w.forms.embeds["p1"]) != 1 {
		t.Fatalf("embeds %v", w.forms.embeds)
	}
	if err := w.svc.Deactivate(context.Background(), "p1"); err != nil || len(w.forms.active) != 0 {
		t.Fatalf("deactivate err=%v active=%v", err, w.forms.active)
	}
}

func TestActivationRefusesAWorkflowFormWithoutAReachableChannel(t *testing.T) {
	w := newWorld(t)
	err := w.svc.Activate(context.Background(), ActivateInput{
		WorkspaceID: "w1", ProjectID: "p1", DeploymentID: "d1",
		Manifests: []siteform.Manifest{{ID: "news", Fields: []siteform.Field{{Name: "lead.email", Kind: siteform.KindEmail}}}},
		Settings:  map[string]site.FormSettings{"news": {WorkflowID: "wf", FollowUp: &site.FollowUp{Kind: site.FollowUpWhatsApp, ID: "n1"}}},
	})
	if !errors.Is(err, site.ErrFormChannelRequired) || len(w.forms.activations) != 0 {
		t.Fatalf("err = %v activations %d", err, len(w.forms.activations))
	}
	stray := w.svc.Activate(context.Background(), ActivateInput{WorkspaceID: "w1", ProjectID: "p1", DeploymentID: "d1",
		Manifests: []siteform.Manifest{contactForm()}, Settings: map[string]site.FormSettings{"other": {}}})
	if stray != nil {
		t.Fatalf("settings of a form no longer on the page are ignored: %v", stray)
	}
}

func TestAWhatsAppSuccessGetsItsTrackedLinkAtActivation(t *testing.T) {
	w := newWorld(t)
	key := w.publish(t, site.FormSettings{Success: &site.Success{Kind: site.SuccessWhatsApp, Channel: "n1"}})
	if got := w.forms.active[key].SuccessURL; got != "https://vz.co/c1" {
		t.Fatalf("success url = %q", got)
	}
}

func TestAWhatsAppLinkGoesThroughAShortLinkWithAVisibleReference(t *testing.T) {
	w := newWorld(t)
	link, err := w.svc.WhatsAppLink(context.Background(), "p1", "n1", "Quero saber mais", "/oferta")
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "https://vz.co/c1" || len(link.Reference) != conversation.PageReferenceLength {
		t.Fatalf("link %+v", link)
	}
	target, _ := url.Parse(w.links.created[0].TargetURL)
	if target.Host != "wa.me" || target.Path != "/5584999990000" || target.Query().Get("text") != "Quero saber mais (ref "+link.Reference+")" || w.links.created[0].WorkspaceID != "w1" {
		t.Fatalf("target %s", w.links.created[0].TargetURL)
	}
	again, err := w.svc.WhatsAppLink(context.Background(), "p1", "n1", "Quero saber mais", "/oferta")
	if err != nil || again != link || len(w.links.created) != 1 {
		t.Fatalf("again %+v err %v links %d", again, err, len(w.links.created))
	}
	found, _ := w.refs.Target(context.Background(), link.Reference)
	if found == nil || found.ProjectID != "p1" || found.Route != "/oferta" || found.WorkspaceID != "w1" {
		t.Fatalf("reference target %+v", found)
	}
	if _, err := w.svc.WhatsAppLink(context.Background(), "p1", "unknown", "", "/"); err == nil {
		t.Fatal("an unknown number must refuse")
	}
	if _, err := w.svc.WhatsAppLink(context.Background(), "p9", "n1", "", "/"); err == nil {
		t.Fatal("an unknown project must refuse")
	}
	if _, err := w.svc.WhatsAppLink(context.Background(), "p1", "n1", strings.Repeat("a", 600), "/"); err == nil {
		t.Fatal("a huge text must refuse")
	}
}

func TestADirectWhatsAppLinkOpensTheNumberWithoutAReference(t *testing.T) {
	w := newWorld(t)
	link, err := w.svc.DirectLink(context.Background(), "w1", "n1", "Quero saber mais")
	if err != nil {
		t.Fatal(err)
	}
	if link != "https://wa.me/5584999990000?text=Quero%20saber%20mais" || len(w.links.created) != 0 || len(w.refs.rows) != 0 {
		t.Fatalf("link %q links %d refs %d", link, len(w.links.created), len(w.refs.rows))
	}
	if bare, err := w.svc.DirectLink(context.Background(), "w1", "n1", "  "); err != nil || bare != "https://wa.me/5584999990000" {
		t.Fatalf("bare %q %v", bare, err)
	}
	if _, err := w.svc.DirectLink(context.Background(), "w1", "unknown", ""); !errors.Is(err, ErrWhatsAppNumber) {
		t.Fatalf("an unknown number must refuse: %v", err)
	}
	if _, err := w.svc.DirectLink(context.Background(), "w1", "n1", strings.Repeat("a", 600)); !errors.Is(err, ErrWhatsAppText) {
		t.Fatalf("a huge text must refuse: %v", err)
	}
}

func TestAWidgetAcceptsThePublishedAddressesOfPagesThatEmbedIt(t *testing.T) {
	w := newWorld(t)
	w.publish(t, site.FormSettings{})
	got, err := w.svc.WidgetOrigins(context.Background(), "w1", "widget-1")
	if err != nil || strings.Join(got, ",") != "https://loja.vozko.site,https://www.loja.com.br" {
		t.Fatalf("origins %v err %v", got, err)
	}
	none, err := w.svc.WidgetOrigins(context.Background(), "w1", "widget-2")
	if err != nil || len(none) != 0 {
		t.Fatalf("origins %v err %v", none, err)
	}
	_ = w.svc.Deactivate(context.Background(), "p1")
	gone, _ := w.svc.WidgetOrigins(context.Background(), "w1", "widget-1")
	if len(gone) != 0 {
		t.Fatalf("an unpublished page still accepted: %v", gone)
	}
}

func TestALiveFormIsReadableByItsWorkspaceForWorkflowTriggers(t *testing.T) {
	w := newWorld(t)
	key := w.publish(t, site.FormSettings{})
	form, err := w.svc.LiveForm(context.Background(), "w1", key)
	if err != nil || form.FormID != "contato" || len(form.Manifest.Fields) == 0 {
		t.Fatalf("form %+v err %v", form, err)
	}
	if _, err := w.svc.LiveForm(context.Background(), "w2", key); !errors.Is(err, sitesubmission.ErrFormUnknown) {
		t.Fatalf("another workspace never reads it: %v", err)
	}
	if _, err := w.svc.LiveForm(context.Background(), "w1", "unknown-key"); !errors.Is(err, sitesubmission.ErrFormUnknown) {
		t.Fatalf("an unpublished key is unknown: %v", err)
	}
}

func TestAStoreFailureIsNeverReportedAsAMissingPageOrNumber(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	cases := map[string]struct {
		run     func() error
		want    error
		mustNot error
	}{
		"missing page":             {run: func() error { _, err := w.svc.WhatsAppLink(ctx, "p9", "n1", "", "/"); return err }, want: ErrProjectNotFound},
		"page store down":          {run: func() error { _, err := w.svc.WhatsAppLink(ctx, "broken", "n1", "", "/"); return err }, want: errStoreDown, mustNot: ErrProjectNotFound},
		"missing number":           {run: func() error { _, err := w.svc.WhatsAppLink(ctx, "p1", "unknown", "", "/"); return err }, want: ErrWhatsAppNumber},
		"number store down":        {run: func() error { _, err := w.svc.WhatsAppLink(ctx, "p1", "broken", "", "/"); return err }, want: errStoreDown, mustNot: ErrWhatsAppNumber},
		"direct number store down": {run: func() error { _, err := w.svc.DirectLink(ctx, "w1", "broken", ""); return err }, want: errStoreDown, mustNot: ErrWhatsAppNumber},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.run()
			if !errors.Is(err, tc.want) || (tc.mustNot != nil && errors.Is(err, tc.mustNot)) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
