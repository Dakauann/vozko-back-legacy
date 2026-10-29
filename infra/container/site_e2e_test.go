package container

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/lead"
	pfdomain "vozko/domain/publicform"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/domain/sitesubmission"
	"vozko/infra/sitehosting"
	"vozko/usecases/formlead"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/sitebuild"
	"vozko/usecases/siteforms"
	sitepublish_usecase "vozko/usecases/sitepublish"
	siteserve_usecase "vozko/usecases/siteserve"
	"vozko/usecases/sitethumbnail"
)

const (
	e2eWorkspace   = "2b8f6a1e-4c3d-4e5f-9a0b-1c2d3e4f5a6b"
	e2eUser        = "7d1e2f3a-4b5c-4d6e-8f90-a1b2c3d4e5f6"
	e2ePublic      = "paginas.vozko.test"
	e2eAPI         = "https://api.vozko.test"
	e2eSigningKey  = "0123456789abcdef0123456789abcdef-e2e"
	e2eFollowPhone = "phone-1"
)

type siteWorld struct {
	clock       time.Time
	clockMu     sync.Mutex
	store       *e2eSiteStore
	builder     *replayBuilder
	renderer    *replayRenderer
	build       *sitebuild.Service
	storage     *site_usecase.Service
	forms       *siteforms.Service
	formStore   *e2eFormStore
	submissions *e2eSubmissions
	leads       *e2eLeads
	official    *e2eOfficial
	links       *e2eLinks
	publisher   *sitepublish_usecase.Service
	objects     *e2eObjects
	visits      *e2eVisits
	pages       *siteserve_usecase.Service
	hostnames   *e2eHostnames
	deployments *e2eDeployments
	library     *e2eLibrary
	sections    *sitebuild.WorkspaceSections
	thumbnails  *sitethumbnail.Service
}

func (w *siteWorld) now() time.Time {
	w.clockMu.Lock()
	defer w.clockMu.Unlock()
	return w.clock
}

func (w *siteWorld) advance(d time.Duration) {
	w.clockMu.Lock()
	defer w.clockMu.Unlock()
	w.clock = w.clock.Add(d)
}

