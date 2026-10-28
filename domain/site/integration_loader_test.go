package site

import (
	"net/url"
	"slices"
	"testing"
)

func TestEveryIntegrationLoadsFromOneExactURLItsPolicyAllows(t *testing.T) {
	for _, kind := range IntegrationKinds() {
		loader, ok := IntegrationLoader(kind)
		if !ok {
			t.Fatalf("%s has no loader", kind)
		}
		u, err := url.Parse(loader)
		if err != nil || u.Scheme != "https" || u.RawQuery != "" || u.Fragment != "" {
			t.Fatalf("%s loader %q", kind, loader)
		}
		origins, _ := IntegrationOriginsOf(kind)
		if !slices.Contains(origins.Script, "https://"+u.Host) {
			t.Fatalf("%s loads from %s outside its script policy %v", kind, u.Host, origins.Script)
		}
	}
	if _, ok := IntegrationLoader("unknown"); ok {
		t.Fatal("an unknown kind has no loader")
	}
}
