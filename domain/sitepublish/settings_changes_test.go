package sitepublish

import (
	"encoding/json"
	"reflect"
	"testing"

	"vozko/domain/site"
)

func publishedSettings() site.Settings {
	return site.Settings{
		Pages:        map[string]site.PageMeta{"/": {Title: "Bolos"}, "/sobre": {Title: "Sobre"}},
		Integrations: site.Integrations{GA4: "G-ABC1234", MetaPixel: "1234567890"},
		Forms: map[string]site.FormSettings{
			"contato": {FunnelID: "f1", StageID: "s1", Success: &site.Success{Kind: site.SuccessMessage, Message: "Obrigado"}},
			"antigo":  {FunnelID: "f1"},
		},
		Redirects: []site.Redirect{{From: "/velho", To: "/", Status: 301}, {From: "/promo", To: "https://loja.example.com/a", Status: 302}},
		Consent:   site.Consent{Banner: true, Text: "Usamos cookies"},
	}
}

func TestSettingsChangesIsEmptyWhenNothingChanged(t *testing.T) {
	got := DiffSettings(publishedSettings(), true, publishedSettings())
	if !got.Empty() {
		t.Fatalf("got %+v", got)
	}
	raw, err := json.Marshal(got)
	if err != nil || string(raw) != `{"redirects":[],"integrations":[],"forms":[],"pages":[]}` {
		t.Fatalf("an empty diff keeps the lists: %s %v", raw, err)
	}
}

func TestSettingsChangesListRedirectsByActionAndPath(t *testing.T) {
	current := publishedSettings()
	current.Redirects = []site.Redirect{
		{From: "/promo", To: "https://golpe.example.net/", Status: 302},
		{From: "/nova", To: "https://outra.example.com/", Status: 301},
	}
	got := DiffSettings(publishedSettings(), true, current).Redirects
	want := []RedirectChange{
		{Action: ChangeAdded, From: "/nova", To: "https://outra.example.com/", Status: 301},
		{Action: ChangeChanged, From: "/promo", To: "https://golpe.example.net/", Status: 302, PreviousTo: "https://loja.example.com/a", PreviousStatus: 302},
		{Action: ChangeRemoved, From: "/velho", PreviousTo: "/", PreviousStatus: 301},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestSettingsChangesListIntegrationsInCatalogOrder(t *testing.T) {
	current := publishedSettings()
	current.Integrations = site.Integrations{GA4: "G-ZZZ9999", Clarity: "abcdef12"}
	got := DiffSettings(publishedSettings(), true, current).Integrations
	want := []IntegrationChange{
		{Action: ChangeRemoved, Kind: site.IntegrationMetaPixel, PreviousID: "1234567890"},
		{Action: ChangeChanged, Kind: site.IntegrationGA4, ID: "G-ZZZ9999", PreviousID: "G-ABC1234"},
		{Action: ChangeAdded, Kind: site.IntegrationClarity, ID: "abcdef12"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestSettingsChangesNameTheChangedFormFields(t *testing.T) {
	current := publishedSettings()
	contato := current.Forms["contato"]
	contato.StageID = "s2"
	contato.Success = &site.Success{Kind: site.SuccessWhatsApp, Channel: "num-1"}
	contato.Notify = true
	current.Forms = map[string]site.FormSettings{"contato": contato, "novo": {Owner: &site.Owner{Kind: site.OwnerUnassigned}}}
	got := DiffSettings(publishedSettings(), true, current).Forms
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	if got[0].FormID != "antigo" || got[0].Action != ChangeRemoved || got[0].Before == nil || got[0].After != nil {
		t.Fatalf("removed form %+v", got[0])
	}
	if got[1].FormID != "contato" || got[1].Action != ChangeChanged || !reflect.DeepEqual(got[1].Fields, []string{FieldStage, FieldSuccess, FieldNotify}) {
		t.Fatalf("changed form %+v", got[1])
	}
	if got[1].After.Success.Channel != "num-1" || got[1].Before.Success.Kind != site.SuccessMessage {
		t.Fatalf("both sides are kept %+v", got[1])
	}
	if got[2].FormID != "novo" || got[2].Action != ChangeAdded || !reflect.DeepEqual(got[2].Fields, []string{FieldOwner}) {
		t.Fatalf("added form %+v", got[2])
	}
}

func TestSettingsChangesCoverConsentAndSEO(t *testing.T) {
	current := publishedSettings()
	current.Consent = site.Consent{Banner: false, Text: "Usamos cookies", Categories: []site.ConsentCategory{site.ConsentAnalytics}}
	current.Pages = map[string]site.PageMeta{"/": {Title: "Bolos da Ana", NoIndex: true}, "/contato": {Description: "Fale"}}
	current.Site = site.Identity{FaviconMediaID: "m1"}
	got := DiffSettings(publishedSettings(), true, current)
	if got.Consent == nil || !reflect.DeepEqual(got.Consent.Fields, []string{FieldBanner, FieldCategories}) || got.Consent.Before.Banner != true {
		t.Fatalf("consent %+v", got.Consent)
	}
	want := []PageChange{
		{Action: ChangeChanged, Route: "/", Fields: []string{FieldTitle, FieldNoIndex}, Before: &site.PageMeta{Title: "Bolos"}, After: &site.PageMeta{Title: "Bolos da Ana", NoIndex: true}},
		{Action: ChangeAdded, Route: "/contato", Fields: []string{FieldDescription}, After: &site.PageMeta{Description: "Fale"}},
		{Action: ChangeRemoved, Route: "/sobre", Fields: []string{FieldTitle}, Before: &site.PageMeta{Title: "Sobre"}},
	}
	if !reflect.DeepEqual(got.Pages, want) {
		t.Fatalf("pages %+v", got.Pages)
	}
	if got.Site == nil || !reflect.DeepEqual(got.Site.Fields, []string{FieldFavicon}) {
		t.Fatalf("site %+v", got.Site)
	}
}

func TestSettingsChangesWithoutAnActiveDeploymentCountEverythingAsAdded(t *testing.T) {
	got := DiffSettings(site.Settings{Redirects: []site.Redirect{{From: "/x", To: "/", Status: 301}}}, false, publishedSettings())
	if got.First != true || len(got.Redirects) != 2 || len(got.Integrations) != 2 || len(got.Forms) != 2 || len(got.Pages) != 2 || got.Consent == nil {
		t.Fatalf("got %+v", got)
	}
	for _, r := range got.Redirects {
		if r.Action != ChangeAdded {
			t.Fatalf("a first publish only adds: %+v", r)
		}
	}
	if !reflect.DeepEqual(got.Consent.Fields, []string{FieldBanner, FieldText}) {
		t.Fatalf("consent %+v", got.Consent)
	}
}

func TestSettingsChangesKnowWhereVisitorsAndDataGo(t *testing.T) {
	current := publishedSettings()
	current.Consent.Banner = false
	if DiffSettings(publishedSettings(), true, current).Routing() {
		t.Fatal("consent alone does not change where visitors or data go")
	}
	current.Redirects = nil
	if !DiffSettings(publishedSettings(), true, current).Routing() {
		t.Fatal("redirects change where visitors go")
	}
	current = publishedSettings()
	current.Integrations.Clarity = "abcdef12"
	if !DiffSettings(publishedSettings(), true, current).Routing() {
		t.Fatal("integrations change where data goes")
	}
}
