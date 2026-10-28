package siteserve_usecase

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

const (
	projectID    = "7f1c2a9e-4b3d-4e5f-8a6b-1c2d3e4f5a6b"
	deploymentID = "0d9e8f7a-6b5c-4d3e-2f1a-0b9c8d7e6f5a"
	pageHost     = "sorriso.paginas.test"
	browser      = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Safari/604.1"
	homeHTML     = `<!doctype html><html lang="pt-BR"><body><h1>Clareamento em <vz-text param="utm_term" fallback="Curitiba">Curitiba</vz-text></h1></body></html>`
)

var errDown = errors.New("down")

type fakeDirectory struct {
	values map[string]sitepublish.ServingSettings
	err    error
	asked  []string
}

func (d *fakeDirectory) Serving(_ context.Context, key string) (sitepublish.ServingSettings, bool, error) {
	d.asked = append(d.asked, key)
	if d.err != nil {
		return sitepublish.ServingSettings{}, false, d.err
	}
	s, ok := d.values[key]
	return s, ok, nil
}

type fakeFiles struct {
	manifests   map[string]sitepublish.DeploymentManifest
	blobs       map[string][]byte
	manifestErr error
}

func (f *fakeFiles) Manifest(_ context.Context, id string) (sitepublish.DeploymentManifest, error) {
	if f.manifestErr != nil {
		return sitepublish.DeploymentManifest{}, f.manifestErr
	}
	m, ok := f.manifests[id]
	if !ok {
		return sitepublish.DeploymentManifest{}, errDown
	}
	return m, nil
}

func (f *fakeFiles) Blob(_ context.Context, sha string) ([]byte, error) {
	b, ok := f.blobs[sha]
	if !ok {
		return nil, errDown
	}
	return b, nil
}

type fakeVisits struct {
	recorded []sitepublish.Visit
	err      error
}

func (v *fakeVisits) Record(_ context.Context, visit sitepublish.Visit) error {
	if v.err != nil {
		return v.err
	}
	v.recorded = append(v.recorded, visit)
	return nil
}

type fakeLimiter struct {
	allowed bool
	err     error
	keys    []string
}

func (l *fakeLimiter) Allow(key string) (bool, time.Duration, error) {
	l.keys = append(l.keys, key)
	return l.allowed, time.Minute, l.err
}

type rig struct {
	directory *fakeDirectory
	files     *fakeFiles
	visits    *fakeVisits
	beacons   *fakeLimiter
	passwords *fakeLimiter
	service   *Service
	now       time.Time
}

func sha(body string) string {
	return sitepublish.SHA256Hex([]byte(body))
}

