package sitetools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/usecases/copilot/copilottools/tooltest"
)

func TestThePublishCardListsTheSettingsThatChange(t *testing.T) {
	r := newRig(t)
	published := site.Settings{
		Redirects:    []site.Redirect{{From: "/promo", To: "https://loja.example.com/", Status: 302}, {From: "/velho", To: "/", Status: 301}},
		Integrations: site.Integrations{MetaPixel: "1234567890"},
		Forms:        map[string]site.FormSettings{"contato": {Success: &site.Success{Kind: site.SuccessMessage, Message: "Oi"}}},
	}
	current := site.Settings{
		Redirects:    []site.Redirect{{From: "/promo", To: "https://golpe.example.net/", Status: 302}, {From: "/nova", To: "/", Status: 301}},
		Integrations: site.Integrations{GA4: "G-ABC1234"},
		Forms:        map[string]site.FormSettings{"contato": {Success: &site.Success{Kind: site.SuccessWhatsApp, Channel: "num-1"}}},
		Consent:      site.Consent{Banner: true},
		Pages:        map[string]site.PageMeta{"/": {Title: "Novo"}},
	}
	r.publisher.settings = sitepublish.DiffSettings(published, true, current)
	fields := tooltest.FieldMap(NewSitePublishTool(r.deps).(copilot.Describer).Describe(context.Background(), r.cc, map[string]interface{}{}))
	got := fields["settings"]
	for _, want := range []string{
		"redirecionamento novo /nova para /",
		"redirecionamento /promo agora vai para https://golpe.example.net/ (antes https://loja.example.com/)",
		"redirecionamento /velho removido",
		"integração ga4 nova: G-ABC1234",
		"integração meta_pixel removida",
		"formulário contato muda success",
		"aviso de cookies muda banner",
		"SEO de / configurado: title",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Index(got, "redirecionamento") > strings.Index(got, "integração") || strings.Index(got, "integração") > strings.Index(got, "formulário") {
		t.Fatalf("where visitors and data go comes first: %q", got)
	}
}

func TestThePublishCardSaysWhenSettingsDidNotChangeOrCannotBeRead(t *testing.T) {
	r := newRig(t)
	fields := tooltest.FieldMap(NewSitePublishTool(r.deps).(copilot.Describer).Describe(context.Background(), r.cc, map[string]interface{}{}))
	if fields["settings"] != publishSettingsSame {
		t.Fatalf("got %+v", fields)
	}
	r.publisher.settingsErr = errors.New("down")
	fields = tooltest.FieldMap(NewSitePublishTool(r.deps).(copilot.Describer).Describe(context.Background(), r.cc, map[string]interface{}{}))
	if fields["settings"] != publishSettingsUnknown {
		t.Fatalf("got %+v", fields)
	}
}
