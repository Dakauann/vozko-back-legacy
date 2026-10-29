package sitepublish_usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"vozko/domain/sitepublish"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

type originBuilder struct {
	candidate Candidate
	requests  []CandidateRequest
}

func (b *originBuilder) Candidate(_ context.Context, req CandidateRequest) (Candidate, error) {
	b.requests = append(b.requests, req)
	out := b.candidate
	out.Assets = make([]BuildAsset, len(b.candidate.Assets))
	for i, a := range b.candidate.Assets {
		a.Body = []byte(strings.ReplaceAll(string(a.Body), originPlaceholder, req.PrimaryOrigin))
		out.Assets[i] = a
	}
	return out, nil
}

func withSiteFiles(c Candidate) Candidate {
	c.Assets = append(c.Assets,
		BuildAsset{Path: "sitemap.xml", ContentType: "application/xml", Body: []byte(`<urlset><url><loc>__VZ_ORIGIN__/</loc></url></urlset>`)},
		BuildAsset{Path: "robots.txt", ContentType: "text/plain", Body: []byte("User-agent: *\nAllow: /\nSitemap: __VZ_ORIGIN__/sitemap.xml\n")},
	)
	return c
}

func uploadedText(r *rig) string {
	var all []string
	for _, body := range r.hosting.blobs {
		all = append(all, string(body))
	}
	return strings.Join(all, "\n")
}

func TestPublishBuildsForThePrimaryOriginAndUploadsNoPlaceholder(t *testing.T) {
	r := newRig(t)
	builder := &originBuilder{candidate: withSiteFiles(htmlCandidate(homeHTML))}
	r.svc.Builder = builder
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err != nil {
		t.Fatal(err)
	}
	if got := builder.requests[0].PrimaryOrigin; got != "https://"+liveHost {
		t.Fatalf("without a custom domain the platform hostname is primary: %q", got)
	}
	r.hostnames.rows["custom"] = sitepublish.HostnameRecord{ID: "custom", WorkspaceID: wsID, ProjectID: projectID, Hostname: "www.bolosdaana.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnameActive}
	r.hostnames.rows["pending"] = sitepublish.HostnameRecord{ID: "pending", WorkspaceID: wsID, ProjectID: projectID, Hostname: "bolos.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnamePendingSSL}
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err != nil {
		t.Fatal(err)
	}
	if got := builder.requests[1].PrimaryOrigin; got != "https://www.bolosdaana.com.br" {
		t.Fatalf("an active custom domain is primary: %q", got)
	}
	text := uploadedText(r)
	if strings.Contains(text, originPlaceholder) || !strings.Contains(text, "https://www.bolosdaana.com.br/sitemap.xml") {
		t.Fatalf("uploaded assets keep the origin placeholder:\n%s", text)
	}
}

func TestPublishRefusesABuildThatKeptTheOriginPlaceholder(t *testing.T) {
	r := newRig(t)
	r.builder.candidate = withSiteFiles(htmlCandidate(homeHTML))
	if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, ErrBuildFailed) {
		t.Fatalf("got %v", err)
	}
	if len(r.hosting.puts) != 0 || len(r.forms.activated) != 0 {
		t.Fatal("nothing is uploaded or activated")
	}
}

func TestPublishNeverUploadsThePrerenderPageOrItsBundle(t *testing.T) {
	r := newRig(t)
	shell := `<!doctype html><html lang="pt-BR"><head><title>Bolos</title><link rel="modulepreload" href="/assets/chunk-SHARED11.js"><script type="module" src="/assets/vz-runtime-ABCD1234.js"></script></head><body><div id="root"><!--vz-prerender--></div><script type="module" data-vz-app src="/assets/app-APP11111.js"></script></body></html>`
	r.builder.candidate = Candidate{
		URL: "https://cand.preview.test/",
		Assets: []BuildAsset{
			{Path: "index.html", ContentType: "text/html; charset=utf-8", Body: []byte(shell)},
			{Path: "assets/app-APP11111.js", ContentType: "text/javascript", Body: []byte(`import "./chunk-SHARED11.js";hydrate()`)},
			{Path: "assets/chunk-SHARED11.js", ContentType: "text/javascript", Body: []byte(`export const react=1`)},
			{Path: "assets/vz-runtime-ABCD1234.js", ContentType: "text/javascript", Body: []byte(`runtime()`)},
			{Path: "assets/prerender-PRE11111.js", ContentType: "text/javascript", Body: []byte(`import "./chunk-SHARED11.js";import "./chunk-ONLYPRE1.js";window.__vzPrerender=1`)},
			{Path: "assets/chunk-ONLYPRE1.js", ContentType: "text/javascript", Body: []byte(`export const server=1`)},
			{Path: "__vz/prerender.html", ContentType: "text/html; charset=utf-8", Body: []byte(`<!doctype html><html><head><link rel="modulepreload" href="/assets/chunk-SHARED11.js"><link rel="modulepreload" href="/assets/chunk-ONLYPRE1.js"><script type="module" src="/assets/prerender-PRE11111.js"></script></head><body><div id="root"></div></body></html>`)},
		},
		Routes:        []BuildRoute{{Route: "/", HTMLPath: "index.html", Interactive: true}},
		PrerenderPath: "__vz/prerender.html",
	}
	r.renderer.markup = map[string]string{"/": "<h1>Bolos caseiros</h1>"}
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := sitepublish.DecodeDeploymentManifest(r.hosting.manifests[res.Deployment.ID])
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"/__vz/prerender.html", "/assets/prerender-PRE11111.js", "/assets/chunk-ONLYPRE1.js"} {
		if _, ok := manifest.Files[gone]; ok {
			t.Fatalf("%s is only for the renderer and must not be served", gone)
		}
	}
	for _, kept := range []string{"/index.html", "/assets/app-APP11111.js", "/assets/chunk-SHARED11.js", "/assets/vz-runtime-ABCD1234.js"} {
		if _, ok := manifest.Files[kept]; !ok {
			t.Fatalf("%s is part of the page and must be served", kept)
		}
	}
}

