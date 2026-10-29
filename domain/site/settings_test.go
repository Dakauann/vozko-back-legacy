package site

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const fullSettings = `{
  "pages": {"/": {"title": "Clínica Sorriso", "description": "Implantes em SP", "og_media_id": "med_01", "canonical": "https://sorriso.example.com/"}, "/obrigado": {"noindex": true}},
  "integrations": {"meta_pixel": "123456789012345", "ga4": "G-AB12CD34", "gtm": "GTM-5XK2P9", "google_ads": "AW-123456789/AbC_dE", "tiktok_pixel": "C4ABCDEFGH1234567890", "clarity": "abc123xyz9"},
  "forms": {"contato": {"funnel_id": "fun_1", "stage_id": "stg_1", "owner": {"kind": "roulette", "id": "rou_1"}, "workflow_id": "wf_1", "follow_up": {"kind": "whatsapp", "id": "phone_1"}, "success": {"kind": "redirect", "route": "/obrigado"}, "notify": true}},
  "redirects": [{"from": "/promo", "to": "/", "status": 301}, {"from": "/blog", "to": "https://blog.example.com", "status": 302}],
  "consent": {"banner": true, "text": "Usamos cookies para medir visitas.", "categories": ["analytics", "marketing"]}
}`

func TestDecodeSettingsReadsTheWholeDocument(t *testing.T) {
	s, err := DecodeSettings([]byte(fullSettings))
	if err != nil {
		t.Fatal(err)
	}
	if s.Pages["/"].Title != "Clínica Sorriso" || !s.Pages["/obrigado"].NoIndex {
		t.Fatalf("got %+v", s.Pages)
	}
	enabled := s.Integrations.Enabled()
	if len(enabled) != 6 || enabled[0] != IntegrationMetaPixel || enabled[5] != IntegrationClarity {
		t.Fatalf("got %v", enabled)
	}
	if id, ok := s.Integrations.ID(IntegrationGA4); !ok || id != "G-AB12CD34" {
		t.Fatalf("got %q %v", id, ok)
	}
	form := s.Forms["contato"]
	if form.Owner.Kind != OwnerRoulette || form.Success.Kind != SuccessRedirect || form.Success.Route != "/obrigado" || !form.Notify {
		t.Fatalf("got %+v", form)
	}
	if len(s.Redirects) != 2 || s.Redirects[1].Status != 302 || !s.Consent.Banner {
		t.Fatalf("got %+v", s)
	}
	raw, _ := json.Marshal(s)
	again, err := DecodeSettings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if back, _ := json.Marshal(again); string(back) != string(raw) {
		t.Fatalf("round trip\n%s\n%s", back, raw)
	}
}

func TestEmptySettingsAreValid(t *testing.T) {
	s, err := DecodeSettings([]byte(`{}`))
	if err != nil || len(s.Integrations.Enabled()) != 0 {
		t.Fatalf("got %+v %v", s, err)
	}
}

