package site

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSiteIdentityDecodesFaviconAndStructuredData(t *testing.T) {
	s, err := DecodeSettings([]byte(`{"site":{"favicon_media_id":"med_ico","structured_data":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Site.FaviconMediaID != "med_ico" || !s.Site.StructuredData {
		t.Fatalf("got %+v", s.Site)
	}
	raw, _ := json.Marshal(Settings{})
	if strings.Contains(string(raw), `"site"`) {
		t.Fatalf("an empty identity stays out of the document: %s", raw)
	}
}

func TestSiteIdentityRefusesAnythingButAMediaIDAndABoolean(t *testing.T) {
	cases := map[string]string{
		"free json-ld":       `{"site":{"structured_data":{"@type":"Organization"}}}`,
		"unknown field":      `{"site":{"json_ld":"{}"}}`,
		"favicon url":        `{"site":{"favicon_media_id":"https://evil.example/x.ico"}}`,
		"favicon with space": `{"site":{"favicon_media_id":"med 1"}}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeSettings([]byte(raw)); !errors.Is(err, ErrSettingsInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestOrganizationDataIsBuiltFromTrustedValuesOnly(t *testing.T) {
	raw, err := OrganizationData("Clínica Sorriso", "https://sorriso.example.com", "https://cdn.example.com/logo.png")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"@context": "https://schema.org", "@type": "Organization", "name": "Clínica Sorriso", "url": "https://sorriso.example.com", "logo": "https://cdn.example.com/logo.png"}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %q, want %q (%s)", k, got[k], v, raw)
		}
	}
	bare, err := OrganizationData("Loja", "", "")
	if err != nil || strings.Contains(string(bare), `"url"`) || strings.Contains(string(bare), `"logo"`) {
		t.Fatalf("optional values stay out: %s %v", bare, err)
	}
	for name, args := range map[string][3]string{
		"no name":       {"  ", "", ""},
		"http url":      {"Loja", "http://loja.example.com", ""},
		"script logo":   {"Loja", "", "javascript:alert(1)"},
		"long name":     {strings.Repeat("a", 121), "", ""},
		"url with path": {"Loja", "https://loja.example.com/x", ""},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := OrganizationData(args[0], args[1], args[2]); !errors.Is(err, ErrOrganizationInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