func TestPublishRecordsThePublicationAndReturnsTheProjectVersion(t *testing.T) {
	r := newRig(t)
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.projects.last(); got != (publication{state: PublicationLive, deploymentID: res.Deployment.ID}) {
		t.Fatalf("got %+v", got)
	}
	if res.ProjectVersion != 12 {
		t.Fatalf("the editor gets the version the publish snapshot left: %d", res.ProjectVersion)
	}
}

func TestPublishLinksWhatsAppAnchorsThroughPageReferences(t *testing.T) {
	r := newRig(t)
	home := strings.Replace(homeHTML, `<a href="https://wa.me/5511999999999">Fale conosco</a>`, `<a href="#" data-vz-whatsapp="phone-1" data-vz-text="Quero um bolo">Fale conosco</a>`, 1)
	r.builder.candidate = htmlCandidate(home)
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.forms.links) != 1 || r.forms.links[0] != projectID+"|phone-1|Quero um bolo|/" {
		t.Fatalf("links = %v", r.forms.links)
	}
	manifest, _ := sitepublish.DecodeDeploymentManifest(r.hosting.manifests[res.Deployment.ID])
	html := string(r.hosting.blobs[manifest.Files[manifest.Routes["/"]].SHA256])
	if !strings.Contains(html, `href="https://vzk.to/phone-1"`) || !slices.Contains(r.deployments.rows[res.Deployment.ID].Outbound, "https://vzk.to/phone-1") {
		t.Fatalf("the anchor points at the tracked link and the link is recorded: %s", html)
	}
	r.forms.linkErr = errors.New("siteforms: the WhatsApp number cannot be used by this page")
	if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, r.forms.linkErr) {
		t.Fatalf("an unusable number refuses the publish: %v", err)
	}
}

func TestPublishActivatesEmbeddedWidgetsByTheirID(t *testing.T) {
	r := newRig(t)
	r.builder.candidate = htmlCandidate(strings.Replace(homeHTML, "</body>", `<vz-webchat widget="pk_chat"></vz-webchat></body>`, 1))
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.forms.activated[0].Widgets; len(got) != 1 || got[0] != "6b1f6d2e-3c4a-4f5b-8e9d-0a1b2c3d4e5f" {
		t.Fatalf("widgets = %v", got)
	}
	manifest, _ := sitepublish.DecodeDeploymentManifest(r.hosting.manifests[res.Deployment.ID])
	served := string(r.hosting.blobs[manifest.Files[manifest.Routes["/"]].SHA256])
	if !strings.Contains(served, `data-vz-webchat-key="pk_chat"`) {
		t.Fatalf("the page carries the widget public key for the runtime: %s", served)
	}
	csp := manifest.ContentSecurityPolicy()
	if !strings.Contains(csp, "https://api.vozko.test/public/webchat/loader.js") || !strings.Contains(csp, "https://api.vozko.test/public/webchat/") {
		t.Fatalf("csp %s", csp)
	}
	r.builder.candidate = htmlCandidate(strings.Replace(homeHTML, "</body>", `<vz-webchat widget="6b1f6d2e-3c4a-4f5b-8e9d-0a1b2c3d4e5f"></vz-webchat></body>`, 1))
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err != nil {
		t.Fatalf("a widget named by its id is accepted: %v", err)
	}
	for _, ref := range []string{"pk_other", "7c2a7e3f-4d5b-4a6c-9fae-1b2c3d4e5f60", "pk_missing"} {
		r.builder.candidate = htmlCandidate(strings.Replace(homeHTML, "</body>", `<vz-webchat widget="`+ref+`"></vz-webchat></body>`, 1))
		if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, ErrWidgetUnknown) {
			t.Fatalf("%s: a widget outside the workspace refuses: %v", ref, err)
		}
	}
	r.widgets.err = errors.New("db down")
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err == nil || errors.Is(err, ErrWidgetUnknown) {
		t.Fatalf("a store failure is not an unknown widget: %v", err)
	}
}