func TestDecodeSettingsRefusesInvalidValues(t *testing.T) {
	cases := map[string]string{
		"unknown top level":   `{"theme":"dark"}`,
		"unknown integration": `{"integrations":{"hotjar":"1"}}`,
		"bad pixel":           `{"integrations":{"meta_pixel":"abc"}}`,
		"bad ga4":             `{"integrations":{"ga4":"UA-1234-1"}}`,
		"bad gtm":             `{"integrations":{"gtm":"gtm-1234"}}`,
		"bad google ads":      `{"integrations":{"google_ads":"AW-12"}}`,
		"bad tiktok":          `{"integrations":{"tiktok_pixel":"short"}}`,
		"bad clarity":         `{"integrations":{"clarity":"<script>"}}`,
		"bad seo route":       `{"pages":{"Contato":{"title":"x"}}}`,
		"long title":          `{"pages":{"/":{"title":"` + longText(71) + `"}}}`,
		"long description":    `{"pages":{"/":{"description":"` + longText(321) + `"}}}`,
		"bad canonical":       `{"pages":{"/":{"canonical":"javascript:alert(1)"}}}`,
		"bad form id":         `{"forms":{"Contato Principal":{}}}`,
		"bad owner kind":      `{"forms":{"a":{"owner":{"kind":"boss","id":"x"}}}}`,
		"owner without id":    `{"forms":{"a":{"owner":{"kind":"user"}}}}`,
		"unassigned with id":  `{"forms":{"a":{"owner":{"kind":"unassigned","id":"x"}}}}`,
		"bad success":         `{"forms":{"a":{"success":{"kind":"popup"}}}}`,
		"redirect no route":   `{"forms":{"a":{"success":{"kind":"redirect"}}}}`,
		"message no text":     `{"forms":{"a":{"success":{"kind":"message"}}}}`,
		"whatsapp no channel": `{"forms":{"a":{"success":{"kind":"whatsapp"}}}}`,
		"follow up kind":      `{"forms":{"a":{"follow_up":{"kind":"sms","id":"x"}}}}`,
		"follow up no id":     `{"forms":{"a":{"follow_up":{"kind":"email"}}}}`,
		"follow up bad id":    `{"forms":{"a":{"follow_up":{"kind":"unofficial_whatsapp","id":"a b"}}}}`,
		"redirect status":     `{"redirects":[{"from":"/a","to":"/","status":307}]}`,
		"redirect from home":  `{"redirects":[{"from":"/","to":"/a","status":301}]}`,
		"redirect to script":  `{"redirects":[{"from":"/a","to":"javascript:x","status":301}]}`,
		"redirect plain http": `{"redirects":[{"from":"/a","to":"http://x.example","status":301}]}`,
		"redirect repeated":   `{"redirects":[{"from":"/a","to":"/","status":301},{"from":"/a","to":"/b","status":301}]}`,
		"redirect to itself":  `{"redirects":[{"from":"/a","to":"/a","status":301}]}`,
		"unknown category":    `{"consent":{"banner":true,"categories":["ads"]}}`,
		"trailing data":       `{} {}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeSettings([]byte(raw)); !errors.Is(err, ErrSettingsInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestAFormWorkflowNeedsAFollowUpChannel(t *testing.T) {
	_, err := DecodeSettings([]byte(`{"forms":{"contato":{"workflow_id":"wf_1"}}}`))
	if !errors.Is(err, ErrSettingsInvalid) || !errors.Is(err, ErrFormChannelRequired) || ErrorCode(err) != CodeFormChannelRequired {
		t.Fatalf("got %v (%s)", err, ErrorCode(err))
	}
	for _, kind := range []FollowUpKind{FollowUpWhatsApp, FollowUpUnofficialWhatsApp, FollowUpEmail} {
		raw := `{"forms":{"contato":{"workflow_id":"wf_1","follow_up":{"kind":"` + string(kind) + `","id":"ch_1"}}}}`
		s, err := DecodeSettings([]byte(raw))
		if err != nil || s.Forms["contato"].FollowUp.Kind != kind || s.Forms["contato"].FollowUp.ID != "ch_1" {
			t.Fatalf("%s: got %+v %v", kind, s.Forms["contato"], err)
		}
	}
	if _, err := DecodeSettings([]byte(`{"forms":{"contato":{"follow_up":{"kind":"email","id":"inbox_1"}}}}`)); err != nil {
		t.Fatalf("a follow up channel without a workflow is allowed: %v", err)
	}
}

func TestAFormFunnelNeedsSomeoneToOwnTheDealOnlyWhenTheFormIsWritten(t *testing.T) {
	for _, raw := range []string{
		`{"forms":{"contato":{"funnel_id":"f_1"}}}`,
		`{"forms":{"contato":{"funnel_id":"f_1","owner":{"kind":"unassigned"}}}}`,
	} {
		legacy, err := DecodeSettings([]byte(raw))
		if err != nil {
			t.Fatalf("%s: stored settings must stay readable: %v", raw, err)
		}
		err = legacy.Forms["contato"].CheckDealOwner("contato")
		if !errors.Is(err, ErrSettingsInvalid) || !errors.Is(err, ErrFormDealOwnerRequired) || ErrorCode(err) != CodeFormDealOwnerRequired {
			t.Fatalf("%s: got %v (%s)", raw, err, ErrorCode(err))
		}
	}
	for _, raw := range []string{
		`{"forms":{"contato":{"funnel_id":"f_1","owner":{"kind":"user","id":"u_1"}}}}`,
		`{"forms":{"contato":{"funnel_id":"f_1","owner":{"kind":"roulette","id":"d_1"}}}}`,
		`{"forms":{"contato":{"funnel_id":"f_1","workflow_id":"wf_1","follow_up":{"kind":"whatsapp","id":"ch_1"}}}}`,
		`{"forms":{"contato":{"owner":{"kind":"unassigned"}}}}`,
	} {
		if _, err := DecodeSettings([]byte(raw)); err != nil {
			t.Fatalf("%s: got %v", raw, err)
		}
	}
}

func TestAFormKeepsAFriendlyName(t *testing.T) {
	s, err := DecodeSettings([]byte(`{"forms":{"contato":{"name":"Fale conosco"}}}`))
	if err != nil || s.Forms["contato"].Name != "Fale conosco" {
		t.Fatalf("got %+v %v", s.Forms, err)
	}
	long := strings.Repeat("a", MaxFormNameRunes+1)
	for _, name := range []string{" Fale", "Fale ", "Fale\nconosco", long, "   "} {
		raw, _ := json.Marshal(map[string]any{"forms": map[string]any{"contato": map[string]string{"name": name}}})
		if _, err := DecodeSettings(raw); !errors.Is(err, ErrSettingsInvalid) {
			t.Errorf("%q: got %v", name, err)
		}
	}
}
