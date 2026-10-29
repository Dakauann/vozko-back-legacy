package site

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

const (
	WebchatFramePath  = "/public/webchat/"
	WebchatLoaderPath = "/public/webchat/loader.js"
)

var ErrContentPolicyInvalid = errors.New("site: a content policy source is invalid")

var policySource = regexp.MustCompile(`^https://(\*\.)?[a-z0-9.-]+(/[A-Za-z0-9._~/-]*)?$`)

type IntegrationOrigins struct {
	Loader  string
	Script  []string
	Connect []string
	Img     []string
}

var integrationOrigins = map[IntegrationKind]IntegrationOrigins{
	IntegrationMetaPixel: {
		Loader:  "https://connect.facebook.net/en_US/fbevents.js",
		Script:  []string{"https://connect.facebook.net"},
		Connect: []string{"https://www.facebook.com", "https://connect.facebook.net"},
		Img:     []string{"https://www.facebook.com"},
	},
	IntegrationGA4: {
		Loader:  "https://www.googletagmanager.com/gtag/js",
		Script:  []string{"https://www.googletagmanager.com"},
		Connect: []string{"https://www.google-analytics.com", "https://*.google-analytics.com", "https://*.analytics.google.com"},
		Img:     []string{"https://www.google-analytics.com", "https://www.googletagmanager.com"},
	},
	IntegrationGTM: {
		Loader:  "https://www.googletagmanager.com/gtm.js",
		Script:  []string{"https://www.googletagmanager.com"},
		Connect: []string{"https://www.googletagmanager.com"},
		Img:     []string{"https://www.googletagmanager.com"},
	},
	IntegrationGoogleAds: {
		Loader:  "https://www.googletagmanager.com/gtag/js",
		Script:  []string{"https://www.googletagmanager.com", "https://www.googleadservices.com"},
		Connect: []string{"https://www.google.com", "https://googleads.g.doubleclick.net"},
		Img:     []string{"https://www.google.com", "https://googleads.g.doubleclick.net"},
	},
	IntegrationTikTokPixel: {
		Loader:  "https://analytics.tiktok.com/i18n/pixel/events.js",
		Script:  []string{"https://analytics.tiktok.com"},
		Connect: []string{"https://analytics.tiktok.com"},
		Img:     []string{"https://analytics.tiktok.com"},
	},
	IntegrationClarity: {
		Loader:  "https://www.clarity.ms/tag/",
		Script:  []string{"https://www.clarity.ms"},
		Connect: []string{"https://*.clarity.ms", "https://c.bing.com"},
		Img:     []string{"https://*.clarity.ms", "https://c.bing.com"},
	},
}

func IntegrationLoader(kind IntegrationKind) (string, bool) {
	origins, ok := integrationOrigins[kind]
	return origins.Loader, ok && origins.Loader != ""
}

func IntegrationOriginsOf(kind IntegrationKind) (IntegrationOrigins, bool) {
	origins, ok := integrationOrigins[kind]
	return origins, ok
}

type embedFrame struct {
	host   string
	path   string
	prefix bool
}

var embedFrames = []embedFrame{
	{host: "www.youtube-nocookie.com", path: "/embed/", prefix: true},
	{host: "player.vimeo.com", path: "/video/", prefix: true},
	{host: "www.google.com", path: "/maps/embed"},
}

func (f embedFrame) source() string {
	return "https://" + f.host + f.path
}

func EmbedFrames() []string {
	out := make([]string, len(embedFrames))
	for i, f := range embedFrames {
		out[i] = f.source()
	}
	return out
}

func AllowedFrame(src string) bool {
	u, err := url.Parse(strings.TrimSpace(src))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawPath != "" || strings.Contains(u.Path, "..") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, f := range embedFrames {
		if host != f.host {
			continue
		}
		if (f.prefix && strings.HasPrefix(u.Path, f.path) && len(u.Path) > len(f.path)) || (!f.prefix && u.Path == f.path) {
			return true
		}
	}
	return false
}

type ContentPolicy struct {
	Script  []string `json:"script,omitempty"`
	Connect []string `json:"connect,omitempty"`
	Img     []string `json:"img,omitempty"`
	Frame   []string `json:"frame,omitempty"`
}

func appendUnique(list []string, values ...string) []string {
	for _, v := range values {
		if !slices.Contains(list, v) {
			list = append(list, v)
		}
	}
	return list
}

func ContentPolicyFor(integrations []IntegrationKind, apiOrigin string, webchat bool) ContentPolicy {
	var p ContentPolicy
	for _, kind := range integrations {
		origins, ok := integrationOrigins[kind]
		if !ok {
			continue
		}
		p.Script = appendUnique(p.Script, origins.Script...)
		p.Connect = appendUnique(p.Connect, origins.Connect...)
		p.Img = appendUnique(p.Img, origins.Img...)
	}
	p.Frame = EmbedFrames()
	if webchat {
		api := strings.TrimSuffix(apiOrigin, "/")
		p.Frame = appendUnique(p.Frame, api+WebchatFramePath)
		p.Script = appendUnique(p.Script, api+WebchatLoaderPath)
	}
	return p
}

func (p ContentPolicy) Validate() error {
	for _, list := range [][]string{p.Script, p.Connect, p.Img, p.Frame} {
		for _, source := range list {
			if !policySource.MatchString(source) {
				return fmt.Errorf("%w: %q", ErrContentPolicyInvalid, source)
			}
		}
	}
	return nil
}
