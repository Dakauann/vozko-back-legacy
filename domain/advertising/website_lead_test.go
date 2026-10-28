package advertising

import (
	"testing"
	"time"
)

func TestAWebsiteLeadGoesToThePixelWithTheSubmissionAsEventID(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	lead := WebsiteLead{SubmissionID: "sub-1", At: now, Phone: "+5584999990000", Email: "Ana@Example.com", Fbp: "fb.1.1.2", Fbc: "fb.1.1.abc", UserAgent: "Mozilla/5.0", SourceURL: "https://loja.vozko.site/oferta"}
	settings := ConversionSettings{Enabled: true, SendLeads: true, PixelID: "123456789012", DatasetID: "999"}
	event, skip := WebsiteLeadFor(settings, lead, now)
	if skip != "" || event == nil {
		t.Fatalf("skip %q", skip)
	}
	if event.Name != EventNameWebsiteLead || event.EventID != "sub-1" || event.ActionSource != ActionSourceWebsite || event.Target != TargetPixel || event.TargetID != "123456789012" {
		t.Fatalf("event = %+v", event)
	}
	if event.PhoneHash != HashMatch(MatchPhone, lead.Phone, "") || event.EmailHash != HashMatch(MatchEmail, lead.Email, "") || event.EmailHash == "" {
		t.Fatalf("hashes = %q %q", event.PhoneHash, event.EmailHash)
	}
	if event.Fbp != lead.Fbp || event.Fbc != lead.Fbc || event.ClientUserAgent != lead.UserAgent || event.SourceURL != lead.SourceURL || !event.Time.Equal(now) {
		t.Fatalf("browser data = %+v", event)
	}
	datasetOnly, skip := WebsiteLeadFor(ConversionSettings{Enabled: true, SendLeads: true, DatasetID: "999"}, lead, now)
	if skip != "" || datasetOnly.Target != TargetDataset || datasetOnly.TargetID != "999" {
		t.Fatalf("dataset only = %+v %q", datasetOnly, skip)
	}
}

func TestAWebsiteLeadIsNeverSentWithoutADestinationOrConsentToSend(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	lead := WebsiteLead{SubmissionID: "sub-1", At: now, Email: "ana@example.com", UserAgent: "Mozilla/5.0"}
	ready := ConversionSettings{Enabled: true, SendLeads: true, PixelID: "123456789012"}
	cases := map[string]struct {
		settings ConversionSettings
		lead     WebsiteLead
		want     SkipReason
	}{
		"conversions off":     {ConversionSettings{SendLeads: true, PixelID: "1"}, lead, SkipNotEnabled},
		"leads off":           {ConversionSettings{Enabled: true, SendPurchases: true, PixelID: "1"}, lead, SkipEventOff},
		"no pixel or dataset": {ConversionSettings{Enabled: true, SendLeads: true}, lead, SkipNoDataset},
		"too old":             {ready, WebsiteLead{SubmissionID: "sub-1", At: now.Add(-8 * 24 * time.Hour), Email: "ana@example.com", UserAgent: "x"}, SkipTooOld},
		"nobody to match":     {ready, WebsiteLead{SubmissionID: "sub-1", At: now, UserAgent: "x"}, SkipNoAdIdentity},
		"no submission":       {ready, WebsiteLead{At: now, Email: "ana@example.com", UserAgent: "x"}, SkipNoAdIdentity},
		"no browser":          {ready, WebsiteLead{SubmissionID: "sub-1", At: now, Email: "ana@example.com"}, SkipNoAdIdentity},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			event, skip := WebsiteLeadFor(tc.settings, tc.lead, now)
			if event != nil || skip != tc.want {
				t.Fatalf("got %+v %q, want %q", event, skip, tc.want)
			}
		})
	}
}
