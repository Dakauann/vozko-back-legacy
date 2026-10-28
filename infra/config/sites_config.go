package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"vozko/domain/emailaddress"
	"vozko/infra/siterendergate"
)

const minSitesSecretLength = 32

type SitesConfig struct {
	PublicDomain string

	BuilderURL        string
	BuilderTokenFile  string
	BuilderToken      string
	RendererURL       string
	RendererTokenFile string
	RendererToken     string

	RendererReferenceProxy string

	R2Bucket string

	CloudflareZoneID   string
	CloudflareAPIToken string

	PreviewSigningKey string
	FormsChallengeKey string
	CookieSecret      string

	PublicBaseURL      string
	ApplicationDomains []string

	CustomHostnameTarget string

	APIPort string
}

func loadSitesConfig() SitesConfig {
	cfg, missing := readSitesConfig()
	if len(missing) > 0 {
		log.Fatalf("required env var %s is not set", missing[0])
	}
	if err := cfg.readTokens(os.ReadFile); err != nil {
		log.Fatalf("invalid sites configuration: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid sites configuration: %v", err)
	}
	return cfg
}

func readSitesConfig() (SitesConfig, []string) {
	var missing []string
	required := func(key string) string {
		v := strings.TrimSpace(trimEnv(key))
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}
	cfg := SitesConfig{
		PublicDomain:           normalizedDomain(strings.TrimSuffix(strings.ToLower(required("SITES_PUBLIC_DOMAIN")), ".")),
		BuilderURL:             strings.TrimRight(required("SITES_BUILDER_URL"), "/"),
		BuilderTokenFile:       required("SITES_BUILDER_TOKEN_FILE"),
		RendererURL:            strings.TrimRight(required("SITES_RENDERER_URL"), "/"),
		RendererTokenFile:      required("SITES_RENDERER_TOKEN_FILE"),
		RendererReferenceProxy: strings.TrimRight(required("SITES_RENDERER_REFERENCE_PROXY"), "/"),
		R2Bucket:               required("SITES_R2_BUCKET"),
		CloudflareZoneID:       required("SITES_CLOUDFLARE_ZONE_ID"),
		CloudflareAPIToken:     required("SITES_CLOUDFLARE_API_TOKEN"),
		PreviewSigningKey:      required("SITES_PREVIEW_SIGNING_KEY"),
		FormsChallengeKey:      required("SITES_FORMS_CHALLENGE_KEY"),
		CookieSecret:           required("SITES_COOKIE_SECRET"),
		PublicBaseURL:          strings.TrimRight(required("API_BASE_URL"), "/"),
		ApplicationDomains:     splitTrimmed(required("SITES_APPLICATION_DOMAINS")),
		CustomHostnameTarget:   normalizedDomain(strings.TrimSuffix(strings.ToLower(required("SITES_CUSTOM_HOSTNAME_TARGET")), ".")),
		APIPort:                required("PORT"),
	}
	return cfg, missing
}

func (c *SitesConfig) readTokens(read func(string) ([]byte, error)) error {
	for _, t := range []struct {
		name  string
		path  string
		value *string
	}{
		{"SITES_BUILDER_TOKEN_FILE", c.BuilderTokenFile, &c.BuilderToken},
		{"SITES_RENDERER_TOKEN_FILE", c.RendererTokenFile, &c.RendererToken},
	} {
		raw, err := read(t.path)
		if err != nil {
			return fmt.Errorf("%s points at %q, which cannot be read: %w", t.name, t.path, err)
		}
		*t.value = strings.TrimSpace(string(raw))
	}
	return nil
}

func (c SitesConfig) Validate() error {
	if _, err := emailaddress.NormalizeDomain(c.PublicDomain); err != nil {
		return fmt.Errorf("SITES_PUBLIC_DOMAIN %q is not a domain", c.PublicDomain)
	}
	for _, app := range c.ApplicationDomains {
		if _, err := emailaddress.NormalizeDomain(app); err != nil {
			return fmt.Errorf("SITES_APPLICATION_DOMAINS has %q, which is not a domain", app)
		}
	}
	if !absoluteURL(c.PublicBaseURL, true) {
		return fmt.Errorf("API_BASE_URL must be an absolute https url for page forms")
	}
	applicationHosts := append([]string{apiHost(c.PublicBaseURL)}, c.ApplicationDomains...)
	for _, app := range applicationHosts {
		if nestedDomains(c.PublicDomain, app) {
			return fmt.Errorf("customer pages must never share a domain with the application (%s)", app)
		}
	}
	if _, err := emailaddress.NormalizeDomain(c.CustomHostnameTarget); err != nil || !strings.HasSuffix(c.CustomHostnameTarget, "."+c.PublicDomain) {
		return fmt.Errorf("SITES_CUSTOM_HOSTNAME_TARGET %q must be a hostname on SITES_PUBLIC_DOMAIN", c.CustomHostnameTarget)
	}
	if !absoluteURL(c.BuilderURL, false) {
		return fmt.Errorf("SITES_BUILDER_URL must be an absolute url")
	}
	if !absoluteURL(c.RendererURL, false) {
		return fmt.Errorf("SITES_RENDERER_URL must be an absolute url")
	}
	if !proxyAddress(c.RendererReferenceProxy) {
		return fmt.Errorf("SITES_RENDERER_REFERENCE_PROXY must be an http proxy address like http://siterender-gateway:3129")
	}
	secrets := []namedSecret{
		{"SITES_BUILDER_TOKEN", c.BuilderToken},
		{"SITES_PREVIEW_SIGNING_KEY", c.PreviewSigningKey},
		{"SITES_FORMS_CHALLENGE_KEY", c.FormsChallengeKey},
		{"SITES_COOKIE_SECRET", c.CookieSecret},
	}
	for _, s := range secrets {
		if len(s.value) < minSitesSecretLength {
			return fmt.Errorf("%s must have at least %d characters", s.name, minSitesSecretLength)
		}
	}
	if !siterendergate.ValidRelayToken(c.RendererToken) {
		return fmt.Errorf("SITES_RENDERER_TOKEN must have at least %d characters from A-Z, a-z, 0-9, dot, underscore, tilde and hyphen", siterendergate.MinRelayTokenLength)
	}
	return distinctSecrets(append(secrets, namedSecret{"SITES_RENDERER_TOKEN", c.RendererToken}))
}

type namedSecret struct {
	name  string
	value string
}

func distinctSecrets(secrets []namedSecret) error {
	for i, a := range secrets {
		for _, b := range secrets[i+1:] {
			if a.value == b.value {
				return fmt.Errorf("%s and %s must be different secrets", a.name, b.name)
			}
		}
	}
	return nil
}

func apiHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func proxyAddress(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "http" && u.Host != "" && u.Port() != "" && (u.Path == "" || u.Path == "/") && u.RawQuery == "" && u.User == nil
}
