package sitetools

import (
	"context"
	"maps"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/domain/tools"
	"vozko/domain/workspace"
	"vozko/usecases/copilot/copilottools/tooltest"
	sitepublish_usecase "vozko/usecases/sitepublish"
)

func TestPublishingAlwaysAsksWithAddressesChangesAndGaps(t *testing.T) {
	r := newRig(t)
	r.editor.pages = allRoutes(gappedHome)
	r.publisher.hosts = []sitepublish.HostnameRecord{{Hostname: "sorriso.paginas.example.com", Status: sitepublish.HostnameActive}, {Hostname: "www.sorriso.com.br", Status: sitepublish.HostnamePendingDNS}}
	previous := site.Project{Stack: site.StackReact, Files: maps.Clone(r.storage.view.Files)}
	previous.Files["src/App.jsx"] = site.File{Content: "antigo", Rev: 1}
	r.storage.previous = &previous
	r.publisher.deployments = []sitepublish.DeploymentRecord{{ID: "d1", RevisionID: "rev-1", Status: sitepublish.DeploymentActive}}
	tool := NewSitePublishTool(r.deps)
	for _, mode := range []copilot.Mode{copilot.ModeAsk, copilot.ModeEdit, copilot.ModeFull} {
		if !tool.(copilot.Graded).NeedsApproval(mode) {
			t.Fatalf("publishing asks in %s too", mode)
		}
	}
	fields := tooltest.FieldMap(tool.(copilot.Describer).Describe(context.Background(), r.cc, args(t, `{"summary":"Novo título"}`)))
	if fields["addresses"] != "sorriso.paginas.example.com" || fields["changes"] != "1 arquivos: src/App.jsx" || !strings.Contains(fields["open_gaps"], "alt_missing") || fields["summary"] != "Novo título" {
		t.Fatalf("got %+v", fields)
	}
	if tool.Meta().Action != workspace.ActionPublish || tool.Meta().Resource != workspace.ResourceSites {
		t.Fatalf("got %+v", tool.Meta())
	}
}

func TestAFirstPublishSaysSo(t *testing.T) {
	r := newRig(t)
	fields := tooltest.FieldMap(NewSitePublishTool(r.deps).(copilot.Describer).Describe(context.Background(), r.cc, map[string]interface{}{}))
	if fields["changes"] != publishFirst || fields["addresses"] != publishPlatform || fields["open_gaps"] != publishNoGaps {
		t.Fatalf("got %+v", fields)
	}
}

func TestPublishingRunsAsThePersonAndReportsTheOutcome(t *testing.T) {
	r := newRig(t)
	r.publisher.result = sitepublish_usecase.PublishResult{Deployment: sitepublish.DeploymentRecord{ID: "d2"}, Hostnames: []string{"sorriso.paginas.example.com"}}
	cc := proposed(r, "proposal-1")
	NewSitePublishTool(r.deps).(copilot.Describer).Describe(context.Background(), cc, map[string]interface{}{})
	res := NewSitePublishTool(r.deps).Execute(context.Background(), cc, map[string]interface{}{})
	data := decoded(t, res.Data)
	if res.Status != copilot.StatusOK || data["state"] != "live" || data["deployment_id"] != "d2" || r.publisher.published[0] != (sitepublish_usecase.Actor{UserID: "user-1", WorkspaceID: "ws-1"}) {
		t.Fatalf("got %+v", res)
	}
	r.publisher.result.Held = true
	if data := decoded(t, NewSitePublishTool(r.deps).Execute(context.Background(), cc, map[string]interface{}{}).Data); data["state"] != "held" {
		t.Fatalf("got %+v", data)
	}
	r.publisher.err = &sitepublish_usecase.IssuesFailure{Kind: sitepublish_usecase.ErrGapsOpen, Issues: []site.Issue{{Code: site.CodeConsentRequired, Route: "/", Detail: "contato"}}}
	res = NewSitePublishTool(r.deps).Execute(context.Background(), cc, map[string]interface{}{})
	if res.Status != copilot.StatusError || !strings.Contains(res.Message, "consent_required em /: contato") {
		t.Fatalf("got %+v", res)
	}
}

