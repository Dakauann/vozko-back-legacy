package sitepublish

import (
	"errors"
	"strings"
	"testing"

	"vozko/domain/site"
)

func validManifest() DeploymentManifest {
	html := NewFileEntry([]byte("<!doctype html><html></html>"), "text/html; charset=utf-8", "/index.html")
	js := NewFileEntry([]byte("console.log(1)"), "text/javascript", "/assets/index-5XKZ3QJ2.js")
	return DeploymentManifest{
		DeploymentID: "dep_1",
		ProjectID:    "11111111-2222-3333-4444-555555555555",
		APIOrigin:    "https://api.example.com",
		Files:        map[string]FileEntry{"/index.html": html, "/assets/index-5XKZ3QJ2.js": js},
		Routes:       map[string]string{"/": "/index.html"},
		Integrations: []site.IntegrationKind{site.IntegrationGA4},
		Policy:       site.ContentPolicyFor([]site.IntegrationKind{site.IntegrationGA4}, "https://api.example.com", true),
	}
}

func TestNewFileEntryHashesTheBodyAndMarksHashedAssetsImmutable(t *testing.T) {
	entry := NewFileEntry([]byte("abc"), "text/css", "/assets/app-AB12CD34.css")
	if entry.SHA256 != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" || entry.Size != 3 || !entry.Immutable {
		t.Fatalf("got %+v", entry)
	}
	cases := map[string]bool{
		"/index.html":                              false,
		"/robots.txt":                              false,
		"/favicon.ico":                             false,
		"/assets/index-5XKZ3QJ2.js":                true,
		"/assets/chunk.8f3a9b2c1d.js":              true,
		"/_m/" + strings.Repeat("a", 64) + ".webp": true,
		"/sobre/index.html":                        false,
		"/assets/logo.svg":                         false,
	}
	for path, want := range cases {
		if got := ImmutablePath(path); got != want {
			t.Fatalf("%s: got %v", path, got)
		}
	}
}

func TestManifestEncodesAndDecodesStrictly(t *testing.T) {
	m := validManifest()
	raw, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeDeploymentManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if back.Version != ManifestVersion || back.Routes["/"] != "/index.html" || len(back.Files) != 2 || len(back.Policy.Frame) == 0 || len(back.Policy.Script) == 0 {
		t.Fatalf("got %+v", back)
	}
	if _, err := DecodeDeploymentManifest([]byte(strings.Replace(string(raw), `"v":1`, `"v":1,"extra":true`, 1))); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("unknown fields refuse, got %v", err)
	}
}

func TestManifestRefusesBrokenContracts(t *testing.T) {
	cases := map[string]func(*DeploymentManifest){
		"deployment id": func(m *DeploymentManifest) { m.DeploymentID = "a/b" },
		"project id":    func(m *DeploymentManifest) { m.ProjectID = "" },
		"api origin":    func(m *DeploymentManifest) { m.APIOrigin = "http://api.example.com" },
		"no files":      func(m *DeploymentManifest) { m.Files = nil },
		"relative path": func(m *DeploymentManifest) { m.Files["x.js"] = m.Files["/index.html"] },
		"dot segments":  func(m *DeploymentManifest) { m.Files["/a/../b.js"] = m.Files["/index.html"] },
		"bad sha":       func(m *DeploymentManifest) { e := m.Files["/index.html"]; e.SHA256 = "zz"; m.Files["/index.html"] = e },
		"no type": func(m *DeploymentManifest) {
			e := m.Files["/index.html"]
			e.ContentType = ""
			m.Files["/index.html"] = e
		},
		"route target":   func(m *DeploymentManifest) { m.Routes["/sobre"] = "/sobre/index.html" },
		"route path":     func(m *DeploymentManifest) { m.Routes["Sobre"] = "/index.html" },
		"no home":        func(m *DeploymentManifest) { m.Routes = map[string]string{} },
		"not found file": func(m *DeploymentManifest) { m.NotFound = "/404.html" },
		"integration":    func(m *DeploymentManifest) { m.Integrations = []site.IntegrationKind{"bogus"} },
		"policy source": func(m *DeploymentManifest) {
			m.Policy.Frame = append(m.Policy.Frame, "https://x.example; script-src *")
		},
		"variant route": func(m *DeploymentManifest) { m.Variants = map[string]map[string]string{"/x": {"b": "/index.html"}} },
		"variant file":  func(m *DeploymentManifest) { m.Variants = map[string]map[string]string{"/": {"b": "/b.html"}} },
		"variant key": func(m *DeploymentManifest) {
			m.Variants = map[string]map[string]string{"/": {"control": "/index.html"}}
		},
	}
	for name, mutate := range cases {
		m := validManifest()
		mutate(&m)
		if _, err := m.Encode(); !errors.Is(err, ErrManifestInvalid) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
}

func TestStorageKeys(t *testing.T) {
	if BlobKey("ab") != "blobs/ab" || ManifestKey("dep_1") != "deploys/dep_1.json" {
		t.Fatal("keys changed")
	}
}

func TestDeploymentKeyNamesTheImmutableAddress(t *testing.T) {
	if DeploymentKey("dep_1") != "deploy:dep_1" || HostKey("a.example") != "host:a.example" {
		t.Fatal("kv keys changed")
	}
}