func newSiteWorld(t *testing.T) *siteWorld {
	t.Helper()
	rec := loadRecordedBuild(t)
	w := &siteWorld{
		clock:       time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC),
		store:       newE2ESiteStore(),
		builder:     &replayBuilder{rec: rec},
		renderer:    &replayRenderer{rec: rec},
		formStore:   &e2eFormStore{keys: map[string]map[string]string{}, active: map[string]siteforms.ActiveForm{}},
		submissions: &e2eSubmissions{effects: &e2eEffects{runs: map[string]*sitesubmission.EffectRun{}}},
		leads:       &e2eLeads{byPhone: map[string]*lead.Lead{}},
		official:    &e2eOfficial{},
		links:       &e2eLinks{},
		objects:     &e2eObjects{objects: map[string][]byte{}, keys: map[string][]byte{}},
		visits:      &e2eVisits{},
		hostnames:   &e2eHostnames{rows: map[string]sitepublish.HostnameRecord{}},
		deployments: &e2eDeployments{rows: map[string]sitepublish.DeploymentRecord{}},
		library:     &e2eLibrary{},
	}
	c := &Container{}
	leads, err := formlead.New(formlead.Deps{Phones: w.leads, Emails: w.leads, Profiles: w.leads, Consents: w.leads, Triggers: w.leads})
	if err != nil {
		t.Fatal(err)
	}
	w.forms, err = siteforms.New(siteforms.Deps{
		Forms: w.formStore, Submissions: w.submissions, Effects: w.submissions.effects, Hostnames: w.hostnames, Projects: w.store,
		OneShot: &e2eSpent{seen: map[string]bool{}}, PerIP: e2eOpenLimiter{}, PerForm: e2eOpenLimiter{}, PerPreview: e2eOpenLimiter{},
		Leads: leads, EmailOptIns: e2eOptIns{}, Roulette: e2eRoulette{}, LeadOwners: e2eOwners{}, Deals: e2eDeals{}, LeadDeals: e2eNoDeals{},
		Official: w.official, Unofficial: e2eUnofficial{}, Email: e2eEmail{}, Notices: &e2eNotices{}, Conversions: e2eConversions{},
		Links: w.links, Numbers: e2eNumbers{e2eFollowPhone: "5511999990000"}, References: &e2eReferences{}, Automations: e2eAutomations{},
		Calendar: e2eCalendar{}, Slots: e2eMemo{}, BookingLock: e2eBookingLock{}, LeadFields: e2eLeadFields{},
		Config: siteforms.Config{
			ChallengeKey: []byte(e2eSigningKey), IPHashKey: sitesubmission.IPHashKey(e2eSigningKey),
			Addresses: sitepublish.Addresses{Domain: e2ePublic}, ShortLinkBase: "https://vzk.to",
		},
		Now: w.now, Async: func(fn func()) { fn() },
	})
	if err != nil {
		t.Fatal(err)
	}
	c.siteFormsWiring = w.forms
	library, err := sitebuild.Sections()
	if err != nil {
		t.Fatal(err)
	}
	w.sections = sitebuild.NewWorkspaceSections(library, siteSnippetSource{store: w.store})
	thumbnails, err := sitethumbnail.New(sitethumbnail.Deps{
		Renderer: w.renderer, Uploader: w.library, Projects: w.store, Media: w.library, Files: w.library, Throttle: &e2eThrottle{}, Spawn: func(fn func()) { fn() },
	})
	if err != nil {
		t.Fatal(err)
	}
	w.thumbnails = thumbnails
	names := e2eWorkspaces{e2eWorkspace: "Clínica Sorriso"}
	w.build, err = sitebuild.NewService(sitebuild.Config{
		Addresses: sitepublish.Addresses{Domain: e2ePublic}, SigningKey: e2eSigningKey, APIOrigin: e2eAPI, Runtime: siteRuntimeFiles(),
	}, sitebuild.Deps{
		Builder: w.builder, Cache: &e2eCache{items: map[string]any{}}, Media: e2eNoMedia{}, Sections: w.sections,
		WhatsApp: c.siteFormsForPublish(), Widgets: e2eNoWidgets{}, Workspaces: names, Now: w.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	editor := siteBuildEditor{service: w.build}
	w.storage, err = site_usecase.NewService(site_usecase.Deps{
		Store: w.store, Executor: editor, Previewer: editor, References: e2eAnyReference{}, Bindings: c.siteFormsForPublish(), Forms: c.siteFormsForPublish(), Retirement: e2eRetirement{world: w}, Published: sitePublishedRevisions{deployments: w.deployments},
		Media: w.library, Numbers: e2eNumbers{e2eFollowPhone: "5511999990000"}, Hostnames: w.hostnames, Previews: &e2ePreviewCache{},
	})
	if err != nil {
		t.Fatal(err)
	}
	directory := sitehosting.NewDirectory(w.objects, w.now)
	marker, err := site_usecase.NewPublishMarker(w.store, w.now, uuid.NewString)
	if err != nil {
		t.Fatal(err)
	}
	w.publisher, err = sitepublish_usecase.New(sitepublish_usecase.Deps{
		Projects: siteProjectsAdapter{storage: w.storage, marker: marker}, Workspaces: names, Builder: siteCandidateBuilder{service: w.build},
		Renderer: w.renderer, Forms: c.siteFormsForPublish(), Hosting: sitehosting.New(w.objects, directory), Media: e2eNoMedia{}, CustomHostnames: e2eNoCustomHostnames{},
		Access: e2eAllow{}, Subscriptions: e2eActive{}, Entitlements: e2eEntitlements{}, Widgets: e2eNoWidgets{},
		Deployments: w.deployments, Hostnames: w.hostnames, Abuse: &e2eAbuse{}, Alerts: &e2eNotices{}, Thumbnails: thumbnails, Locks: &e2eLocks{held: map[string]bool{}}, Counters: e2eCounters{}, NewID: uuid.NewString,
		DNS: e2eNoDNS{}, Reversals: e2eNoReversals{}, Calendars: e2eCalendar{},
	}, sitepublish_usecase.Config{
		Addresses: sitepublish.Addresses{Domain: e2ePublic}, APIOrigin: e2eAPI, CNAMETarget: "sites." + e2ePublic,
		ApplicationDomains: []string{"vozko.test"}, BrandWords: []string{"vozko"},
	})
	if err != nil {
		t.Fatal(err)
	}
	w.pages, err = siteserve_usecase.New(siteserve_usecase.Deps{
		Directory: directory, Files: sitehosting.NewFiles(w.objects), Visits: w.visits, Beacons: e2eOpenLimiter{}, Passwords: e2eOpenLimiter{},
		Now: w.now, Roll: func(int) int { return 0 },
	}, siteserve_usecase.Config{
		Addresses: sitepublish.Addresses{Domain: e2ePublic}, PlatformDomains: sitepublish.PlatformDomains(e2ePublic, []string{"vozko.test"}),
		CookieSecret: []byte(e2eSigningKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func solveChallenge(t *testing.T, c pfdomain.Challenge) string {
	t.Helper()
	for n := 0; n < 1<<24; n++ {
		if pfdomain.SolvesChallenge(c.Token, strconv.Itoa(n), c.Bits) {
			return strconv.Itoa(n)
		}
	}
	t.Fatal("the challenge has no solution")
	return ""
}

func (w *siteWorld) visit(t *testing.T, host, path string) siteserve_usecase.Content {
	t.Helper()
	out := w.pages.Serve(context.Background(), siteserve_usecase.Request{
		Host: host, Method: "GET", Path: path, Cookie: func(string) string { return "" }, UserAgent: "Mozilla/5.0 (iPhone) Safari/604.1", ClientIP: "203.0.113.9",
	})
	if out.Kind != siteserve_usecase.KindContent || out.Content.Missing {
		t.Fatalf("%s%s is not served: %+v", host, path, out)
	}
	return out.Content
}

func (w *siteWorld) servedFile(t *testing.T, dep sitepublish.DeploymentRecord, path string) string {
	t.Helper()
	return string(w.visit(t, "deploy--"+dep.ID+"."+e2ePublic, path).Body)
}

func (w *siteWorld) manifest(t *testing.T, dep sitepublish.DeploymentRecord) sitepublish.DeploymentManifest {
	t.Helper()
	manifest, err := sitepublish.DecodeDeploymentManifest(w.objects.objects[sitepublish.ManifestKey(dep.ID)])
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestALeadCapturePageGoesFromStarterToALiveFormThatCreatesExactlyOneLead(t *testing.T) {
	w := newSiteWorld(t)
	ctx := context.Background()
	actor := sitepublish_usecase.Actor{UserID: e2eUser, WorkspaceID: e2eWorkspace}

	created, err := w.storage.Create(ctx, site_usecase.CreateInput{WorkspaceID: e2eWorkspace, UserID: e2eUser, Name: "Clínica Sorriso", Stack: site.StackReact, Starter: "captura-de-leads"})
	if err != nil {
		t.Fatal(err)
	}
	projectID := created.Project.ID
	settings := `{"forms":{"contato":{"workflow_id":"wf_boas_vindas","follow_up":{"kind":"whatsapp","id":"` + e2eFollowPhone + `"},"success":{"kind":"redirect","route":"/obrigado"}}}}`
	if _, err := w.storage.PutSettings(ctx, e2eWorkspace, projectID, created.Settings.Version, []byte(settings)); err != nil {
		t.Fatal(err)
	}
	edited, err := w.storage.Edit(ctx, site_usecase.EditInput{
		WorkspaceID: e2eWorkspace, ProjectID: projectID, ExpectedVersion: created.Project.Version,
		Author: site_usecase.EloAuthor("thread_e2e"), Label: "Número do WhatsApp", Batch: w.builder.rec.Edits,
	})
	if err != nil {
		t.Fatal(err)
	}
	if edited.View.Revision.Kind != site_usecase.RevisionElo || !strings.Contains(edited.View.Files["src/routes/index.jsx"].Content, `data-vz-whatsapp="phone-1"`) {
		t.Fatalf("the Elo batch lands as an Elo revision: %+v", edited.View.Revision)
	}

	preview, err := w.storage.Preview(ctx, e2eWorkspace, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if shell := w.builder.last().Shell; shell.Forms["contato"] != sitesubmission.PreviewFormKey || shell.API != e2eAPI || w.builder.last().Mode != sitebuild.ModePreview {
		t.Fatalf("the preview posts its forms to the sink: %+v", shell)
	}
	host := strings.TrimSuffix(strings.TrimPrefix(preview.URL, "https://"), "/")
	if served, err := w.build.Serve(ctx, host, "/"); err != nil || !strings.Contains(string(served.Body), `vz-preview`) {
		t.Fatalf("the signed preview serves the page: %v", err)
	}

	if upload, err := w.thumbnails.Upload(ctx, e2eWorkspace, projectID, e2eJPEG(t, 1440, 900)); err != nil || !upload.Accepted {
		t.Fatalf("the editor uploads the draft thumbnail: %+v %v", upload, err)
	}
	if viewed, err := w.storage.Get(ctx, e2eWorkspace, projectID); err != nil || viewed.Project.ThumbnailURL == "" || len(w.renderer.shots) != 0 {
		t.Fatalf("the draft thumbnail comes from the browser, never the renderer: %+v %v %v", viewed, err, w.renderer.shots)
	}
	if upload, err := w.thumbnails.Upload(ctx, e2eWorkspace, projectID, e2eJPEG(t, 1440, 900)); err != nil || upload.Accepted || upload.RetryAfter <= 0 {
		t.Fatalf("a second upload inside ten minutes is declined: %+v %v", upload, err)
	}

	hostname, err := w.publisher.AddHostname(ctx, actor, projectID, sitepublish_usecase.AddHostnameInput{Kind: sitepublish.HostnamePlatform, Slug: "clinica-sorriso"})
	if err != nil {
		t.Fatal(err)
	}
	if hostname.Hostname != "clinica-sorriso."+e2ePublic {
		t.Fatalf("hostname %+v", hostname)
	}

	first, err := w.publisher.Publish(ctx, actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Held || len(first.Hostnames) != 1 || first.Hostnames[0] != hostname.Hostname || w.renderer.calls != 1 {
		t.Fatalf("publish %+v renderer calls %d", first, w.renderer.calls)
	}
	serving, err := sitepublish.DecodeServing(w.objects.keys[sitepublish.HostKey(hostname.Hostname)])
	if err != nil || serving.DeploymentID != first.Deployment.ID || serving.Suspended {
		t.Fatalf("host key %+v %v", serving, err)
	}
	if _, ok := w.objects.keys[sitepublish.DeploymentKey(first.Deployment.ID)]; !ok {
		t.Fatal("the immutable deployment key is written")
	}
	for sha, body := range w.objects.objects {
		if strings.Contains(string(body), sitepublish.OriginPlaceholder) {
			t.Fatalf("blob %s keeps the origin placeholder", sha)
		}
	}
	manifest := w.manifest(t, first.Deployment)
	if _, ok := manifest.Files["/__vz/prerender.html"]; ok {
		t.Fatal("the prerender page is never served")
	}
	social := w.builder.last().Shell.Pages["/"].OGImage
	socialPath := strings.TrimPrefix(social, "https://"+hostname.Hostname)
	if len(w.renderer.socials) != 3 || !strings.HasPrefix(social, "https://"+hostname.Hostname+sitepublish.MediaPathPrefix) || !strings.HasSuffix(social, ".jpg") {
		t.Fatalf("each route gets a rendered social image: %q %v", social, w.renderer.socials)
	}
	if image := w.servedFile(t, first.Deployment, socialPath); !strings.HasPrefix(image, "og:https://") {
		t.Fatalf("the social image is served by the deployment: %q", image)
	}
	if len(w.renderer.shots) != 1 || len(w.library.deleted) != 1 || w.library.deleted[0] != "media-1" {
		t.Fatalf("the publish replaces the preview thumbnail: %v %v", w.renderer.shots, w.library.deleted)
	}
	if sitemap := w.servedFile(t, first.Deployment, "/sitemap.xml"); !strings.Contains(sitemap, "https://"+hostname.Hostname+"/obrigado") {
		t.Fatalf("sitemap %s", sitemap)
	}
	formKey := w.formStore.keys[projectID]["contato"]
	live := w.visit(t, hostname.Hostname, "/")
	home := string(live.Body)
	if live.ETag != manifest.Files[manifest.Routes["/"]].ETag() || live.NoIndex || !strings.Contains(live.Policy, e2eAPI) {
		t.Fatalf("the hostname serves the published home: %+v", live)
	}
	beacon := w.pages.Serve(ctx, siteserve_usecase.Request{
		Host: hostname.Hostname, Method: "POST", Path: siteserve_usecase.BeaconPath, Cookie: func(string) string { return "" },
		UserAgent: "Mozilla/5.0 (iPhone) Safari/604.1", ClientIP: "203.0.113.9", Body: []byte(`{"event":"view","route":"/","variant":"control","utm":{"source":"instagram"}}`),
	})
	if beacon.Kind != siteserve_usecase.KindAccepted || len(w.visits.recorded) != 1 || w.visits.recorded[0].ProjectID != projectID || w.visits.recorded[0].UTMSource != "instagram" {
		t.Fatalf("the runtime beacon counts the view for the page: %+v %+v", beacon, w.visits.recorded)
	}
	if formKey == "" || !strings.Contains(home, formKey) || !strings.Contains(home, `data-vz-form="contato"`) || !strings.Contains(home, `href="https://vzk.to/c1"`) {
		t.Fatalf("the live page carries its form key and the tracked WhatsApp link:\n%s", home)
	}
	if len(w.links.created) == 0 || !strings.HasPrefix(w.links.created[0].TargetURL, "https://wa.me/5511999990000?text=") {
		t.Fatalf("short links %+v", w.links.created)
	}
	active, err := w.formStore.ActiveForm(ctx, formKey)
	if err != nil || active == nil || active.DeploymentID != first.Deployment.ID || active.Settings.WorkflowID != "wf_boas_vindas" {
		t.Fatalf("forms activation %+v %v", active, err)
	}
	stored, err := w.storage.Get(ctx, e2eWorkspace, projectID)
	if err != nil || stored.Project.Status != site_usecase.StatusPublished || stored.Project.PublishedDeploymentID != first.Deployment.ID || first.ProjectVersion != stored.Project.Version {
		t.Fatalf("project after publish %+v version %d", stored.Project, first.ProjectVersion)
	}

	visitor := siteforms.Visitor{Origin: "https://" + hostname.Hostname, IP: "203.0.113.9", UserAgent: "Mozilla/5.0"}
	challenge, err := w.forms.Challenge(ctx, formKey, visitor)
	if err != nil {
		t.Fatal(err)
	}
	w.advance(4 * time.Second)
	body, _ := json.Marshal(map[string]any{
		"utm":      map[string]string{"source": "instagram", "medium": "bio", "campaign": "", "term": "", "content": ""},
		"referrer": "", "fbclid": "", "gclid": "", "fbp": "", "fbc": "", "variant": "", "idempotencyKey": "5f0c3e2a-9b1d-4c7e-8a6f-2d3b4c5e6f70",
		"website": "", "route": "/", "challengeToken": challenge.Token, "nonce": solveChallenge(t, challenge),
		"fields": map[string]any{"lead.name": "Ana Souza", "lead.phone": "(11) 98888-7777", "lead.email": "ana@exemplo.com.br", "consent": "on"},
	})
	result, err := w.forms.Submit(ctx, formKey, visitor, body)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success.Kind != site.SuccessRedirect || result.Success.Route != "/obrigado" {
		t.Fatalf("success %+v", result.Success)
	}
	if len(w.leads.byPhone) != 1 || len(w.leads.consents) != 1 || len(w.submissions.rows) != 1 || len(w.leads.triggers) != 1 {
		t.Fatalf("leads %d consents %d submissions %d triggers %d", len(w.leads.byPhone), len(w.leads.consents), len(w.submissions.rows), len(w.leads.triggers))
	}
	row := w.submissions.rows[0]
	trigger := w.leads.triggers[0]
	if row.LeadID == "" || !row.Consent.Granted || row.ProjectID != projectID || row.DeploymentID != first.Deployment.ID || row.FormID != "contato" {
		t.Fatalf("submission row %+v", row)
	}
	if trigger.Form != (formlead.Form{Kind: formlead.KindSiteForm, ID: formKey}) || trigger.EntryID != "entry-1" || len(w.official.opened) != 1 || w.official.opened[0].BusinessPhoneID != e2eFollowPhone || w.official.opened[0].PhoneNumber == "" {
		t.Fatalf("trigger %+v follow up %+v", trigger, w.official.opened)
	}

	second, err := w.publisher.Publish(ctx, actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if active, _ := w.formStore.ActiveForm(ctx, formKey); active == nil || active.DeploymentID != second.Deployment.ID {
		t.Fatalf("the second publish moves the form to its deployment: %+v", active)
	}
	if _, err := w.publisher.Rollback(ctx, actor, projectID, first.Deployment.ID); err != nil {
		t.Fatal(err)
	}
	if active, _ := w.formStore.ActiveForm(ctx, formKey); active == nil || active.DeploymentID != first.Deployment.ID {
		t.Fatalf("a rollback reactivates the forms of the restored deployment: %+v", active)
	}
	if restored := w.visit(t, hostname.Hostname, "/"); restored.ETag != manifest.Files[manifest.Routes["/"]].ETag() || string(restored.Body) != home {
		t.Fatalf("the hostname serves the restored deployment: %+v", restored)
	}
	if stored, _ := w.storage.Get(ctx, e2eWorkspace, projectID); stored.Project.PublishedDeploymentID != first.Deployment.ID {
		t.Fatalf("the page points at the restored deployment: %+v", stored.Project)
	}
}
