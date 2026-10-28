package sitepublish_usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

func TestPublishBuildsUploadsAndFlipsTheHostKeys(t *testing.T) {
	r := newRig(t)
	existing := sitepublish.SHA256Hex([]byte("console.log(1)"))
	r.hosting.blobs[existing] = []byte("console.log(1)")

	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	dep := r.deployments.rows[res.Deployment.ID]
	if dep.Status != sitepublish.DeploymentActive || dep.PublishedBy != "person:"+userID || dep.RevisionID != "rev-publish-"+dep.ID {
		t.Fatalf("deployment = %+v", dep)
	}
	if res.Held || len(res.Hostnames) != 1 || res.Hostnames[0] != "bolos-loja-da-ana.paginas.test" {
		t.Fatalf("result = %+v", res)
	}
	if slices.Contains(r.hosting.puts, existing) {
		t.Fatal("blobs already in the bucket are never uploaded again")
	}
	serving := r.hosting.serving(t, "bolos-loja-da-ana.paginas.test")
	if serving.DeploymentID != dep.ID || serving.Suspended {
		t.Fatalf("serving = %+v", serving)
	}
	if _, ok := r.hosting.deployments[dep.ID]; !ok {
		t.Fatal("the immutable deployment address must be written")
	}
	manifest, err := sitepublish.DecodeDeploymentManifest(r.hosting.manifests[dep.ID])
	if err != nil {
		t.Fatal(err)
	}
	home := manifest.Files[manifest.Routes["/"]]
	if !home.CountView || home.Immutable {
		t.Fatalf("a page without the runtime is counted by the edge and never immutable: %+v", home)
	}
	html := string(r.hosting.blobs[home.SHA256])
	if strings.Contains(html, "index-5XKZ3QJ2.js") || strings.Contains(html, "vz-runtime") {
		t.Fatalf("a static page drops the app bundle and the idle runtime: %s", html)
	}
	if strings.Contains(html, "cdn.vozko.test") || !strings.Contains(html, sitepublish.MediaPathPrefix+sitepublish.SHA256Hex([]byte("webp-bytes"))+".webp") {
		t.Fatalf("library media is copied into the deployment: %s", html)
	}
	css := manifest.Files["/assets/app-AB12CD34.css"]
	if strings.Contains(string(r.hosting.blobs[css.SHA256]), "cdn.vozko.test") || !css.Immutable {
		t.Fatalf("css media rewritten and hashed css immutable: %+v", css)
	}
	if len(r.forms.activated) != 1 || r.forms.activated[0].DeploymentID != dep.ID {
		t.Fatalf("forms = %+v", r.forms.activated)
	}
	if len(r.projects.published) != 1 || r.projects.published[0].SourceRevisionID != "rev-7" {
		t.Fatalf("publish revision = %+v", r.projects.published)
	}
	if !slices.Contains(r.deployments.rows[dep.ID].Outbound, "https://wa.me/5511999999999") {
		t.Fatalf("outbound = %v", r.deployments.rows[dep.ID].Outbound)
	}
}

func TestPublishingAgainSupersedesTheLiveDeployment(t *testing.T) {
	r := newRig(t)
	first, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if r.deployments.rows[first.Deployment.ID].Status != sitepublish.DeploymentSuperseded || r.hosting.serving(t, "bolos-loja-da-ana.paginas.test").DeploymentID != second.Deployment.ID {
		t.Fatal("the second publish must take over")
	}
	if len(r.hostnames.rows) != 1 {
		t.Fatalf("the platform subdomain is created once, got %d", len(r.hostnames.rows))
	}
}

