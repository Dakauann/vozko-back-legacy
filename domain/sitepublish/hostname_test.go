package sitepublish

import (
	"errors"
	"strings"
	"testing"

	"vozko/domain/site"
)

var ours = []string{"paginas.example", "previa.example", "app.example"}

func TestValidateCustomHostnameNormalisesAndRefuses(t *testing.T) {
	accepted := map[string]string{
		"www.Clinica-Sorriso.com.br": "www.clinica-sorriso.com.br",
		" clinica.com.br. ":          "clinica.com.br",
		"xn--sorriso-7ua.com":        "xn--sorriso-7ua.com",
		"lp.paginasexample.com":      "lp.paginasexample.com",
	}
	for raw, want := range accepted {
		got, err := ValidateCustomHostname(raw, ours)
		if err != nil || got != want {
			t.Fatalf("%q: got %q %v", raw, got, err)
		}
	}
	invalid := []string{"", "localhost", "clinica", "*.clinica.com", "clinica..com", "-a.com", "a_b.com", "192.168.0.1", "clínica.com", "https://clinica.com", "clinica.com/path", strings.Repeat("a", 64) + ".com"}
	for _, raw := range invalid {
		if _, err := ValidateCustomHostname(raw, ours); !errors.Is(err, ErrHostnameInvalid) {
			t.Fatalf("%q: got %v", raw, err)
		}
	}
	forbidden := []string{"paginas.example", "loja.paginas.example", "x.previa.example", "app.example", "site.local", "a.test", "b.localhost", "c.internal", "d.invalid", "e.example"}
	for _, raw := range forbidden {
		if _, err := ValidateCustomHostname(raw, ours); !errors.Is(err, ErrHostnameForbidden) {
			t.Fatalf("%q: got %v", raw, err)
		}
	}
}

