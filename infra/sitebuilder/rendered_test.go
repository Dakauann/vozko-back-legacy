package sitebuilder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vozko/domain/site"
	"vozko/domain/sitehtml"
)

func TestPublishedBuilderOutputPassesTheRenderedRules(t *testing.T) {
	files, err := filepath.Glob("../../services/sitebuilder/test/golden/*.html")
	if err != nil || len(files) == 0 {
		t.Fatalf("the builder goldens are missing: %v", err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := sitehtml.Parse(string(raw))
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		issues := site.CheckPages([]site.RenderedPage{{Route: "/", Document: doc}}).Issues
		if structural, _ := site.Split(issues); len(structural) > 0 {
			t.Fatalf("%s: %+v", file, structural)
		}
		if strings.Contains(string(raw), "data-vz-node") || strings.Contains(string(raw), "vz-revs") {
			t.Fatalf("%s carries preview annotations", file)
		}
	}
}