func TestReactPagesArePrerenderedAndKeepTheRuntimeForForms(t *testing.T) {
	r := newRig(t)
	shell := strings.Replace(formHTML, `<form data-vz-form="contato">`, `<div id="root"><!--vz-prerender--></div><form data-vz-form="contato">`, 1)
	c := htmlCandidate(shell)
	c.PrerenderPath = "prerender.js"
	c.Imports = []site.Import{{Importer: "src/main.tsx", Specifier: "react"}}
	r.builder.candidate = c
	r.renderer.markup = map[string]string{"/": `<p>Olá</p>`}
	r.projects.snap.Project = site.NewProject(site.StackReact, map[string]string{"src/pages/Home.tsx": `<form data-vz-form="contato">`})

	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if r.builder.requests[0].FormKeys["contato"] != "fk_contato" {
		t.Fatalf("form keys reach the build: %+v", r.builder.requests[0].FormKeys)
	}
	entry := r.deployments.rows[res.Deployment.ID].Files["/index.html"]
	html := string(r.hosting.blobs[entry.SHA256])
	if !strings.Contains(html, `<div id="root"><p>Olá</p></div>`) || !strings.Contains(html, "vz-runtime-ABCD1234.js") || strings.Contains(html, "index-5XKZ3QJ2.js") || entry.CountView {
		t.Fatalf("html = %s entry = %+v", html, entry)
	}
	if got := r.deployments.rows[res.Deployment.ID].Forms.Manifests; len(got) != 1 || got[0].ID != "contato" {
		t.Fatalf("manifests = %+v", got)
	}
}

func TestPublishRefusals(t *testing.T) {
	cases := map[string]struct {
		arrange func(r *rig)
		want    error
	}{
		"no permission": {func(r *rig) { r.access.denied["sites:publish"] = true }, ErrAccessDenied},
		"unpaid":        {func(r *rig) { r.subs.err = workspace_plan.ErrSubscriptionNotActive }, ErrSubscriptionInactive},
		"build errors": {func(r *rig) {
			r.builder.candidate.Errors = []BuildError{{Path: "index.html", Line: 3, Message: "unexpected token"}}
		}, ErrBuildFailed},
		"forbidden import": {func(r *rig) {
			r.builder.candidate.Imports = []site.Import{{Importer: "scripts/main.js", Specifier: "https://evil.example/x.js"}}
		}, ErrPublishBlocked},
		"external script": {func(r *rig) {
			r.builder.candidate = htmlCandidate(strings.Replace(homeHTML, "</head>", `<script src="https://evil.example/x.js"></script></head>`, 1))
		}, ErrPublishBlocked},
		"open gap": {func(r *rig) {
			r.builder.candidate = htmlCandidate(strings.Replace(homeHTML, ` alt="Bolo de cenoura"`, "", 1))
		}, ErrGapsOpen},
		"form without key": {func(r *rig) {
			r.builder.candidate = htmlCandidate(formHTML)
			r.forms.keys = map[string]string{}
		}, ErrFormKeyMissing},
		"media missing":    {func(r *rig) { r.media.err = errors.New("gone") }, ErrMediaUnavailable},
		"prerender broken": {func(r *rig) { r.builder.candidate.PrerenderPath = "p.js"; r.renderer.err = errors.New("timeout") }, ErrPrerenderFailed},
		"forms down":       {func(r *rig) { r.forms.err = errors.New("down") }, ErrFormsUnavailable},
		"under review": {func(r *rig) {
			r.abuse.rows = append(r.abuse.rows, sitepublish.AbuseCase{ID: "c1", ProjectID: projectID, Status: sitepublish.AbuseHeld})
		}, ErrUnderReview},
		"suspended": {func(r *rig) {
			r.abuse.rows = append(r.abuse.rows, sitepublish.AbuseCase{ID: "c1", ProjectID: projectID, Status: sitepublish.AbuseSuspended})
		}, ErrSiteSuspended},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			c.arrange(r)
			_, err := r.svc.Publish(context.Background(), actor, projectID)
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
			for _, dep := range r.deployments.rows {
				if dep.Status == sitepublish.DeploymentActive {
					t.Fatal("a refused publish never goes live")
				}
			}
			if name != "forms down" && len(r.hosting.hosts) != 0 {
				t.Fatalf("a refused publish writes no host key: %v", keys(r.hosting.hosts))
			}
		})
	}
}

func TestRefusedBuildsAreRecordedAsFailedWithTheirGaps(t *testing.T) {
	r := newRig(t)
	r.builder.candidate = htmlCandidate(strings.Replace(homeHTML, ` alt="Bolo de cenoura"`, "", 1))
	_, err := r.svc.Publish(context.Background(), actor, projectID)
	var issues *IssuesFailure
	if !errors.As(err, &issues) || issues.Issues[0].Code != site.CodeAltMissing {
		t.Fatalf("got %v", err)
	}
	for _, dep := range r.deployments.rows {
		if dep.Status != sitepublish.DeploymentFailed || len(dep.Gaps) != 1 {
			t.Fatalf("deployment = %+v", dep)
		}
	}
}

