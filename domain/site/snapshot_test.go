package site

import (
	"testing"

	"vozko/domain/sitehtml"
)

func snapshotFiles() map[string]File {
	return map[string]File{
		ManifestPath:           {Content: "{}", Rev: 9},
		ThemePath:              {Content: "", Rev: 4},
		"pages/index.html":     {Content: "<h1>a</h1>", Rev: 3},
		"pages/obrigado.html":  {Content: "<h1>b</h1>", Rev: 1},
		"partials/header.html": {Content: "<nav></nav>", Rev: 2},
		"scripts/analytics.js": {Content: "", Rev: 5},
	}
}

func parsed(t *testing.T, head string) *sitehtml.Document {
	t.Helper()
	doc, err := sitehtml.Parse("<!doctype html><html><head>" + head + "</head><body><h1>x</h1></body></html>")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestPreviewRevisionsListOnlyTheMarkupTheBuildStamps(t *testing.T) {
	got := PreviewRevisions(StackHTML, snapshotFiles())
	want := map[string]uint64{"pages/index.html": 3, "pages/obrigado.html": 1, "partials/header.html": 2}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for path, rev := range want {
		if got[path] != rev {
			t.Fatalf("got %v", got)
		}
	}
}

func TestASnapshotBelongsToTheRevisionWhoseMarkupItWasBuiltFrom(t *testing.T) {
	current := `<meta name="vz-revs" content='{"pages/index.html":3,"pages/obrigado.html":1,"partials/header.html":2}'>`
	serialized := `<meta name="vz-revs" content="{&quot;partials/header.html&quot;:2,&quot;pages/index.html&quot;:3,&quot;pages/obrigado.html&quot;:1}">`
	for _, head := range []string{current, serialized} {
		if !BuiltFrom(parsed(t, head), StackHTML, snapshotFiles()) {
			t.Fatalf("the current stamp matches: %s", head)
		}
	}
	cases := map[string]string{
		"no stamp":       ``,
		"older rev":      `<meta name="vz-revs" content='{"pages/index.html":2,"pages/obrigado.html":1,"partials/header.html":2}'>`,
		"missing file":   `<meta name="vz-revs" content='{"pages/index.html":3,"partials/header.html":2}'>`,
		"deleted file":   `<meta name="vz-revs" content='{"pages/index.html":3,"pages/obrigado.html":1,"partials/header.html":2,"pages/velha.html":1}'>`,
		"not json":       `<meta name="vz-revs" content='{"pages/index.html":'>`,
		"two stamps":     current + current,
		"negative rev":   `<meta name="vz-revs" content='{"pages/index.html":-3,"pages/obrigado.html":1,"partials/header.html":2}'>`,
		"trailing value": `<meta name="vz-revs" content='{"pages/index.html":3,"pages/obrigado.html":1,"partials/header.html":2} 1'>`,
	}
	for name, head := range cases {
		t.Run(name, func(t *testing.T) {
			if BuiltFrom(parsed(t, head), StackHTML, snapshotFiles()) {
				t.Fatal("a stale or forged stamp is refused")
			}
		})
	}
}

func TestMarkRankOrdersInteractiveThenContentThenLayout(t *testing.T) {
	ranks := MarkRanks()
	rank := func(tag string) int {
		for i, group := range ranks {
			for _, member := range group {
				if member == tag {
					return i
				}
			}
		}
		return len(ranks)
	}
	if !(rank("button") < rank("h1") && rank("h1") < rank("p") && rank("p") < rank("section") && rank("section") < rank("div")) {
		t.Fatalf("ranks %v", ranks)
	}
	if MaxMarks != 20 || MinMarkWidth != 16 || MinMarkHeight != 12 {
		t.Fatal("the mark limits are the ones the look promises")
	}
}
