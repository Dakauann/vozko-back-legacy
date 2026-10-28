package sitebuild

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"vozko/domain/inlinemarkup"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

const (
	testKey    = "0123456789abcdef0123456789abcdef-signing"
	testDomain = "paginas.test"
)

func htmlProject() site.Project {
	return site.NewProject(site.StackHTML, map[string]string{
		"site.json":           `{"name":"Clínica","lang":"pt-BR","routes":[{"path":"/","page":"pages/index.html"},{"path":"/obrigado","page":"pages/obrigado.html"}]}`,
		"theme.css":           "@import \"tailwindcss\";\n",
		"pages/index.html":    "<h1>Oi</h1>\n",
		"pages/obrigado.html": "<h1>Obrigado</h1>\n",
	})
}

func okResult() BuildResult {
	return BuildResult{
		Assets: []Asset{
			{Path: "index.html", ContentType: "text/html; charset=utf-8", Body: []byte("<!DOCTYPE html><html><head></head><body>home</body></html>")},
			{Path: "obrigado/index.html", ContentType: "text/html; charset=utf-8", Body: []byte("<!DOCTYPE html><html><head></head><body>thanks</body></html>")},
			{Path: "assets/index-abc.css", ContentType: "text/css; charset=utf-8", Body: []byte("h1{color:red}")},
			{Path: "__vz/prerender.html", ContentType: "text/html; charset=utf-8", Body: []byte("<!DOCTYPE html><html><head></head><body></body></html>")},
		},
		Imports: []site.Import{{Importer: "scripts/a.js", Specifier: "gsap"}},
		Routes:  []RouteOutput{{Route: "/", HTMLPath: "index.html", Interactive: true}, {Route: "/obrigado", HTMLPath: "obrigado/index.html"}},
	}
}

type fixture struct {
	svc      *Service
	builder  *fakeBuilder
	cache    *memoryCache
	media    *fakeMedia
	clock    *clock
	sections fakeSections
	whatsapp *fakeWhatsApp
}

func newFixture(t *testing.T, tweaks ...func(*Config)) *fixture {
	t.Helper()
	f := &fixture{
		builder:  &fakeBuilder{result: okResult()},
		cache:    newMemoryCache(),
		media:    &fakeMedia{urls: map[string]string{"og1": "https://cdn.vozko.test/og1.webp", "m1": "https://cdn.vozko.test/m1.webp"}},
		clock:    &clock{now: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)},
		sections: fakeSections{"html/faq": "<section>FAQ</section>"},
		whatsapp: &fakeWhatsApp{numbers: map[string]string{"phone-1": "5511999990000"}},
	}
	cfg := Config{
		Addresses:  sitepublish.Addresses{Domain: testDomain},
		SigningKey: testKey,
		APIOrigin:  "https://api.vozko.test",
		Runtime:    map[string]string{"vz-runtime.js": "runtime", "pow.js": "pow", "vz-editor-bridge.js": "bridge", "vz-screenshot.js": "screenshot"},
	}
	for _, tweak := range tweaks {
		tweak(&cfg)
	}
	svc, err := NewService(cfg, Deps{Builder: f.builder, Cache: f.cache, Media: f.media, Sections: f.sections, WhatsApp: f.whatsapp, Workspaces: fakeWorkspaceNames{"ws1": "Clínica Sorriso"}, Now: f.clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	f.svc = svc
	return f
}

func previewInput() PreviewInput {
	return PreviewInput{
		WorkspaceID: "ws1",
		ProjectID:   "p1",
		RevisionID:  "r1",
		Project:     htmlProject(),
		Settings: site.Settings{
			Pages:        map[string]site.PageMeta{"/": {Title: "Sorriso", Description: "Implantes", OGMediaID: "og1", Canonical: "https://sorriso.vozko.site/"}, "/obrigado": {NoIndex: true}},
			Integrations: site.Integrations{MetaPixel: "1234567890"},
			Consent:      site.Consent{Text: "Usamos cookies"},
		},
	}
}

func hostOf(t *testing.T, raw string) (string, string) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host, u.Path
}

