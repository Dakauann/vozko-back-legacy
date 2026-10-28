package sitepublish

import (
	"strings"
	"testing"
	"time"
)

func TestValidSourceKeepsReadableUTMValuesAndPlainOrigins(t *testing.T) {
	cases := []struct {
		kind  SourceKind
		value string
		ok    bool
	}{
		{SourceUTM, "instagram", true},
		{SourceUTM, "Black Friday 2026", true},
		{SourceUTM, "", false},
		{SourceUTM, "a\x00b", false},
		{SourceUTM, strings.Repeat("a", MaxSourceValueLength+1), false},
		{SourceReferrer, "https://www.google.com", true},
		{SourceReferrer, "http://localhost:8080", true},
		{SourceReferrer, "https://www.google.com/search", false},
		{SourceReferrer, "android-app://com.google", false},
		{SourceReferrer, "https://user@evil.test", false},
		{SourceReferrer, "", false},
		{SourceKind("device"), "mobile", false},
	}
	for _, c := range cases {
		if got := ValidSource(c.kind, c.value); got != c.ok {
			t.Fatalf("%s %q: got %v", c.kind, c.value, got)
		}
	}
}

func TestRankSourcesSumsDaysAndOrdersByViews(t *testing.T) {
	day := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	stats := []SourceStat{
		{Day: day, Kind: SourceUTM, Value: "instagram", Views: 10, Submits: 1},
		{Day: day.AddDate(0, 0, 1), Kind: SourceUTM, Value: "instagram", Views: 5, Submits: 2},
		{Day: day, Kind: SourceUTM, Value: "google", Views: 15, Submits: 0},
		{Day: day, Kind: SourceUTM, Value: "email", Views: 2, Submits: 4},
		{Day: day, Kind: SourceReferrer, Value: "https://www.google.com", Views: 99},
		{Day: day, Kind: SourceReferrer, Value: "https://loja.paginas.test", Views: 200},
	}
	utm := RankSources(stats, SourceUTM, nil, 2)
	if len(utm) != 2 || utm[0] != (SourceTotal{Value: "instagram", Views: 15, Submits: 3}) || utm[1] != (SourceTotal{Value: "google", Views: 15}) {
		t.Fatalf("ties on views go by submits then by name, and the list is capped: %+v", utm)
	}
	own := func(origin string) bool { return origin == "https://loja.paginas.test" }
	referrers := RankSources(stats, SourceReferrer, own, 10)
	if len(referrers) != 1 || referrers[0].Value != "https://www.google.com" {
		t.Fatalf("the page's own origin is navigation, not a source: %+v", referrers)
	}
	if empty := RankSources(nil, SourceUTM, nil, 10); empty == nil || len(empty) != 0 {
		t.Fatalf("an empty ranking is an empty list: %#v", empty)
	}
}
