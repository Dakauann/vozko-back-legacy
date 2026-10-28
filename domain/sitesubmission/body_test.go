package sitesubmission

import (
	"errors"
	"strings"
	"testing"

	"vozko/domain/siteform"
)

const runtimeBody = `{"utm":{"source":"meta","medium":"cpc","campaign":"natal","term":"","content":""},"referrer":"https://l.facebook.com/l.php?u=x","fbclid":"abc","gclid":"","fbp":"fb.1.1.2","fbc":"fb.1.1.abc","variant":"b","idempotencyKey":"0f8e4b1a-1c2d-4e5f-8a9b-0c1d2e3f4a5b","website":"","route":"/oferta","fields":{"lead.name":"Ana","lead.phone":"+55 84 99999-0000","field.plano":["a","b"]},"challengeToken":"id.scope.16.1700000000.sig","nonce":"123"}`

func TestTheRuntimeBodyDecodesWithEveryField(t *testing.T) {
	b, err := DecodeBody([]byte(runtimeBody))
	if err != nil {
		t.Fatal(err)
	}
	if b.UTM.Source != "meta" || b.UTM.Campaign != "natal" || b.Fbclid != "abc" || b.Fbp != "fb.1.1.2" || b.Fbc != "fb.1.1.abc" ||
		b.Variant != "b" || b.Route != "/oferta" || b.ChallengeToken == "" || b.Nonce != "123" || b.IdempotencyKey == "" {
		t.Fatalf("body = %+v", b)
	}
	values, problems := b.Values()
	if values["lead.name"] != "Ana" || values["lead.phone"] != "+55 84 99999-0000" {
		t.Fatalf("values = %v", values)
	}
	if len(problems) != 1 || problems[0].Field != "field.plano" || problems[0].Code != siteform.CodeValueInvalid {
		t.Fatalf("problems = %+v", problems)
	}
	if got := ReferrerOrigin(b.Referrer); got != "https://l.facebook.com" {
		t.Fatalf("referrer origin = %q", got)
	}
}

func TestDecodeBodyRefusesWhatTheRuntimeNeverSends(t *testing.T) {
	cases := map[string]string{
		"empty":                 ``,
		"not an object":         `[]`,
		"unknown field":         strings.Replace(runtimeBody, `"nonce":"123"`, `"nonce":"123","extra":1`, 1),
		"unknown utm key":       strings.Replace(runtimeBody, `"term":""`, `"term":"","id":"x"`, 1),
		"no idempotency key":    strings.Replace(runtimeBody, `"0f8e4b1a-1c2d-4e5f-8a9b-0c1d2e3f4a5b"`, `""`, 1),
		"bad idempotency key":   strings.Replace(runtimeBody, `"0f8e4b1a-1c2d-4e5f-8a9b-0c1d2e3f4a5b"`, `"a b"`, 1),
		"no challenge":          strings.Replace(runtimeBody, `"id.scope.16.1700000000.sig"`, `""`, 1),
		"route not a path":      strings.Replace(runtimeBody, `"/oferta"`, `"https://x.com"`, 1),
		"huge utm":              strings.Replace(runtimeBody, `"natal"`, `"`+strings.Repeat("n", 300)+`"`, 1),
		"field value is number": strings.Replace(runtimeBody, `"Ana"`, `12`, 1),
		"trailing data":         runtimeBody + `{}`,
		"no fields":             strings.Replace(runtimeBody, `"fields":{"lead.name":"Ana","lead.phone":"+55 84 99999-0000","field.plano":["a","b"]},`, ``, 1),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeBody([]byte(raw)); !errors.Is(err, ErrBodyInvalid) {
				t.Fatalf("DecodeBody = %v, want ErrBodyInvalid", err)
			}
		})
	}
}

func TestReferrerOriginKeepsOnlyTheOrigin(t *testing.T) {
	cases := map[string]string{
		"https://www.google.com/search?q=x": "https://www.google.com",
		"http://example.com:8080/a":         "http://example.com:8080",
		"":                                  "",
		"android-app://com.google":          "",
		"not a url":                         "",
		"https://user:pw@evil.com/":         "",
	}
	for raw, want := range cases {
		if got := ReferrerOrigin(raw); got != want {
			t.Errorf("ReferrerOrigin(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestDetailsNameTheFieldsThePersonMustFix(t *testing.T) {
	manifest := siteform.Manifest{ID: "contato", Fields: []siteform.Field{{Name: "lead.phone", Kind: siteform.KindTel}, {Name: "lead.email", Kind: siteform.KindEmail}}}
	got := Details([]siteform.Problem{{Field: "lead.name", Code: "form_field_required"}, {Code: siteform.CodeIdentityMissing}}, manifest)
	want := map[string]string{"lead.name": "form_field_required", "lead.phone": siteform.CodeIdentityMissing, "lead.email": siteform.CodeIdentityMissing}
	if len(got) != len(want) {
		t.Fatalf("details = %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("details = %v, want %v", got, want)
		}
	}
}