func TestLookalikePagesAreHeldForReviewInsteadOfGoingLive(t *testing.T) {
	r := newRig(t)
	r.builder.candidate = htmlCandidate(strings.Replace(formHTML, "<h1>Contato</h1>", "<h1>Atualize seu cadastro Itaú</h1>", 1))
	r.projects.snap.Project = site.NewProject(site.StackHTML, map[string]string{"index.html": formHTML})
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Held || r.deployments.rows[res.Deployment.ID].Status != sitepublish.DeploymentReady {
		t.Fatalf("result = %+v", res)
	}
	if len(r.hosting.hosts) != 0 || len(r.hosting.deployments) != 0 || len(r.forms.activated) != 0 {
		t.Fatal("a held page is never served")
	}
	if len(r.abuse.rows) != 1 || r.abuse.rows[0].Source != sitepublish.AbuseHeuristic || r.abuse.rows[0].DeploymentID != res.Deployment.ID {
		t.Fatalf("cases = %+v", r.abuse.rows)
	}

	if _, err := r.svc.AdminRestore(context.Background(), "admin-1", projectID, "conferido, padaria real"); err != nil {
		t.Fatal(err)
	}
	if r.deployments.rows[res.Deployment.ID].Status != sitepublish.DeploymentActive || r.abuse.rows[0].Status != sitepublish.AbuseReleased {
		t.Fatal("an admin release takes the held deployment live")
	}
	if r.hosting.serving(t, "bolos-loja-da-ana.paginas.test").DeploymentID != res.Deployment.ID {
		t.Fatal("released deployment is served")
	}
}

type failingSecondHost struct {
	*fakeHosting
	writes int
}

func (f *failingSecondHost) PutHost(ctx context.Context, host string, value []byte) error {
	f.writes++
	if f.writes == 2 {
		return errors.New("kv refused")
	}
	return f.fakeHosting.PutHost(ctx, host, value)
}

func TestAFailedKeyFlipPutsTheLiveDeploymentBack(t *testing.T) {
	r := newRig(t)
	first := publish(t, r)
	r.hostnames.rows["custom"] = sitepublish.HostnameRecord{ID: "custom", WorkspaceID: wsID, ProjectID: projectID, Hostname: "www.cliente.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnameActive}
	flaky := &failingSecondHost{fakeHosting: r.hosting}
	r.svc.Hosting = flaky
	if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, ErrHostingUnavailable) {
		t.Fatalf("got %v", err)
	}
	if r.hosting.serving(t, liveHost).DeploymentID != first.ID || r.deployments.rows[first.ID].Status != sitepublish.DeploymentActive {
		t.Fatal("the live deployment keeps serving after a failed flip")
	}
}

func TestAnAdminReleaseWaitsForTheRunningPublishBeforeClosingTheCase(t *testing.T) {
	r := newRig(t)
	r.builder.candidate = htmlCandidate(strings.Replace(formHTML, "<h1>Contato</h1>", "<h1>Atualize seu cadastro Itaú</h1>", 1))
	r.projects.snap.Project = site.NewProject(site.StackHTML, map[string]string{"index.html": formHTML})
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil || !res.Held {
		t.Fatalf("got %+v %v", res, err)
	}
	r.locks.held[publishLockKey(projectID)] = "another-publish"
	if _, err := r.svc.AdminRestore(context.Background(), "admin-1", projectID, "conferido"); !errors.Is(err, ErrPublishInProgress) {
		t.Fatalf("got %v", err)
	}
	if r.abuse.rows[0].Status != sitepublish.AbuseHeld || r.deployments.rows[res.Deployment.ID].Status != sitepublish.DeploymentReady {
		t.Fatalf("a held lock leaves the case open to retry: %+v", r.abuse.rows[0])
	}
	delete(r.locks.held, publishLockKey(projectID))
	if _, err := r.svc.AdminRestore(context.Background(), "admin-1", projectID, "conferido"); err != nil {
		t.Fatal(err)
	}
	if r.abuse.rows[0].Status != sitepublish.AbuseReleased || r.deployments.rows[res.Deployment.ID].Status != sitepublish.DeploymentActive {
		t.Fatalf("the retry releases it: %+v", r.abuse.rows[0])
	}
}
