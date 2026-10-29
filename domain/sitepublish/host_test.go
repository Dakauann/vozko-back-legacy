package sitepublish

import "testing"

const addressID = "7f1c2a9e-4b3d-4e5f-8a6b-1c2d3e4f5a6b"

func TestPlatformDomainsListsEveryDomainWeOwn(t *testing.T) {
	got := PlatformDomains("vozko.app", []string{"vozkoia.com", "api.vozkoia.com"})
	if len(got) != 3 || got[0] != "vozko.app" || got[2] != "api.vozkoia.com" {
		t.Errorf("got %v", got)
	}
}

func TestOriginsServeLoopbackNamesOverHTTPOnTheAPIPort(t *testing.T) {
	origins := Origins{LoopbackPort: "4000"}
	cases := map[string]string{
		"abc.paginas.localhost":  "http://abc.paginas.localhost:4000",
		"ABC.Paginas.Localhost.": "http://abc.paginas.localhost:4000",
		"localhost":              "http://localhost:4000",
		"abc.vozko.app":          "https://abc.vozko.app",
		"notlocalhost.com":       "https://notlocalhost.com",
	}
	for host, want := range cases {
		if got := origins.Of(host); got != want {
			t.Errorf("%s: got %q want %q", host, got, want)
		}
	}
}

func TestAddressesPutPreviewsAndDeploymentsBesidePagesOnOneDomain(t *testing.T) {
	a := Addresses{Domain: "Vozko.App"}
	if got := a.PreviewURL("abc123"); got != "https://preview--abc123.vozko.app/" {
		t.Errorf("got %q", got)
	}
	if got := a.DeploymentURL(addressID); got != "https://deploy--"+addressID+".vozko.app/" {
		t.Errorf("got %q", got)
	}
	local := Addresses{Domain: "paginas.localhost", Origins: Origins{LoopbackPort: "4000"}}
	if got := local.PreviewURL("abc123"); got != "http://preview--abc123.paginas.localhost:4000/" {
		t.Errorf("got %q", got)
	}
}

func TestAddressesNameTheDomainFreePageAddressesLiveOn(t *testing.T) {
	if got := (Addresses{Domain: "Vozko.App."}).PagesDomain(); got != "vozko.app" {
		t.Errorf("got %q", got)
	}
}

func TestAddressesReadTokensAndDeploymentsBackFromHosts(t *testing.T) {
	a := Addresses{Domain: "vozko.app"}
	if token, ok := a.PreviewToken("PREVIEW--abc123.vozko.app:443"); !ok || token != "abc123" {
		t.Errorf("got %q %v", token, ok)
	}
	for _, host := range []string{"abc123.vozko.app", "preview--.vozko.app", "preview--abc.other.app", "x.preview--abc.vozko.app"} {
		if _, ok := a.PreviewToken(host); ok {
			t.Errorf("%s is not a preview", host)
		}
	}
	if id, ok := a.DeploymentID("deploy--" + addressID + ".vozko.app"); !ok || id != addressID {
		t.Errorf("got %q %v", id, ok)
	}
	for _, host := range []string{"deploy--abc.vozko.app", addressID + ".vozko.app", "deploy--" + addressID + ".other.app"} {
		if _, ok := a.DeploymentID(host); ok {
			t.Errorf("%s is not a deployment address", host)
		}
	}
	if !a.Within("sorriso.vozko.app") || !a.Within("vozko.app") || a.Within("vozko.app.evil.test") || a.Within("xvozko.app") {
		t.Error("within means the domain or one of its subdomains")
	}
}

func TestOnlyPreviewOriginsCountAsPreviews(t *testing.T) {
	a := Addresses{Domain: "vozko.app"}
	if !a.PreviewOrigin("https://preview--abc.vozko.app") {
		t.Error("a preview origin")
	}
	for _, origin := range []string{"http://preview--abc.vozko.app", "https://sorriso.vozko.app", "https://preview--abc.vozko.app/path", "https://preview--abc.evil.test"} {
		if a.PreviewOrigin(origin) {
			t.Errorf("%q is not a preview origin", origin)
		}
	}
	local := Addresses{Domain: "paginas.localhost", Origins: Origins{LoopbackPort: "4000"}}
	if !local.PreviewOrigin("http://preview--abc.paginas.localhost:4000") || local.PreviewOrigin("http://preview--abc.paginas.localhost:3000") {
		t.Error("a local preview origin carries the API port")
	}
}

func TestAPreviewOriginNamesItsCanonicalHost(t *testing.T) {
	a := Addresses{Domain: "vozko.app"}
	if host, ok := a.PreviewOriginHost("https://preview--abc.vozko.app"); !ok || host != "preview--abc.vozko.app" {
		t.Errorf("got %q %v", host, ok)
	}
	for _, origin := range []string{"https://sorriso.vozko.app", "http://preview--abc.vozko.app", "https://preview--abc.vozko.app/x", ""} {
		if host, ok := a.PreviewOriginHost(origin); ok || host != "" {
			t.Errorf("%q named %q", origin, host)
		}
	}
}
