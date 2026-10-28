package webchat

import (
	"strings"
	"testing"
)

func TestAWidgetAlsoAcceptsThePublishedPagesThatEmbedIt(t *testing.T) {
	w := &Widget{Status: StatusActive, AllowedOrigins: []string{"https://*.loja.com.br"}}
	w.AcceptSiteOrigins([]string{"https://ofertas.vozko.site", "not an origin", "http://insecure.example"})
	for origin, want := range map[string]bool{
		"https://www.loja.com.br":        true,
		"https://ofertas.vozko.site":     true,
		"https://sub.ofertas.vozko.site": false,
		"http://insecure.example":        false,
		"https://evil.example":           false,
	} {
		if got := w.AllowsOrigin(origin); got != want {
			t.Errorf("AllowsOrigin(%q) = %v, want %v", origin, got, want)
		}
	}
	if !strings.Contains(w.FrameAncestors(), "https://ofertas.vozko.site") || !strings.Contains(w.FrameAncestors(), "https://*.loja.com.br") {
		t.Fatalf("frame ancestors = %q", w.FrameAncestors())
	}
	w.AcceptSiteOrigins(nil)
	if w.AllowsOrigin("https://ofertas.vozko.site") {
		t.Fatal("an unpublished page must stop being accepted")
	}
}

func TestSiteOriginsAreNeverSaved(t *testing.T) {
	w := &Widget{AllowedOrigins: []string{"https://loja.com.br"}}
	w.AcceptSiteOrigins([]string{"https://ofertas.vozko.site"})
	if len(w.AllowedOrigins) != 1 {
		t.Fatalf("allowed origins changed: %v", w.AllowedOrigins)
	}
}
