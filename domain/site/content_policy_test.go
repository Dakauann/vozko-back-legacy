package site

import (
	"slices"
	"testing"
)

func TestAllowedFrameAcceptsOnlyTheEmbedList(t *testing.T) {
	cases := map[string]bool{
		"https://www.youtube-nocookie.com/embed/abc123":             true,
		"https://player.vimeo.com/video/76979871?h=8272103f6e":      true,
		"https://www.google.com/maps/embed?pb=!1m18":                true,
		"https://www.youtube.com/embed/abc123":                      false,
		"http://www.youtube-nocookie.com/embed/abc123":              false,
		"https://www.google.com/search?q=x":                         false,
		"https://www.google.com/maps/embedded":                      false,
		"https://player.vimeo.com.evil.example/video/1":             false,
		"https://user@player.vimeo.com/video/1":                     false,
		"//player.vimeo.com/video/1":                                false,
		"https://www.youtube-nocookie.com/embed/../../watch?v=1":    false,
		"https://evil.example/":                                     false,
		"":                                                          false,
		"javascript:alert(1)":                                       false,
		" https://www.youtube-nocookie.com/embed/abc123 ":           true,
		"https://WWW.YOUTUBE-NOCOOKIE.COM/embed/abc123":             true,
		"https://www.youtube-nocookie.com:8443/embed/abc123":        false,
		"https://www.youtube-nocookie.com/embed%2F..%2Fwatch?v=abc": false,
	}
	for src, want := range cases {
		if got := AllowedFrame(src); got != want {
			t.Errorf("%q: got %v", src, got)
		}
	}
}

func TestContentPolicyComesFromTheIntegrationsAndTheEmbeds(t *testing.T) {
	policy := ContentPolicyFor([]IntegrationKind{IntegrationGA4, IntegrationClarity}, "https://api.vozko.test", false)
	if !slices.Contains(policy.Script, "https://www.googletagmanager.com") || !slices.Contains(policy.Script, "https://www.clarity.ms") {
		t.Fatalf("script %v", policy.Script)
	}
	if !slices.Contains(policy.Connect, "https://*.clarity.ms") || !slices.Contains(policy.Img, "https://www.google-analytics.com") {
		t.Fatalf("policy %+v", policy)
	}
	if slices.Contains(policy.Script, "https://connect.facebook.net") {
		t.Fatal("only enabled integrations widen the policy")
	}
	if !slices.Equal(policy.Frame, EmbedFrames()) {
		t.Fatalf("frames %v", policy.Frame)
	}
	withChat := ContentPolicyFor(nil, "https://api.vozko.test", true)
	if !slices.Contains(withChat.Frame, "https://api.vozko.test/public/webchat/") || len(withChat.Script) != 0 {
		t.Fatalf("webchat frame %+v", withChat)
	}
	if err := withChat.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range IntegrationKinds() {
		hosts, ok := IntegrationOriginsOf(kind)
		if !ok || len(hosts.Script) == 0 {
			t.Fatalf("%s has no origins", kind)
		}
		if err := ContentPolicyFor([]IntegrationKind{kind}, "https://api.vozko.test", true).Validate(); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
}

func TestContentPolicyRefusesSourcesThatBreakTheHeader(t *testing.T) {
	for _, bad := range []string{"https://a.example; script-src *", "https://a.example 'unsafe-inline'", "http://a.example", "*", "https:", "https://a.example,b", "'self'", "https://a.example/\"x"} {
		if err := (ContentPolicy{Frame: []string{bad}}).Validate(); err == nil {
			t.Errorf("%q is accepted", bad)
		}
	}
}