func newRig(t *testing.T) *rig {
	t.Helper()
	files := map[string]string{
		"/index.html":             homeHTML,
		"/b.html":                 `<!doctype html><html lang="pt-BR"><body>B</body></html>`,
		"/assets/app-5d2f8a1c.js": "console.log(1)",
		"/404.html":               "<!doctype html><html><body>404</body></html>",
	}
	manifest := sitepublish.DeploymentManifest{
		Version: sitepublish.ManifestVersion, DeploymentID: deploymentID, ProjectID: projectID, APIOrigin: "https://api.vozkoia.com",
		Files:  map[string]sitepublish.FileEntry{},
		Routes: map[string]string{"/": "/index.html"}, NotFound: "/404.html",
		Variants: map[string]map[string]string{"/": {"b": "/b.html"}},
		Policy:   site.ContentPolicy{Script: []string{"https://www.googletagmanager.com"}},
	}
	blobs := map[string][]byte{}
	for path, body := range files {
		entry := sitepublish.NewFileEntry([]byte(body), "text/html; charset=utf-8", path)
		if strings.HasSuffix(path, ".js") {
			entry.ContentType = "text/javascript"
		}
		entry.CountView = path == "/index.html"
		manifest.Files[path] = entry
		blobs[sha(body)] = []byte(body)
	}
	r := &rig{
		directory: &fakeDirectory{values: map[string]sitepublish.ServingSettings{
			sitepublish.HostKey(pageHost):             {Version: 1, DeploymentID: deploymentID},
			sitepublish.HostKey("www.sorriso.com.br"): {Version: 1, DeploymentID: deploymentID},
			sitepublish.DeploymentKey(deploymentID):   {Version: 1, DeploymentID: deploymentID},
		}},
		files:     &fakeFiles{manifests: map[string]sitepublish.DeploymentManifest{deploymentID: manifest}, blobs: blobs},
		visits:    &fakeVisits{},
		beacons:   &fakeLimiter{allowed: true},
		passwords: &fakeLimiter{allowed: true},
		now:       time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC),
	}
	service, err := New(Deps{
		Directory: r.directory, Files: r.files, Visits: r.visits, Beacons: r.beacons, Passwords: r.passwords,
		Now: func() time.Time { return r.now }, Roll: func(total int) int { return total - 1 },
	}, Config{
		Addresses:       sitepublish.Addresses{Domain: "paginas.test"},
		PlatformDomains: sitepublish.PlatformDomains("paginas.test", []string{"vozkoia.com"}),
		CookieSecret:    []byte(strings.Repeat("k", 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	r.service = service
	return r
}

func get(host, path string) Request {
	target, query, _ := strings.Cut(path, "?")
	return Request{Host: host, Method: "GET", Path: target, RawQuery: query, Cookie: func(string) string { return "" }, UserAgent: browser, ClientIP: "203.0.113.9"}
}

func (r *rig) serve(t *testing.T, req Request) Outcome {
	t.Helper()
	out := r.service.Serve(context.Background(), req)
	return out
}

func TestNewRefusesMissingDependencies(t *testing.T) {
	if _, err := New(Deps{}, Config{Addresses: sitepublish.Addresses{Domain: "paginas.test"}, CookieSecret: []byte(strings.Repeat("k", 32))}); err == nil {
		t.Fatal("dependencies are required")
	}
	r := newRig(t)
	if _, err := New(Deps{Directory: r.directory, Files: r.files, Visits: r.visits, Beacons: r.beacons, Passwords: r.passwords}, Config{Addresses: sitepublish.Addresses{Domain: "paginas.test"}, CookieSecret: []byte("short")}); err == nil {
		t.Fatal("a short cookie secret is refused")
	}
}

func TestClaimsOnlyPageHosts(t *testing.T) {
	r := newRig(t)
	cases := map[string]bool{
		pageHost:            true,
		"nada.paginas.test": true,
		"paginas.test":      true,
		"deploy--" + deploymentID + ".paginas.test": true,
		"deploy--mfrggzdfmztwq.paginas.test":        true,
		"preview--mfrggzdfmztwq.paginas.test":       false,
		"PREVIEW--abc.Paginas.Test:443":             false,
		"www.sorriso.com.br":                        true,
		"www.desconhecido.com.br":                   false,
		"api.vozkoia.com":                           false,
		"host.docker.internal":                      false,
		"localhost:4000":                            false,
		"127.0.0.1:4000":                            false,
		"www.sorriso.com.br.:443":                   true,
	}
	for host, want := range cases {
		got, err := r.service.Claims(context.Background(), host)
		if err != nil || got != want {
			t.Errorf("%s: got %v %v", host, got, err)
		}
	}
	for _, key := range r.directory.asked {
		if strings.Contains(key, "vozkoia.com") || strings.Contains(key, "internal") || strings.Contains(key, "paginas.test") {
			t.Errorf("%s must not be looked up", key)
		}
	}
	r.directory.err = errDown
	if _, err := r.service.Claims(context.Background(), "www.sorriso.com.br"); err == nil {
		t.Error("a failed lookup is an error, never a guess")
	}
}

func TestDeploymentAddressesReadTheDeploymentKeyReadOnly(t *testing.T) {
	r := newRig(t)
	out := r.serve(t, get("deploy--"+deploymentID+".paginas.test", "/"))
	if out.Kind != KindContent || !out.Content.NoIndex {
		t.Fatalf("got %+v", out)
	}
	if !slices.Contains(r.directory.asked, sitepublish.DeploymentKey(deploymentID)) {
		t.Fatalf("asked %v", r.directory.asked)
	}
	if out := r.serve(t, get("deploy--not-an-id.paginas.test", "/")); out.Kind != KindNotFound || !slices.Contains(r.directory.asked, sitepublish.HostKey("deploy--not-an-id.paginas.test")) {
		t.Fatalf("a malformed deployment address is just an unknown host: %+v %v", out, r.directory.asked)
	}
}

func TestServesTheHomeWithItsPolicyAndCountsTheView(t *testing.T) {
	r := newRig(t)
	req := get(pageHost, "/?utm_source=instagram&utm_term=São%20Paulo")
	req.Referer = "https://www.instagram.com/p/abc"
	out := r.serve(t, req)
	if out.Kind != KindContent || out.Content.Missing || out.Content.NoIndex {
		t.Fatalf("got %+v", out)
	}
	if !strings.Contains(string(out.Content.Body), ">São Paulo</vz-text>") {
		t.Fatalf("got %s", out.Content.Body)
	}
	if out.Content.CacheControl != "public, max-age=60" || out.Content.ETag == "" || !strings.Contains(out.Content.Policy, "https://www.googletagmanager.com") {
		t.Fatalf("got %+v", out.Content)
	}
	if len(r.visits.recorded) != 1 {
		t.Fatalf("got %+v", r.visits.recorded)
	}
	v := r.visits.recorded[0]
	if v.Event != sitepublish.VisitView || v.Route != "/" || v.Variant != sitepublish.ControlVariant || v.Referrer != "https://www.instagram.com" || v.UTMSource != "instagram" || v.ProjectID != projectID || v.Day != time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("got %+v", v)
	}
}

func TestViewsAreNotCountedForBotsHeadPreviewsOrFailedStores(t *testing.T) {
	r := newRig(t)
	bot := get(pageHost, "/?utm_term=Rio")
	bot.UserAgent = "Googlebot/2.1"
	out := r.serve(t, bot)
	if !strings.Contains(string(out.Content.Body), ">Curitiba</vz-text>") {
		t.Fatalf("bots see the fallback, got %s", out.Content.Body)
	}
	head := get(pageHost, "/")
	head.Method = "HEAD"
	r.serve(t, head)
	preview := r.serve(t, get("deploy--"+deploymentID+".paginas.test", "/"))
	if !preview.Content.NoIndex || preview.Kind != KindContent {
		t.Fatalf("got %+v", preview)
	}
	if len(r.visits.recorded) != 0 {
		t.Fatalf("got %+v", r.visits.recorded)
	}
	r.visits.err = errDown
	if out := r.serve(t, get(pageHost, "/")); out.Kind != KindContent {
		t.Fatalf("a lost count never takes the page down, got %+v", out)
	}
}

func TestFilesAndMissingPaths(t *testing.T) {
	r := newRig(t)
	asset := r.serve(t, get(pageHost, "/assets/app-5d2f8a1c.js"))
	if asset.Kind != KindContent || asset.Content.CacheControl != "public, max-age=31536000, immutable" || asset.Content.ETag != "" {
		t.Fatalf("got %+v", asset.Content)
	}
	missing := r.serve(t, get(pageHost, "/nada"))
	if missing.Kind != KindContent || !missing.Content.Missing || string(missing.Content.Body) != "<!doctype html><html><body>404</body></html>" {
		t.Fatalf("got %+v", missing)
	}
	m := r.files.manifests[deploymentID]
	m.NotFound = ""
	r.files.manifests[deploymentID] = m
	if out := r.serve(t, get(pageHost, "/nada")); out.Kind != KindNotFound {
		t.Fatalf("got %+v", out)
	}
}

func TestFailuresAnswerClosed(t *testing.T) {
	r := newRig(t)
	if out := r.serve(t, get("outra.paginas.test", "/")); out.Kind != KindNotFound {
		t.Fatalf("got %+v", out)
	}
	if out := r.serve(t, get("deploy--mfrggzdfmztwq.paginas.test", "/")); out.Kind != KindNotFound {
		t.Fatalf("got %+v", out)
	}
	r.files.manifestErr = errDown
	if out := r.serve(t, get(pageHost, "/")); out.Kind != KindUnavailable {
		t.Fatalf("got %+v", out)
	}
	r.files.manifestErr = nil
	r.files.blobs = map[string][]byte{}
	if out := r.serve(t, get(pageHost, "/")); out.Kind != KindUnavailable {
		t.Fatalf("got %+v", out)
	}
	r.directory.err = errDown
	if out := r.serve(t, get(pageHost, "/")); out.Kind != KindUnavailable {
		t.Fatalf("got %+v", out)
	}
	post := get(pageHost, "/")
	post.Method = "POST"
	r.directory.err = nil
	if out := r.serve(t, post); out.Kind != KindMethodNotAllowed || strings.Join(out.Allow, ",") != "GET,HEAD" {
		t.Fatalf("got %+v", out)
	}
}

func TestSuspendedAndRedirectsAnswerBeforeFiles(t *testing.T) {
	r := newRig(t)
	r.directory.values[sitepublish.HostKey(pageHost)] = sitepublish.ServingSettings{Version: 1, DeploymentID: deploymentID, Suspended: true}
	if out := r.serve(t, get(pageHost, "/")); out.Kind != KindSuspended {
		t.Fatalf("got %+v", out)
	}
	r.directory.values[sitepublish.HostKey(pageHost)] = sitepublish.ServingSettings{Version: 1, DeploymentID: deploymentID, Redirects: []site.Redirect{{From: "/velho", To: "/", Status: 301}, {From: "/promo", To: "https://loja.example.com/x", Status: 302}}}
	r.files.manifestErr = errDown
	if out := r.serve(t, get(pageHost, "/velho/")); out.Kind != KindRedirect || out.Location != "/" || out.Status != 301 {
		t.Fatalf("got %+v", out)
	}
	if out := r.serve(t, get(pageHost, "/promo")); out.Location != "https://loja.example.com/x" || out.Status != 302 {
		t.Fatalf("got %+v", out)
	}
}

func TestExperimentsPinTheVariantWithACookie(t *testing.T) {
	r := newRig(t)
	r.directory.values[sitepublish.HostKey(pageHost)] = sitepublish.ServingSettings{Version: 1, DeploymentID: deploymentID, Experiments: []sitepublish.Experiment{{Route: "/", Variants: []sitepublish.Variant{{Key: "control", Weight: 50}, {Key: "b", Weight: 50}}}}}
	out := r.serve(t, get(pageHost, "/"))
	if !strings.Contains(string(out.Content.Body), `data-vz-variant="b"`) || out.Content.CacheControl != "private, no-store" {
		t.Fatalf("got %+v", out.Content)
	}
	if len(out.Cookies) != 1 || out.Cookies[0] != (Cookie{Name: VariantCookie, Value: "b", Path: "/", MaxAge: VariantCookieLifetime}) {
		t.Fatalf("got %+v", out.Cookies)
	}
	pinned := get(pageHost, "/")
	pinned.Cookie = func(name string) string {
		if name == VariantCookie {
			return "control"
		}
		return ""
	}
	again := r.serve(t, pinned)
	if len(again.Cookies) != 0 || !strings.Contains(string(again.Content.Body), `data-vz-variant="control"`) || !strings.Contains(string(again.Content.Body), "Clareamento") {
		t.Fatalf("got %+v", again)
	}
}

func hashOf(t *testing.T, password string) string {
	t.Helper()
	salt := []byte("0123456789abcdef")
	key, err := pbkdf2.Key(sha256.New, password, salt, 100_000, 32)
	if err != nil {
		t.Fatal(err)
	}
	return "pbkdf2-sha256$100000$" + base64.RawURLEncoding.EncodeToString(salt) + "$" + base64.RawURLEncoding.EncodeToString(key)
}

func TestPasswordGate(t *testing.T) {
	r := newRig(t)
	settings := sitepublish.ServingSettings{Version: 1, DeploymentID: deploymentID, PasswordHash: hashOf(t, "sorriso")}
	r.directory.values[sitepublish.HostKey(pageHost)] = settings
	if out := r.serve(t, get(pageHost, "/promo?a=1")); out.Kind != KindPasswordPrompt || out.Prompt != (Prompt{Next: "/promo?a=1"}) {
		t.Fatalf("got %+v", out)
	}
	attempt := func(password string) Outcome {
		req := get(pageHost, PasswordPath)
		req.Method, req.Password, req.Next = "POST", password, "//evil.test"
		return r.serve(t, req)
	}
	if out := attempt("errada"); out.Kind != KindPasswordPrompt || out.Prompt != (Prompt{Next: "/", Failed: true}) {
		t.Fatalf("got %+v", out)
	}
	if r.passwords.keys[0] != "203.0.113.9|"+pageHost {
		t.Fatalf("got %v", r.passwords.keys)
	}
	out := attempt("sorriso")
	if out.Kind != KindRedirect || out.Status != 303 || out.Location != "/" || len(out.Cookies) != 1 || out.Cookies[0].Name != PassCookie || !out.Cookies[0].HTTPOnly || out.Cookies[0].MaxAge != sitepublish.PassTokenLifetime {
		t.Fatalf("got %+v", out)
	}
	token := out.Cookies[0].Value
	inside := get(pageHost, "/")
	inside.Cookie = func(name string) string {
		if name == PassCookie {
			return token
		}
		return ""
	}
	if page := r.serve(t, inside); page.Kind != KindContent || page.Content.CacheControl != "private, no-store" {
		t.Fatalf("got %+v", page)
	}
	r.passwords.allowed = false
	if out := attempt("sorriso"); out.Kind != KindTooManyRequests {
		t.Fatalf("got %+v", out)
	}
	r.passwords.allowed, r.passwords.err = true, errDown
	if out := attempt("sorriso"); out.Kind != KindTooManyRequests {
		t.Fatalf("a limiter failure refuses, got %+v", out)
	}
}

func beacon(body string) Request {
	req := get(pageHost, BeaconPath)
	req.Method, req.Body = "POST", []byte(body)
	return req
}

func TestBeaconRecordsRuntimeEvents(t *testing.T) {
	r := newRig(t)
	out := r.serve(t, beacon(`{"event":"form_submit","route":"/","variant":"b","referrer":"https://www.google.com","utm":{"source":"google","medium":"cpc"},"extra":1}`))
	if out.Kind != KindAccepted || len(r.visits.recorded) != 1 {
		t.Fatalf("got %+v %+v", out, r.visits.recorded)
	}
	v := r.visits.recorded[0]
	if v.Event != sitepublish.VisitFormSubmit || v.Variant != "b" || v.Referrer != "https://www.google.com" || v.UTMSource != "google" || len(v.Visitor) != sitepublish.VisitorHashLength {
		t.Fatalf("got %+v", v)
	}
	if r.beacons.keys[0] != "203.0.113.9" {
		t.Fatalf("got %v", r.beacons.keys)
	}
	r.serve(t, beacon(`{"event":"view","route":"/","variant":"Bad!"}`))
	if r.visits.recorded[1].Variant != sitepublish.ControlVariant {
		t.Fatalf("got %+v", r.visits.recorded[1])
	}
}

func TestBeaconRefusals(t *testing.T) {
	r := newRig(t)
	cases := map[string]struct {
		req  func() Request
		want Kind
	}{
		"get":           {func() Request { return get(pageHost, BeaconPath) }, KindMethodNotAllowed},
		"bad json":      {func() Request { return beacon(`{`) }, KindBadRequest},
		"unknown event": {func() Request { return beacon(`{"event":"click","route":"/"}`) }, KindBadRequest},
		"bad route":     {func() Request { return beacon(`{"event":"view","route":"/A"}`) }, KindBadRequest},
		"too large":     {func() Request { return beacon(strings.Repeat(" ", MaxBeaconBytes+1)) }, KindPayloadTooLarge},
		"no ip": {func() Request {
			req := beacon(`{"event":"view","route":"/"}`)
			req.ClientIP = ""
			return req
		}, KindTooManyRequests},
		"bot": {func() Request {
			req := beacon(`{"event":"view","route":"/"}`)
			req.UserAgent = "curl/8"
			return req
		}, KindAccepted},
		"preview": {func() Request {
			req := beacon(`{"event":"view","route":"/"}`)
			req.Host = "deploy--" + deploymentID + ".paginas.test"
			return req
		}, KindAccepted},
	}
	for name, c := range cases {
		if out := r.serve(t, c.req()); out.Kind != c.want {
			t.Errorf("%s: got %+v", name, out)
		}
	}
	if len(r.visits.recorded) != 0 {
		t.Fatalf("got %+v", r.visits.recorded)
	}
	r.beacons.allowed = false
	if out := r.serve(t, beacon(`{"event":"view","route":"/"}`)); out.Kind != KindTooManyRequests {
		t.Fatalf("got %+v", out)
	}
	r.beacons.allowed, r.beacons.err = true, errDown
	if out := r.serve(t, beacon(`{"event":"view","route":"/"}`)); out.Kind != KindTooManyRequests {
		t.Fatalf("got %+v", out)
	}
	r.beacons.err, r.visits.err = nil, errDown
	if out := r.serve(t, beacon(`{"event":"view","route":"/"}`)); out.Kind != KindUnavailable {
		t.Fatalf("got %+v", out)
	}
}