func TestNewSiteCreatesOnceOpensTheEditorAndShowsACard(t *testing.T) {
	r := newRig(t)
	r.storage.view = nil
	r.cc.View = copilot.View{}
	r.editor.replies = map[copilot.ScreenCommandName]copilot.ScreenReply{copilot.ScreenOpenSite: {OK: true, Data: []byte(`{"projectId":"` + projectID + `","kind":"site","opened":true}`)}}
	tool := NewNewSiteTool(r.deps)
	res := tool.Execute(context.Background(), r.cc, args(t, `{"name":"Clínica Sorriso","starter":"captura-de-leads"}`))
	if res.Status != copilot.StatusOK || res.Opening == nil || !res.Opening.Opened || res.Opening.View != siteView {
		t.Fatalf("got %+v", res)
	}
	if res.Card == nil || res.Card.Destination == nil || res.Card.Destination.Screen != workspace.ScreenSiteEditor || res.Card.Destination.Params["projectId"] != projectID {
		t.Fatalf("the card leads to the editor, got %+v", res.Card)
	}
	created := r.storage.created[0]
	if created.Stack != site.StackReact || created.Starter != "captura-de-leads" || created.UserID != "user-1" || created.Origin.ThreadID != threadID || created.Origin.CommandID == "" {
		t.Fatalf("got %+v", created)
	}
	again := tool.Execute(context.Background(), r.cc, args(t, `{"name":"Clínica Sorriso","starter":"captura-de-leads"}`))
	if again.Status != copilot.StatusOK || len(r.storage.created) != 1 {
		t.Fatal("the same request in the same conversation returns the same page")
	}
	opened := r.editor.sent(copilot.ScreenOpenSite)[0].Args.(siteOpening)
	if opened.ProjectID != projectID || opened.Name != "Clínica Sorriso" {
		t.Fatalf("got %+v", opened)
	}
}

func TestNewSiteWaitsWhenTheAppDoesNotOpenAndStaysOffFocusedSurfaces(t *testing.T) {
	r := newRig(t)
	r.storage.view = nil
	r.cc.View = copilot.View{}
	r.editor.errs = map[copilot.ScreenCommandName]error{copilot.ScreenOpenSite: copilot.ErrScreenUnavailable}
	tool := NewNewSiteTool(r.deps)
	res := tool.Execute(context.Background(), r.cc, args(t, `{"name":"Promo"}`))
	if res.Status != copilot.StatusOK || res.Opening.Opened || decoded(t, res.Data)["state"] != "waiting" || r.storage.created[0].Starter != "blank" {
		t.Fatalf("got %+v", res)
	}
	if res := tool.Execute(context.Background(), r.cc, args(t, `{"name":"  "}`)); res.Status != copilot.StatusError {
		t.Fatalf("got %+v", res)
	}
	scoped := tool.(copilot.Scoped)
	if !scoped.OfferedOn(copilot.View{}) || scoped.OfferedOn(siteView) || tool.Meta().Action != workspace.ActionCreate {
		t.Fatal("new_site lives on the panel and needs sites:create")
	}
}

func TestSiteToolPermissionsFollowTheRBACResources(t *testing.T) {
	want := map[string]copilot.Meta{
		"site_read":              readMeta(),
		"site_grep":              readMeta(),
		"site_look":              readMeta(),
		"site_capture_reference": readMeta(),
		"site_edit":              editMeta(),
		"site_settings":          editMeta(),
	}
	for _, tool := range SiteTools(Deps{}) {
		def := tool.Definition()
		if meta, ok := want[def.Name]; ok && tool.Meta() != meta {
			t.Errorf("%s: got %+v", def.Name, tool.Meta())
		}
		if def.Name != "new_site" && !siteView.Offers(tool) {
			t.Errorf("%s must be offered on the site surface", def.Name)
		}
		for _, other := range []copilot.View{{Surface: copilot.SurfaceEmailDesign, ProjectID: projectID}, {Surface: copilot.SurfaceStudio, ProjectID: projectID, ProjectKind: copilot.StudioImage}} {
			if def.Name != "new_site" && other.Offers(tool) {
				t.Errorf("%s leaks onto %s", def.Name, other.Surface)
			}
		}
		requireNoDash(t, def)
	}
}

func requireNoDash(t *testing.T, def tools.Definition) {
	t.Helper()
	text := def.Description
	for name, p := range def.Parameters {
		text += name + p.Description
	}
	if strings.ContainsAny(text, "–—") {
		t.Errorf("%s carries a long dash", def.Name)
	}
}
