package sitehttp

import (
	"encoding/json"
	"net/http"
	"testing"

	workspace_domain "vozko/domain/workspace"
)

func TestSectionsListsTheLibraryWithTitlesInFourLanguages(t *testing.T) {
	r, grants := router(newFake())
	rec := call(r, http.MethodGet, "/sites/sections", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if got := grants["GET /sites/sections"]; got != (grant{workspace_domain.ResourceSites, workspace_domain.ActionRead}) {
		t.Fatalf("guarded by %+v", got)
	}
	var body SectionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sections) != 12 || body.Sections[0].Key != "hero" || body.Sections[0].Title.PT == "" || body.Sections[0].Title.DE == "" {
		t.Fatalf("got %+v", body)
	}
	if stacks := body.Sections[0].Stacks; len(stacks) != 2 || stacks[0] != "html" || stacks[1] != "react" {
		t.Fatalf("stacks %v", stacks)
	}
	var raw map[string][]map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	title, _ := raw["sections"][0]["title"].(map[string]any)
	for _, locale := range []string{"pt", "en", "es", "de"} {
		if title[locale] == "" || title[locale] == nil {
			t.Fatalf("title.%s missing in %s", locale, rec.Body.String())
		}
	}
}