func TestNewServiceFailsClosedOnMissingDependencies(t *testing.T) {
	good := Config{Addresses: sitepublish.Addresses{Domain: testDomain}, SigningKey: testKey, APIOrigin: "https://api.vozko.test", Runtime: map[string]string{"vz-runtime.js": "r", "pow.js": "p", "vz-editor-bridge.js": "b", "vz-screenshot.js": "s"}}
	deps := Deps{Builder: &fakeBuilder{}, Cache: newMemoryCache(), Media: &fakeMedia{}, WhatsApp: &fakeWhatsApp{}, Workspaces: fakeWorkspaceNames{}}
	cases := map[string]func() (Config, Deps){
		"no builder":    func() (Config, Deps) { d := deps; d.Builder = nil; return good, d },
		"no cache":      func() (Config, Deps) { d := deps; d.Cache = nil; return good, d },
		"no media":      func() (Config, Deps) { d := deps; d.Media = nil; return good, d },
		"no whatsapp":   func() (Config, Deps) { d := deps; d.WhatsApp = nil; return good, d },
		"no workspaces": func() (Config, Deps) { d := deps; d.Workspaces = nil; return good, d },
		"short key":     func() (Config, Deps) { c := good; c.SigningKey = "short"; return c, deps },
		"no domain":     func() (Config, Deps) { c := good; c.Addresses.Domain = ""; return c, deps },
		"no runtime":    func() (Config, Deps) { c := good; c.Runtime = map[string]string{"pow.js": "p"}; return c, deps },
		"bad api url":   func() (Config, Deps) { c := good; c.APIOrigin = "ftp://x"; return c, deps },
		"no screenshot": func() (Config, Deps) {
			c := good
			c.Runtime = map[string]string{"vz-runtime.js": "r", "pow.js": "p", "vz-editor-bridge.js": "b"}
			return c, deps
		},
	}
	for name, build := range cases {
		cfg, d := build()
		if _, err := NewService(cfg, d); err == nil {
			t.Fatalf("%s: expected a refusal", name)
		}
	}
	if _, err := NewService(good, deps); err != nil {
		t.Fatal(err)
	}
}

func TestALoopbackPlatformDomainServesPreviewsOverHTTPOnTheAPIPort(t *testing.T) {
	f := newFixture(t, func(c *Config) {
		c.Addresses = sitepublish.Addresses{Domain: "paginas.localhost", Origins: sitepublish.Origins{LoopbackPort: "4000"}}
	})
	out, err := f.svc.Preview(context.Background(), previewInput())
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(out.URL)
	if err != nil || u.Scheme != "http" || u.Port() != "4000" || !strings.HasPrefix(u.Hostname(), "preview--") || !strings.HasSuffix(u.Hostname(), ".paginas.localhost") || u.Path != "/" {
		t.Fatalf("got %s", out.URL)
	}
	if _, err := f.svc.Serve(context.Background(), u.Host, "/"); err != nil {
		t.Fatalf("the loopback preview serves: %v", err)
	}
}

