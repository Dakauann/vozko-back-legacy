package sitepublish

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validVisit() Visit {
	return Visit{
		ProjectID: "7f1c2a9e-4b3d-4e5f-8a6b-1c2d3e4f5a6b",
		Day:       time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC),
		Route:     "/",
		Variant:   ControlVariant,
		Event:     VisitView,
		Visitor:   strings.Repeat("a", VisitorHashLength),
	}
}

func TestParseVisitEventKnowsOnlyTheRuntimeEvents(t *testing.T) {
	for _, raw := range []string{"view", "cta", "form_view", "form_start", "form_submit", "whatsapp"} {
		if event, ok := ParseVisitEvent(raw); !ok || string(event) != raw {
			t.Fatalf("%s: got %q %v", raw, event, ok)
		}
	}
	for _, raw := range []string{"", "click", "VIEW"} {
		if _, ok := ParseVisitEvent(raw); ok {
			t.Fatalf("%q must be refused", raw)
		}
	}
}

func TestAVisitIsValidOnlyWithReadableFields(t *testing.T) {
	if err := validVisit().Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Visit){
		"no project":       func(v *Visit) { v.ProjectID = "" },
		"bad route":        func(v *Visit) { v.Route = "/Promo" },
		"bad variant":      func(v *Visit) { v.Variant = "B!" },
		"unknown event":    func(v *Visit) { v.Event = "click" },
		"path referrer":    func(v *Visit) { v.Referrer = "https://www.google.com/search" },
		"control utm":      func(v *Visit) { v.UTMSource = "a\x00b" },
		"short visitor":    func(v *Visit) { v.Visitor = "abc" },
		"no day":           func(v *Visit) { v.Day = time.Time{} },
		"route with a bar": func(v *Visit) { v.Route = "/a|b" },
	}
	for name, mutate := range cases {
		v := validVisit()
		mutate(&v)
		if err := v.Validate(); !errors.Is(err, ErrVisitInvalid) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
	v := validVisit()
	v.Referrer, v.UTMSource = "https://www.instagram.com", "instagram"
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestVisitorHashChangesWithTheDayAndStaysPrivate(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	day := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	first := VisitorHash(secret, day, "203.0.113.9", "Mozilla/5.0", "site-1")
	if len(first) != VisitorHashLength || strings.Contains(first, "203.0.113.9") {
		t.Fatalf("got %q", first)
	}
	if again := VisitorHash(secret, day, "203.0.113.9", "Mozilla/5.0", "site-1"); again != first {
		t.Fatal("the same visitor on the same day hashes the same")
	}
	if next := VisitorHash(secret, day.AddDate(0, 0, 1), "203.0.113.9", "Mozilla/5.0", "site-1"); next == first {
		t.Fatal("the hash rotates every day")
	}
	if other := VisitorHash(secret, day, "203.0.113.9", "Mozilla/5.0", "site-2"); other == first {
		t.Fatal("the hash differs per page")
	}
}

func TestBotsAreRecognisedByTheirUserAgent(t *testing.T) {
	for _, ua := range []string{"Googlebot/2.1", "facebookexternalhit/1.1", "curl/8.4.0", "Mozilla/5.0 HeadlessChrome/120", "Go-http-client/2.0", ""} {
		if !IsBot(ua) {
			t.Fatalf("%q is a bot", ua)
		}
	}
	if IsBot("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Safari/604.1") {
		t.Fatal("a phone browser is a person")
	}
}

func TestReferrerOriginKeepsOnlyWebOrigins(t *testing.T) {
	cases := map[string]string{
		"https://www.google.com/search?q=x": "https://www.google.com",
		"http://localhost:8080/a":           "http://localhost:8080",
		"android-app://com.google":          "",
		"not a url":                         "",
		"":                                  "",
	}
	for raw, want := range cases {
		if got := ReferrerOrigin(raw); got != want {
			t.Fatalf("%q: got %q want %q", raw, got, want)
		}
	}
}

func TestDailyStatCountsEachEventInItsColumn(t *testing.T) {
	var s DailyStat
	for _, e := range []VisitEvent{VisitView, VisitView, VisitCTA, VisitFormView, VisitFormStart, VisitFormSubmit, VisitWhatsApp} {
		s.Count(e, 1)
	}
	if s.Views != 2 || s.CTA != 1 || s.FormViews != 1 || s.FormStarts != 1 || s.FormSubmits != 1 || s.WhatsApp != 1 {
		t.Fatalf("got %+v", s)
	}
}

func TestUTMValueIsCleanedAndCut(t *testing.T) {
	if got := UTMValue("  insta\x00gram "); got != "instagram" {
		t.Fatalf("got %q", got)
	}
	if got := UTMValue(strings.Repeat("a", 300)); len(got) != maxUTMLength {
		t.Fatalf("got %d", len(got))
	}
	if got := UTMValue("\x01 "); got != "" {
		t.Fatalf("got %q", got)
	}
}