func TestSuspensionTurnsFormsOffAndRestoreTurnsThemBackOn(t *testing.T) {
	r := newRig(t)
	dep := publish(t, r)
	r.subs.err = workspace_plan.ErrSubscriptionNotActive
	if err := r.svc.OnAccessCut(context.Background(), wsID); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.forms.deactivated, projectID) || r.projects.last() != (publication{state: PublicationSuspended, deploymentID: dep.ID}) {
		t.Fatalf("billing cut: deactivated %v publication %+v", r.forms.deactivated, r.projects.last())
	}
	r.subs.err = nil
	before := len(r.forms.activated)
	if err := r.svc.OnAccessRestored(context.Background(), wsID); err != nil {
		t.Fatal(err)
	}
	if len(r.forms.activated) != before+1 || r.forms.activated[before].DeploymentID != dep.ID || r.projects.last() != (publication{state: PublicationLive, deploymentID: dep.ID}) {
		t.Fatalf("restore: activated %+v publication %+v", r.forms.activated, r.projects.last())
	}
	r.forms.deactivated = nil
	if _, err := r.svc.AdminSuspend(context.Background(), "admin-1", projectID, "phishing"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.forms.deactivated, projectID) || r.projects.last().state != PublicationSuspended {
		t.Fatalf("admin suspend: deactivated %v publication %+v", r.forms.deactivated, r.projects.last())
	}
	before = len(r.forms.activated)
	if _, err := r.svc.AdminRestore(context.Background(), "admin-1", projectID, "resolvido"); err != nil {
		t.Fatal(err)
	}
	if len(r.forms.activated) <= before || r.projects.last().state != PublicationLive {
		t.Fatalf("admin restore: activated %+v publication %+v", r.forms.activated, r.projects.last())
	}
}

func TestRollbackReactivatesTheFormsOfTheDeploymentItRestores(t *testing.T) {
	r := newRig(t)
	first := publish(t, r)
	publish(t, r)
	rec, err := r.svc.Rollback(context.Background(), actor, projectID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := r.forms.activated[len(r.forms.activated)-1]
	if last.DeploymentID != first.ID || rec.Status != sitepublish.DeploymentActive {
		t.Fatalf("activated %+v", last)
	}
	if r.projects.last() != (publication{state: PublicationLive, deploymentID: first.ID}) {
		t.Fatalf("publication %+v", r.projects.last())
	}
}

func TestUnpublishTakesThePageOffline(t *testing.T) {
	r := newRig(t)
	dep := publish(t, r)
	if err := r.svc.Unpublish(context.Background(), actor, projectID); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.hosting.hosts[liveHost]; ok {
		t.Fatal("the hostname stops serving")
	}
	if r.deployments.rows[dep.ID].Status != sitepublish.DeploymentSuperseded {
		t.Fatalf("deployment %+v", r.deployments.rows[dep.ID])
	}
	if !slices.Contains(r.forms.deactivated, projectID) || r.projects.last() != (publication{state: PublicationOffline}) {
		t.Fatalf("deactivated %v publication %+v", r.forms.deactivated, r.projects.last())
	}
	if err := r.svc.Unpublish(context.Background(), actor, projectID); err != nil {
		t.Fatalf("unpublishing an offline page is a no-op: %v", err)
	}
	if _, ok := r.hostnames.rows[firstHostnameID(r)]; !ok {
		t.Fatal("the hostnames stay reserved for the next publish")
	}
	r.access.denied["sites:publish"] = true
	if err := r.svc.Unpublish(context.Background(), actor, projectID); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("got %v", err)
	}
}

func firstHostnameID(r *rig) string {
	for id := range r.hostnames.rows {
		return id
	}
	return ""
}