func TestPlatformSlugsAreDNSLabelsAndNeverReserved(t *testing.T) {
	if got := SlugCandidate("Clínica Sorriso & Cia!"); got != "clinica-sorriso-cia" {
		t.Fatalf("got %q", got)
	}
	if got := SlugCandidate(strings.Repeat("ab ", 40)); len(got) > MaxSlugLength || strings.HasSuffix(got, "-") {
		t.Fatalf("got %q", got)
	}
	if got := PlatformSlug("Black Friday", "Clínica Sorriso"); got != "black-friday-clinica-sorriso" {
		t.Fatalf("got %q", got)
	}
	for _, slug := range []string{"abc", "clinica-sorriso", "a1-b2", strings.Repeat("a", 63)} {
		if err := ValidateSlug(slug); err != nil {
			t.Fatalf("%q: got %v", slug, err)
		}
	}
	for _, slug := range []string{"", "ab", "Abc", "a--b", "xn--abc", "-abc", "abc-", "a_b", "a.b", strings.Repeat("a", 64), "açaí"} {
		if err := ValidateSlug(slug); !errors.Is(err, ErrSlugInvalid) {
			t.Fatalf("%q: got %v", slug, err)
		}
	}
	for _, slug := range []string{"www", "api", "admin", "preview", "status", "mail"} {
		if err := ValidateSlug(slug); !errors.Is(err, ErrSlugReserved) {
			t.Fatalf("%q: got %v", slug, err)
		}
	}
	if err := ValidateSlug("minhamarca", "MinhaMarca"); !errors.Is(err, ErrSlugReserved) {
		t.Fatalf("brand words come from configuration: %v", err)
	}
	host, err := PlatformHostname("clinica-sorriso", "Paginas.Example")
	if err != nil || host != "clinica-sorriso.paginas.example" {
		t.Fatalf("got %q %v", host, err)
	}
	if _, err := PlatformHostname("www", "paginas.example"); !errors.Is(err, ErrSlugReserved) {
		t.Fatalf("got %v", err)
	}
	if _, err := PlatformHostname("abc", "localhost"); !errors.Is(err, ErrHostnameInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestServingSettingsRoundTripThroughKV(t *testing.T) {
	s := ServingSettings{
		DeploymentID: "dep_01HX",
		PasswordHash: "pbkdf2-sha256$210000$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g",
		Experiments:  []Experiment{{Route: "/", Variants: []Variant{{Key: ControlVariant, Weight: 50}, {Key: "b", Weight: 50}}}},
		Redirects:    []site.Redirect{{From: "/promo", To: "/", Status: 301}},
	}
	raw, err := s.Encode()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"v":1,"deployment":"dep_01HX","password_hash":"pbkdf2-sha256$210000$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g","experiments":[{"route":"/","variants":[{"key":"control","weight":50},{"key":"b","weight":50}]}],"redirects":[{"from":"/promo","to":"/","status":301}]}`
	if string(raw) != want {
		t.Fatalf("got  %s\nwant %s", raw, want)
	}
	back, err := DecodeServing(raw)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := back.Encode()
	if string(again) != want {
		t.Fatalf("got %s", again)
	}
	suspended, err := ServingSettings{DeploymentID: "dep_1", Suspended: true}.Encode()
	if err != nil || string(suspended) != `{"v":1,"deployment":"dep_1","suspended":true}` {
		t.Fatalf("got %s %v", suspended, err)
	}
	if HostKey("clinica.com.br") != "host:clinica.com.br" {
		t.Fatal("host key")
	}
}

func TestServingSettingsRefuseWhatTheWorkerCannotTrust(t *testing.T) {
	good := func() ServingSettings {
		return ServingSettings{DeploymentID: "dep_1", Experiments: []Experiment{{Route: "/", Variants: []Variant{{Key: ControlVariant, Weight: 70}, {Key: "b", Weight: 30}}}}}
	}
	cases := map[string]func(*ServingSettings){
		"no deployment":  func(s *ServingSettings) { s.DeploymentID = "" },
		"bad deployment": func(s *ServingSettings) { s.DeploymentID = "dep 1" },
		"plain password": func(s *ServingSettings) { s.PasswordHash = "hunter2" },
		"weak hash": func(s *ServingSettings) {
			s.PasswordHash = "pbkdf2-sha256$1000$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g"
		},
		"weights not 100": func(s *ServingSettings) { s.Experiments[0].Variants[1].Weight = 20 },
		"negative weight": func(s *ServingSettings) {
			s.Experiments[0].Variants = []Variant{{Key: ControlVariant, Weight: 110}, {Key: "b", Weight: -10}}
		},
		"one variant":         func(s *ServingSettings) { s.Experiments[0].Variants = []Variant{{Key: ControlVariant, Weight: 100}} },
		"no control":          func(s *ServingSettings) { s.Experiments[0].Variants[0].Key = "a" },
		"repeated variant":    func(s *ServingSettings) { s.Experiments[0].Variants[1].Key = ControlVariant },
		"bad variant key":     func(s *ServingSettings) { s.Experiments[0].Variants[1].Key = "B!" },
		"bad route":           func(s *ServingSettings) { s.Experiments[0].Route = "obrigado" },
		"repeated experiment": func(s *ServingSettings) { s.Experiments = append(s.Experiments, s.Experiments[0]) },
		"bad redirect":        func(s *ServingSettings) { s.Redirects = []site.Redirect{{From: "/", To: "/a", Status: 301}} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := good()
			mutate(&s)
			if _, err := s.Encode(); !errors.Is(err, ErrServingInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
	for _, raw := range []string{``, `{"deployment":"dep_1"}`, `{"v":2,"deployment":"dep_1"}`, `{"v":1,"deployment":"dep_1","extra":1}`} {
		if _, err := DecodeServing([]byte(raw)); !errors.Is(err, ErrServingInvalid) {
			t.Fatalf("%q: got %v", raw, err)
		}
	}
}
