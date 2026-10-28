package sitepublish

import (
	"strings"
	"testing"

	"vozko/domain/site"
)

func servingManifest() DeploymentManifest {
	return DeploymentManifest{
		Version: ManifestVersion, DeploymentID: "d1", ProjectID: "p1", APIOrigin: "https://api.vozkoia.com",
		Files: map[string]FileEntry{
			"/index.html":         {SHA256: strings.Repeat("a", 64), ContentType: "text/html; charset=utf-8", CountView: true},
			"/b.html":             {SHA256: strings.Repeat("b", 64), ContentType: "text/html; charset=utf-8"},
			"/assets/app-5d2f.js": {SHA256: strings.Repeat("c", 64), ContentType: "text/javascript", Immutable: true},
			"/robots.txt":         {SHA256: strings.Repeat("d", 64), ContentType: "text/plain"},
			"/404.html":           {SHA256: strings.Repeat("e", 64), ContentType: "text/html; charset=utf-8"},
		},
		Routes:   map[string]string{"/": "/index.html"},
		Variants: map[string]map[string]string{"/": {"b": "/b.html"}},
		NotFound: "/404.html",
		Policy:   site.ContentPolicy{Script: []string{"https://www.googletagmanager.com"}, Frame: []string{"https://www.youtube-nocookie.com/embed/"}},
	}
}

func TestContentSecurityPolicyComesFromTheManifestOnly(t *testing.T) {
	csp := servingManifest().ContentSecurityPolicy()
	for _, want := range []string{
		"default-src 'self'",
		"script-src 'self' https://www.googletagmanager.com",
		"connect-src 'self' https://api.vozkoia.com",
		"img-src 'self' data: blob:",
		"frame-src https://www.youtube-nocookie.com/embed/",
		"form-action 'self' https://api.vozkoia.com",
		"base-uri 'none'",
		"object-src 'none'",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("missing %q in %s", want, csp)
		}
	}
	m := servingManifest()
	m.Policy.Frame = nil
	if !strings.Contains(m.ContentSecurityPolicy(), "frame-src 'none'") {
		t.Fatal("no frames unless the manifest lists them")
	}
}

func TestResolvePicksTheFileRouteOrNotFoundPage(t *testing.T) {
	m := servingManifest()
	cases := []struct {
		path, file, route string
		found             bool
	}{
		{"/", "/index.html", "/", true},
		{"/assets/app-5d2f.js", "/assets/app-5d2f.js", "", true},
		{"/robots.txt", "/robots.txt", "", true},
		{"/nada", "/404.html", "", false},
	}
	for _, c := range cases {
		got := m.Resolve(c.path)
		if got.File != c.file || got.Route != c.route || got.Found != c.found {
			t.Fatalf("%s: got %+v", c.path, got)
		}
	}
	m.NotFound = ""
	if got := m.Resolve("/nada"); got.File != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestVariantFileFallsBackToControl(t *testing.T) {
	m := servingManifest()
	if file, key := m.VariantFile("/", "b"); file != "/b.html" || key != "b" {
		t.Fatalf("got %s %s", file, key)
	}
	if file, key := m.VariantFile("/", "c"); file != "/index.html" || key != ControlVariant {
		t.Fatalf("got %s %s", file, key)
	}
	if file, key := m.VariantFile("/", ControlVariant); file != "/index.html" || key != ControlVariant {
		t.Fatalf("got %s %s", file, key)
	}
}

func TestExperimentKeepsTheVisitorsVariantAndRollsByWeight(t *testing.T) {
	e := Experiment{Route: "/", Variants: []Variant{{Key: ControlVariant, Weight: 30}, {Key: "b", Weight: 70}}}
	if key, fresh := e.Pick("b", func(int) int { return 0 }); key != "b" || fresh {
		t.Fatalf("got %s %v", key, fresh)
	}
	if key, fresh := e.Pick("gone", func(int) int { return 29 }); key != ControlVariant || !fresh {
		t.Fatalf("got %s %v", key, fresh)
	}
	if key, _ := e.Pick("", func(total int) int { return total - 1 }); key != "b" {
		t.Fatalf("got %s", key)
	}
	s := ServingSettings{Experiments: []Experiment{e}}
	if _, ok := s.ExperimentFor("/"); !ok {
		t.Fatal("the home route has an experiment")
	}
	if _, ok := s.ExperimentFor("/x"); ok {
		t.Fatal("other routes have none")
	}
}

func TestCachingDependsOnTheFileKind(t *testing.T) {
	m := servingManifest()
	cases := []struct {
		file     string
		personal bool
		want     string
	}{
		{"/index.html", false, "public, max-age=60"},
		{"/index.html", true, "private, no-store"},
		{"/assets/app-5d2f.js", false, "public, max-age=31536000, immutable"},
		{"/robots.txt", false, "public, max-age=300"},
	}
	for _, c := range cases {
		if got := m.Files[c.file].CacheControl(c.personal); got != c.want {
			t.Fatalf("%s: got %s", c.file, got)
		}
	}
	if got := m.Files["/index.html"].ETag(); got != `"`+strings.Repeat("a", 32)+`"` {
		t.Fatalf("got %s", got)
	}
	if !m.Files["/index.html"].IsHTML() || m.Files["/robots.txt"].IsHTML() {
		t.Fatal("html is told by the content type")
	}
}

func TestServedPathIsDecodedAndTrimmed(t *testing.T) {
	cases := map[string]string{"/": "/", "": "/", "/promo/": "/promo", "/promo//": "/promo", "/caf%C3%A9": "/café", "/%zz": "/%zz"}
	for raw, want := range cases {
		if got := ServedPath(raw); got != want {
			t.Fatalf("%q: got %q", raw, got)
		}
	}
}
