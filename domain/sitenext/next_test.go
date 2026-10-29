package sitenext

import (
	"slices"
	"strings"
	"testing"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

func codesOf(steps []Step) []string {
	out := make([]string, 0, len(steps))
	for _, step := range steps {
		out = append(out, step.Code)
	}
	return out
}

func finished() Facts {
	return Facts{
		Forms:        []string{"contato"},
		Settings:     site.Settings{Forms: map[string]site.FormSettings{"contato": {FunnelID: "f-1"}}, Site: site.Identity{FaviconMediaID: "m-1", WebchatWidget: "w-1"}},
		Stage:        StageLive,
		CustomDomain: true,
	}
}

func TestNextSuggestsNothingForAFinishedSite(t *testing.T) {
	if got := Next(finished()); len(got) != 0 {
		t.Fatalf("got %v", codesOf(got))
	}
}

func TestNextKeepsAtMostThreeStepsInPriorityOrder(t *testing.T) {
	f := Facts{
		Gaps:   []site.Issue{{Code: site.CodeLinkEmpty}, {Code: site.CodeFormDealOwnerRequired}, {Code: site.CodeLinkEmpty}, {Code: site.CodeAltMissing}},
		Forms:  []string{"contato"},
		Source: Source{MentionsBooking: true},
		Stage:  StageDraft,
	}
	got := codesOf(Next(f))
	want := []string{CodeFormOwner, CodeEmptyLinks, CodeImageAlt}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNextRules(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Facts)
		want   []string
	}{
		{"form without settings", func(f *Facts) { f.Settings.Forms = nil }, []string{CodeFormDestination}},
		{"form without funnel nor workflow", func(f *Facts) { f.Settings.Forms = map[string]site.FormSettings{"contato": {Notify: true}} }, []string{CodeFormDestination}},
		{"form run by a workflow", func(f *Facts) { f.Settings.Forms = map[string]site.FormSettings{"contato": {WorkflowID: "wf-1"}} }, nil},
		{"booking mentioned without a block", func(f *Facts) { f.Source.MentionsBooking = true }, []string{CodeAddBooking}},
		{"booking mentioned with a block in the page", func(f *Facts) { f.Source = Source{MentionsBooking: true, HasBooking: true} }, nil},
		{"booking mentioned with a booking in settings", func(f *Facts) {
			f.Source.MentionsBooking = true
			f.Settings.Bookings = map[string]site.Booking{"agenda": {}}
		}, nil},
		{"no site chat", func(f *Facts) { f.Settings.Site.WebchatWidget = "" }, []string{CodeSiteChat}},
		{"chat placed in the page", func(f *Facts) {
			f.Settings.Site.WebchatWidget = ""
			f.Source.HasWebchat = true
		}, nil},
		{"no favicon", func(f *Facts) { f.Settings.Site.FaviconMediaID = "" }, []string{CodeFavicon}},
		{"never published", func(f *Facts) { f.Stage = StageDraft }, []string{CodePublish}},
		{"live with changes", func(f *Facts) { f.Stage = StageLiveWithChanges }, []string{CodePublishChanges}},
		{"live without a custom domain", func(f *Facts) { f.CustomDomain = false }, []string{CodeCustomDomain}},
		{"draft without a custom domain asks to publish first", func(f *Facts) {
			f.Stage = StageDraft
			f.CustomDomain = false
		}, []string{CodePublish}},
		{"closed page suggests no publishing", func(f *Facts) {
			f.Stage = StageClosed
			f.CustomDomain = false
		}, nil},
		{"open gaps hold the publish step back", func(f *Facts) {
			f.Stage = StageDraft
			f.Gaps = []site.Issue{{Code: site.CodeConsentBannerMissing}}
		}, []string{CodeConsentBanner}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := finished()
			tc.change(&f)
			if got := codesOf(Next(f)); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestEveryPublishGapHasAStep(t *testing.T) {
	for _, code := range site.GapCodes() {
		f := finished()
		f.Gaps = []site.Issue{{Code: code}}
		got := Next(f)
		if len(got) != 1 {
			t.Fatalf("gap %s gave %v", code, codesOf(got))
		}
	}
}

func TestStepsCarryALabelAndOneUsableAction(t *testing.T) {
	for _, step := range Catalog() {
		if strings.TrimSpace(step.Label) == "" || strings.ContainsAny(step.Label+step.Action.Prompt, "–—") {
			t.Fatalf("%s label %q", step.Code, step.Label)
		}
		switch step.Action.Kind {
		case ActionPrompt:
			if strings.TrimSpace(step.Action.Prompt) == "" || step.Action.Tab != "" {
				t.Fatalf("%s prompt action %+v", step.Code, step.Action)
			}
		case ActionSettings:
			if step.Action.Tab == "" || step.Action.Prompt != "" {
				t.Fatalf("%s settings action %+v", step.Code, step.Action)
			}
		case ActionPublish:
			if step.Action.Tab != "" || step.Action.Prompt != "" {
				t.Fatalf("%s publish action %+v", step.Code, step.Action)
			}
		default:
			t.Fatalf("%s action kind %q", step.Code, step.Action.Kind)
		}
	}
}

func TestCatalogCodesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, step := range Catalog() {
		if seen[step.Code] {
			t.Fatalf("duplicate %s", step.Code)
		}
		seen[step.Code] = true
	}
}

func TestScanSourceReadsBookingWordsAndPlatformElements(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    Source
	}{
		{"plain page", "<h1>Clínica Sorriso</h1>", Source{}},
		{"booking word", "<a>Agende sua consulta</a>", Source{MentionsBooking: true}},
		{"booking noun", "<p>Faça seu agendamento online</p>", Source{MentionsBooking: true}},
		{"word inside another word is ignored", "<p>reagendado</p>", Source{}},
		{"booking element", `<vz-booking booking="consultas"></vz-booking>`, Source{HasBooking: true}},
		{"chat element", `<vz-webchat widget="abc"></vz-webchat>`, Source{HasWebchat: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := site.Project{Stack: site.StackHTML, Files: map[string]site.File{"pages/index.html": {Content: tc.content}}}
			if got := ScanSource(p); got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestHasCustomDomainCountsOnlyCustomHostnamesThatAreNotLeaving(t *testing.T) {
	cases := []struct {
		name string
		recs []sitepublish.HostnameRecord
		want bool
	}{
		{"none", nil, false},
		{"platform only", []sitepublish.HostnameRecord{{Kind: sitepublish.HostnamePlatform, Status: sitepublish.HostnameActive}}, false},
		{"custom pending", []sitepublish.HostnameRecord{{Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnamePendingDNS}}, true},
		{"custom active", []sitepublish.HostnameRecord{{Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnameActive}}, true},
		{"custom removing", []sitepublish.HostnameRecord{{Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnameRemoving}}, false},
		{"custom failed", []sitepublish.HostnameRecord{{Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnameFailed}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasCustomDomain(tc.recs); got != tc.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}
