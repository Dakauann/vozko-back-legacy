package config

import (
	"errors"
	"strings"
	"testing"
)

func validSitesConfig() SitesConfig {
	return SitesConfig{
		PublicDomain:           "paginas.example",
		BuilderURL:             "http://sitebuilder:8080",
		BuilderToken:           strings.Repeat("b", 32),
		RendererURL:            "http://siterenderer:9222",
		RendererToken:          strings.Repeat("r", 32),
		RendererReferenceProxy: "http://siterender-gateway:3129",
		R2Bucket:               "vozko-sites",
		CloudflareZoneID:       "fedcba9876543210fedcba9876543210",
		CloudflareAPIToken:     "zone-token",
		PreviewSigningKey:      strings.Repeat("p", 32),
		FormsChallengeKey:      strings.Repeat("f", 32),
		CookieSecret:           strings.Repeat("c", 32),
		PublicBaseURL:          "https://api.vozkoia.com",
		ApplicationDomains:     []string{"vozkoia.com"},
		CustomHostnameTarget:   "sites.paginas.example",
	}
}

func TestSitesConfigReportsEveryMissingVariable(t *testing.T) {
	t.Setenv("SITES_PUBLIC_DOMAIN", "")
	t.Setenv("SITES_BUILDER_URL", "")
	t.Setenv("SITES_BUILDER_TOKEN_FILE", "")
	t.Setenv("SITES_RENDERER_URL", "")
	t.Setenv("SITES_RENDERER_TOKEN_FILE", "")
	t.Setenv("SITES_RENDERER_REFERENCE_PROXY", "")
	t.Setenv("SITES_R2_BUCKET", "")
	t.Setenv("SITES_CLOUDFLARE_ZONE_ID", "")
	t.Setenv("SITES_CLOUDFLARE_API_TOKEN", "")
	t.Setenv("SITES_PREVIEW_SIGNING_KEY", "")
	t.Setenv("SITES_FORMS_CHALLENGE_KEY", "")
	t.Setenv("SITES_COOKIE_SECRET", "")
	t.Setenv("API_BASE_URL", "")
	t.Setenv("SITES_APPLICATION_DOMAINS", "")
	t.Setenv("SITES_CUSTOM_HOSTNAME_TARGET", "")
	t.Setenv("PORT", "")

	_, missing := readSitesConfig()

	want := []string{
		"SITES_PUBLIC_DOMAIN", "SITES_BUILDER_URL", "SITES_BUILDER_TOKEN_FILE",
		"SITES_RENDERER_URL", "SITES_RENDERER_TOKEN_FILE", "SITES_RENDERER_REFERENCE_PROXY", "SITES_R2_BUCKET",
		"SITES_CLOUDFLARE_ZONE_ID", "SITES_CLOUDFLARE_API_TOKEN", "SITES_PREVIEW_SIGNING_KEY", "SITES_FORMS_CHALLENGE_KEY", "SITES_COOKIE_SECRET",
		"API_BASE_URL", "SITES_APPLICATION_DOMAINS", "SITES_CUSTOM_HOSTNAME_TARGET", "PORT",
	}
	if strings.Join(missing, ",") != strings.Join(want, ",") {
		t.Fatalf("missing = %v, want %v", missing, want)
	}
}

func TestSitesConfigReadsAndNormalizesValues(t *testing.T) {
	t.Setenv("SITES_PUBLIC_DOMAIN", " Paginas.Example. ")
	t.Setenv("SITES_BUILDER_URL", "http://sitebuilder:8080/")
	t.Setenv("SITES_BUILDER_TOKEN_FILE", "secrets/sites_builder_token")
	t.Setenv("SITES_RENDERER_URL", "http://siterenderer:9222")
	t.Setenv("SITES_RENDERER_TOKEN_FILE", "secrets/sites_renderer_token")
	t.Setenv("SITES_RENDERER_REFERENCE_PROXY", "http://siterender-gateway:3129/")
	t.Setenv("SITES_R2_BUCKET", "vozko-sites")
	t.Setenv("SITES_CLOUDFLARE_ZONE_ID", "zone")
	t.Setenv("SITES_CLOUDFLARE_API_TOKEN", "token")
	t.Setenv("SITES_PREVIEW_SIGNING_KEY", strings.Repeat("p", 32))
	t.Setenv("SITES_FORMS_CHALLENGE_KEY", strings.Repeat("f", 32))
	t.Setenv("SITES_COOKIE_SECRET", strings.Repeat("c", 32))
	t.Setenv("API_BASE_URL", "https://api.vozkoia.com/")
	t.Setenv("SITES_APPLICATION_DOMAINS", "vozkoia.com, app.vozkoia.com")
	t.Setenv("SITES_CUSTOM_HOSTNAME_TARGET", "Sites.Paginas.Example.")
	t.Setenv("PORT", "4000")

	cfg, missing := readSitesConfig()

	if len(missing) != 0 {
		t.Fatalf("missing = %v", missing)
	}
	if cfg.PublicDomain != "paginas.example" {
		t.Fatalf("public domain = %q", cfg.PublicDomain)
	}
	if cfg.BuilderURL != "http://sitebuilder:8080" || cfg.PublicBaseURL != "https://api.vozkoia.com" {
		t.Fatalf("urls not trimmed: %q %q", cfg.BuilderURL, cfg.PublicBaseURL)
	}
	if cfg.BuilderTokenFile != "secrets/sites_builder_token" || cfg.RendererTokenFile != "secrets/sites_renderer_token" {
		t.Fatalf("token files = %q %q", cfg.BuilderTokenFile, cfg.RendererTokenFile)
	}
	if cfg.RendererReferenceProxy != "http://siterender-gateway:3129" {
		t.Fatalf("reference proxy = %q", cfg.RendererReferenceProxy)
	}
	if cfg.CustomHostnameTarget != "sites.paginas.example" {
		t.Fatalf("custom hostname target = %q", cfg.CustomHostnameTarget)
	}
	if len(cfg.ApplicationDomains) != 2 || cfg.ApplicationDomains[1] != "app.vozkoia.com" {
		t.Fatalf("application domains = %v", cfg.ApplicationDomains)
	}
}

func TestSitesConfigValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*SitesConfig)
		want   string
	}{
		{"valid", func(*SitesConfig) {}, ""},
		{"public domain not a domain", func(c *SitesConfig) { c.PublicDomain = "not a domain" }, "SITES_PUBLIC_DOMAIN"},
		{"public domain under the application", func(c *SitesConfig) { c.PublicDomain = "sites.vozkoia.com" }, "application"},
		{"public domain is the application", func(c *SitesConfig) { c.PublicDomain = "vozkoia.com" }, "application"},
		{"public domain above the application", func(c *SitesConfig) { c.PublicDomain = "com" }, "SITES_PUBLIC_DOMAIN"},
		{"local platform domain", func(c *SitesConfig) {
			c.PublicDomain, c.CustomHostnameTarget = "paginas.localhost", "cname.paginas.localhost"
		}, ""},
		{"public domain under the api host", func(c *SitesConfig) { c.PublicDomain = "x.api.vozkoia.com" }, "application"},
		{"builder url relative", func(c *SitesConfig) { c.BuilderURL = "sitebuilder:8080" }, "SITES_BUILDER_URL"},
		{"renderer url relative", func(c *SitesConfig) { c.RendererURL = "/renderer" }, "SITES_RENDERER_URL"},
		{"reference proxy relative", func(c *SitesConfig) { c.RendererReferenceProxy = "siterender-gateway:3129" }, "SITES_RENDERER_REFERENCE_PROXY"},
		{"reference proxy with a path", func(c *SitesConfig) { c.RendererReferenceProxy = "http://siterender-gateway:3129/x" }, "SITES_RENDERER_REFERENCE_PROXY"},
		{"short builder token", func(c *SitesConfig) { c.BuilderToken = "short" }, "SITES_BUILDER_TOKEN"},
		{"short renderer token", func(c *SitesConfig) { c.RendererToken = strings.Repeat("r", 31) }, "SITES_RENDERER_TOKEN"},
		{"renderer token outside the url safe alphabet", func(c *SitesConfig) { c.RendererToken = strings.Repeat("r", 31) + "/" }, "SITES_RENDERER_TOKEN"},
		{"renderer token reused from the builder", func(c *SitesConfig) { c.RendererToken = c.BuilderToken }, "different"},
		{"short preview key", func(c *SitesConfig) { c.PreviewSigningKey = "short" }, "SITES_PREVIEW_SIGNING_KEY"},
		{"short challenge key", func(c *SitesConfig) { c.FormsChallengeKey = "short" }, "SITES_FORMS_CHALLENGE_KEY"},
		{"preview key reused as challenge key", func(c *SitesConfig) { c.FormsChallengeKey = c.PreviewSigningKey }, "different"},
		{"short cookie secret", func(c *SitesConfig) { c.CookieSecret = "short" }, "SITES_COOKIE_SECRET"},
		{"cookie secret reused from the preview key", func(c *SitesConfig) { c.CookieSecret = c.PreviewSigningKey }, "different"},
		{"cookie secret reused from the builder", func(c *SitesConfig) { c.CookieSecret = c.BuilderToken }, "different"},
		{"builder token reused as challenge key", func(c *SitesConfig) { c.FormsChallengeKey = c.BuilderToken }, "different"},
		{"api base not https", func(c *SitesConfig) { c.PublicBaseURL = "http://api.vozkoia.com" }, "API_BASE_URL"},
		{"application domain invalid", func(c *SitesConfig) { c.ApplicationDomains = []string{"bad domain"} }, "SITES_APPLICATION_DOMAINS"},
		{"custom hostname target not a domain", func(c *SitesConfig) { c.CustomHostnameTarget = "x" }, "SITES_CUSTOM_HOSTNAME_TARGET"},
		{"custom hostname target outside the sites zone", func(c *SitesConfig) { c.CustomHostnameTarget = "sites.other.example" }, "SITES_CUSTOM_HOSTNAME_TARGET"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validSitesConfig()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want mention of %q", err, tc.want)
			}
		})
	}
}

func TestSitesTokensAreReadFromTheirFiles(t *testing.T) {
	cfg := SitesConfig{BuilderTokenFile: "b", RendererTokenFile: "r"}
	files := map[string]string{"b": strings.Repeat("b", 32) + "\r\n", "r": " " + strings.Repeat("r", 32) + "\n"}
	read := func(path string) ([]byte, error) {
		v, ok := files[path]
		if !ok {
			return nil, errors.New("no such file")
		}
		return []byte(v), nil
	}
	if err := cfg.readTokens(read); err != nil {
		t.Fatal(err)
	}
	if cfg.BuilderToken != strings.Repeat("b", 32) || cfg.RendererToken != strings.Repeat("r", 32) {
		t.Fatalf("tokens = %q %q", cfg.BuilderToken, cfg.RendererToken)
	}
	cfg.RendererTokenFile = "missing"
	if err := cfg.readTokens(read); err == nil || !strings.Contains(err.Error(), "SITES_RENDERER_TOKEN_FILE") {
		t.Fatalf("a missing token file is fatal, got %v", err)
	}
}