func TestPreviewBuildsSignsAndServes(t *testing.T) {
	f := newFixture(t)
	out, err := f.svc.Preview(context.Background(), previewInput())
	if err != nil {
		t.Fatal(err)
	}
	if out.Revision != "r1" {
		t.Fatalf("got %+v", out)
	}
	host, path := hostOf(t, out.URL)
	label, suffix, _ := strings.Cut(host, ".")
	token, previewLabel := strings.CutPrefix(label, "preview--")
	if suffix != testDomain || !previewLabel || len(token) != tokenLength || path != "/" || !strings.HasPrefix(out.URL, "https://") {
		t.Fatalf("got %s", out.URL)
	}
	if len(f.builder.builds) != 1 {
		t.Fatalf("got %d builds", len(f.builder.builds))
	}
	req := f.builder.builds[0]
	if req.Mode != ModePreview || req.Shell.API != "https://api.vozko.test" || req.Runtime["pow.js"] != "pow" {
		t.Fatalf("got %+v", req)
	}
	home := req.Shell.Pages["/"]
	if home.Title != "Sorriso" || home.OGImage != "https://cdn.vozko.test/og1.webp" || home.Canonical != "https://sorriso.vozko.site/" || !req.Shell.Pages["/obrigado"].NoIndex {
		t.Fatalf("got %+v", req.Shell.Pages)
	}
	if req.Shell.Integrations["meta_pixel"] != "1234567890" || req.Shell.ConsentText != "Usamos cookies" {
		t.Fatalf("got %+v", req.Shell)
	}
	for path, want := range map[string]string{"/": "home", "/obrigado": "thanks", "/obrigado/": "thanks", "/assets/index-abc.css": "h1{color:red}"} {
		served, err := f.svc.Serve(context.Background(), host, path)
		if err != nil || !strings.Contains(string(served.Body), want) {
			t.Fatalf("%s: got %q %v", path, served.Body, err)
		}
	}
	served, _ := f.svc.Serve(context.Background(), strings.ToUpper(host)+":443", "/")
	if served.ContentType != "text/html; charset=utf-8" {
		t.Fatalf("got %+v", served)
	}
	if _, err := f.svc.Serve(context.Background(), host, "/nada.js"); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestPreviewReusesACachedBuild(t *testing.T) {
	f := newFixture(t)
	first, err := f.svc.Preview(context.Background(), previewInput())
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.Preview(context.Background(), previewInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.builder.builds) != 1 {
		t.Fatalf("got %d builds", len(f.builder.builds))
	}
	if first.URL == second.URL {
		t.Fatal("every preview gets its own signed token")
	}
	other := previewInput()
	other.Project.Files["pages/index.html"] = site.File{Content: "<h1>Novo</h1>\n", Rev: 2}
	if _, err := f.svc.Preview(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if len(f.builder.builds) != 2 {
		t.Fatalf("got %d builds", len(f.builder.builds))
	}
}

func TestPreviewTokensAreScopedAndExpire(t *testing.T) {
	f := newFixture(t)
	out, err := f.svc.Preview(context.Background(), previewInput())
	if err != nil {
		t.Fatal(err)
	}
	host, _ := hostOf(t, out.URL)
	label, _, _ := strings.Cut(host, ".")
	flip := []byte(label)
	if flip[len(flip)-1] == 'a' {
		flip[len(flip)-1] = 'b'
	} else {
		flip[len(flip)-1] = 'a'
	}
	refused := []string{
		string(flip) + "." + testDomain,
		label + ".evil.test",
		label + "." + testDomain + ".evil.test",
		"x." + label + "." + testDomain,
		testDomain,
		label[:10] + "." + testDomain,
		strings.TrimPrefix(label, "preview--") + "." + testDomain,
		"deploy--" + strings.TrimPrefix(label, "preview--") + "." + testDomain,
	}
	for _, h := range refused {
		if _, err := f.svc.Serve(context.Background(), h, "/"); !errors.Is(err, ErrPreviewNotFound) {
			t.Fatalf("%s: got %v", h, err)
		}
	}
	f.clock.now = f.clock.now.Add(PreviewTTL + time.Second)
	if _, err := f.svc.Serve(context.Background(), host, "/"); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("expired: got %v", err)
	}
}

func TestServeFailsClosedWhenTheCacheFails(t *testing.T) {
	f := newFixture(t)
	out, err := f.svc.Preview(context.Background(), previewInput())
	if err != nil {
		t.Fatal(err)
	}
	host, _ := hostOf(t, out.URL)
	f.cache.failGet = errBoom
	if _, err := f.svc.Serve(context.Background(), host, "/"); !errors.Is(err, ErrPreviewUnavailable) {
		t.Fatalf("got %v", err)
	}
	f.cache.failGet = nil
	f.cache.items = map[string]any{}
	if _, err := f.svc.Serve(context.Background(), host, "/"); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestPreviewRefusesWhenTheCacheCannotStore(t *testing.T) {
	f := newFixture(t)
	f.cache.failSet = errBoom
	if _, err := f.svc.Preview(context.Background(), previewInput()); !errors.Is(err, ErrPreviewUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestPreviewRefusesInvalidProjectsBuildErrorsAndForbiddenImports(t *testing.T) {
	f := newFixture(t)
	bad := previewInput()
	bad.Project.Files["../x.html"] = site.File{Content: "x", Rev: 1}
	var invalid *ProjectInvalid
	if _, err := f.svc.Preview(context.Background(), bad); !errors.As(err, &invalid) || !errors.Is(err, ErrProjectInvalid) {
		t.Fatalf("got %v", err)
	}
	f.builder.result = BuildResult{Errors: []BuildError{{Path: "scripts/a.js", Line: 1, Column: 2, Message: "boom"}}}
	var failure *BuildFailure
	if _, err := f.svc.Preview(context.Background(), previewInput()); !errors.As(err, &failure) || !errors.Is(err, ErrBuildFailed) || failure.Errors[0].Message != "boom" {
		t.Fatalf("got %v", err)
	}
	f.builder.result = okResult()
	f.builder.result.Imports = []site.Import{{Importer: "scripts/a.js", Specifier: "axios"}}
	var refused *ImportsRefused
	if _, err := f.svc.Preview(context.Background(), previewInput()); !errors.As(err, &refused) || !errors.Is(err, ErrImportsRefused) {
		t.Fatalf("got %v", err)
	}
	f.builder.result = okResult()
	f.builder.err = errBoom
	if _, err := f.svc.Preview(context.Background(), previewInput()); !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
}

func TestPreviewRefusesAnUnknownOpenGraphImage(t *testing.T) {
	f := newFixture(t)
	in := previewInput()
	in.Settings.Pages["/"] = site.PageMeta{OGMediaID: "missing"}
	if _, err := f.svc.Preview(context.Background(), in); !errors.Is(err, ErrMediaUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestCandidateBuildsForPublishAndServesThePrerenderPage(t *testing.T) {
	f := newFixture(t)
	f.builder.result.PrerenderPath = "__vz/prerender.html"
	out, err := f.svc.Candidate(context.Background(), CandidateInput{WorkspaceID: "ws1", ProjectID: "p1", Project: htmlProject(), Settings: previewInput().Settings, FormKeys: map[string]string{"contato": "fk1"}, PrimaryOrigin: "https://sorriso.vozko.site"})
	if err != nil {
		t.Fatal(err)
	}
	req := f.builder.builds[0]
	if req.Mode != ModePublish || req.Shell.Forms["contato"] != "fk1" || req.Shell.PrimaryOrigin != "https://sorriso.vozko.site" {
		t.Fatalf("got %+v", req)
	}
	if len(out.Result.Assets) != 4 || out.Result.PrerenderPath != "__vz/prerender.html" {
		t.Fatalf("got %+v", out.Result)
	}
	host, _ := hostOf(t, out.URL)
	if _, err := f.svc.Serve(context.Background(), host, "/__vz/prerender.html"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Candidate(context.Background(), CandidateInput{WorkspaceID: "ws1", ProjectID: "p1", Project: htmlProject(), PrimaryOrigin: "http://x.test"}); !errors.Is(err, ErrBuilderRefused) {
		t.Fatalf("got %v", err)
	}
}

func TestExecutePreparesTextsMediaAndSections(t *testing.T) {
	f := newFixture(t)
	f.builder.execute = ExecuteResult{Steps: []site.StepPatch{{Op: site.OpSetText, Index: 0}}}
	project := htmlProject()
	batch, err := site.PrepareBatch(project, []byte(`[
		{"op":"set_text","node":"pages/index.html#0","rev":1,"text":"Olá **mundo**"},
		{"op":"set_text","node":"pages/obrigado.html#0","rev":1,"text":"  "},
		{"op":"replace_image","node":"pages/index.html#0","rev":1,"media_id":"m1"},
		{"op":"insert_section","at":{"page":"pages/index.html"},"rev":1,"library_key":"faq"},
		{"op":"insert_section","at":{"page":"pages/obrigado.html"},"rev":1,"library_key":"nada"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	bases := map[string][]site.File{"pages/index.html": {{Content: "<h2>Antes</h2>\n", Rev: 1}}}
	out, err := f.svc.Execute(context.Background(), ExecuteInput{WorkspaceID: "ws1", ProjectID: "p1", Project: project, Batch: batch, Bases: bases})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Steps) != 1 {
		t.Fatalf("got %+v", out)
	}
	req := f.builder.executes[0]
	want := []inlinemarkup.Run{{Text: "Olá "}, {Text: "mundo", Bold: true}}
	if len(req.Texts[0]) != 2 || req.Texts[0][0] != want[0] || req.Texts[0][1] != want[1] || len(req.Texts[1]) != 0 {
		t.Fatalf("got %+v", req.Texts)
	}
	if _, ok := req.Texts[1]; !ok {
		t.Fatal("a blank text still sends an empty run list")
	}
	if req.Media["m1"] != "https://cdn.vozko.test/m1.webp" || req.Sections["faq"] != "<section>FAQ</section>" || len(req.Sections) != 1 {
		t.Fatalf("got %+v %+v", req.Media, req.Sections)
	}
	if len(req.Bases["pages/index.html"]) != 1 || len(req.Operations) != 5 {
		t.Fatalf("got %+v", req)
	}
	if f.media.calls[0][0] != "ws1" {
		t.Fatalf("media is located in the workspace, got %+v", f.media.calls)
	}
	f.media.err = errBoom
	if _, err := f.svc.Execute(context.Background(), ExecuteInput{WorkspaceID: "ws1", ProjectID: "p1", Project: project, Batch: batch}); !errors.Is(err, ErrMediaUnavailable) {
		t.Fatalf("got %v", err)
	}
}
