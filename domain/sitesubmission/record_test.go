package sitesubmission

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"vozko/domain/site"
	"vozko/domain/siteform"
)

func phoneForm() siteform.Manifest {
	return siteform.Manifest{ID: "contato", Fields: []siteform.Field{{Name: "lead.phone", Kind: siteform.KindTel}}}
}

func emailForm() siteform.Manifest {
	return siteform.Manifest{ID: "contato", Fields: []siteform.Field{{Name: "lead.email", Kind: siteform.KindEmail}}}
}

func TestTheFollowUpChannelMustReachTheIdentityTheFormAsks(t *testing.T) {
	whatsapp := &site.FollowUp{Kind: site.FollowUpWhatsApp, ID: "n1"}
	unofficial := &site.FollowUp{Kind: site.FollowUpUnofficialWhatsApp, ID: "i1"}
	email := &site.FollowUp{Kind: site.FollowUpEmail, ID: "s1"}
	cases := map[string]struct {
		manifest siteform.Manifest
		settings site.FormSettings
		want     error
	}{
		"no workflow needs no channel":        {phoneForm(), site.FormSettings{}, nil},
		"phone form on whatsapp":              {phoneForm(), site.FormSettings{WorkflowID: "wf", FollowUp: whatsapp}, nil},
		"phone form on unofficial whatsapp":   {phoneForm(), site.FormSettings{WorkflowID: "wf", FollowUp: unofficial}, nil},
		"email form on an inbox":              {emailForm(), site.FormSettings{WorkflowID: "wf", FollowUp: email}, nil},
		"workflow without channel":            {phoneForm(), site.FormSettings{WorkflowID: "wf"}, ErrFormChannelRequired},
		"email only form on whatsapp":         {emailForm(), site.FormSettings{WorkflowID: "wf", FollowUp: whatsapp}, ErrFormChannelRequired},
		"phone only form on an inbox":         {phoneForm(), site.FormSettings{WorkflowID: "wf", FollowUp: email}, ErrFormChannelRequired},
		"form without identity and workflow":  {siteform.Manifest{ID: "x"}, site.FormSettings{WorkflowID: "wf", FollowUp: whatsapp}, ErrFormChannelRequired},
		"unknown channel kind":                {phoneForm(), site.FormSettings{WorkflowID: "wf", FollowUp: &site.FollowUp{Kind: "sms", ID: "x"}}, ErrFormChannelRequired},
		"whatsapp success needs a channel id": {phoneForm(), site.FormSettings{Success: &site.Success{Kind: site.SuccessWhatsApp}}, ErrSuccessChannelRequired},
		"a live legacy funnel stays live":     {phoneForm(), site.FormSettings{FunnelID: "f1"}, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := CheckForm(tc.manifest, tc.settings); !errors.Is(err, tc.want) {
				t.Fatalf("CheckForm = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestTheSuccessActionMatchesWhatTheRuntimeReads(t *testing.T) {
	cases := map[string]struct {
		success *site.Success
		url     string
		want    string
	}{
		"nothing configured": {nil, "", `{"kind":"message"}`},
		"a message":          {&site.Success{Kind: site.SuccessMessage, Message: "Obrigado!"}, "", `{"kind":"message","message":"Obrigado!"}`},
		"a redirect":         {&site.Success{Kind: site.SuccessRedirect, Route: "/obrigado"}, "", `{"kind":"redirect","route":"/obrigado"}`},
		"open whatsapp":      {&site.Success{Kind: site.SuccessWhatsApp, Channel: "n1"}, "https://vz.co/r/Ab3x", `{"kind":"whatsapp","url":"https://vz.co/r/Ab3x"}`},
		"whatsapp no link":   {&site.Success{Kind: site.SuccessWhatsApp, Channel: "n1"}, "", `{"kind":"message"}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(SuccessFor(tc.success, tc.url))
			if err != nil || string(raw) != tc.want {
				t.Fatalf("SuccessFor = %s (%v), want %s", raw, err, tc.want)
			}
		})
	}
}

func TestTheIPIsNeverKeptRaw(t *testing.T) {
	key := IPHashKey("a-forms-challenge-secret-with-32-chars!")
	first, again, other := HashIP(key, "203.0.113.7"), HashIP(key, " 203.0.113.7 "), HashIP(key, "203.0.113.8")
	if first == "" || first != again || first == other || first == "203.0.113.7" {
		t.Fatalf("hashes %q %q %q", first, again, other)
	}
	if HashIP(IPHashKey("another-secret-of-at-least-32-chars!!"), "203.0.113.7") == first {
		t.Fatal("a different secret must give a different hash")
	}
	if HashIP(key, "") != "" || HashIP(nil, "203.0.113.7") != "" {
		t.Fatal("no ip or no key hashes to nothing")
	}
}

func TestAFormKeyIsLongRandomAndURLSafe(t *testing.T) {
	seen := map[string]bool{}
	source := bytes.NewReader(sequence(100 * 18))
	for range 100 {
		key, err := NewFormKey(source)
		if err != nil {
			t.Fatal(err)
		}
		if !ValidFormKey(key) || len(key) < 20 || key == PreviewFormKey {
			t.Fatalf("key %q", key)
		}
		seen[key] = true
	}
	if len(seen) != 100 {
		t.Fatal("keys repeat")
	}
	for _, bad := range []string{"", "a b", "a/b", string(make([]byte, 129))} {
		if ValidFormKey(bad) {
			t.Fatalf("%q must be invalid", bad)
		}
	}
	if !ValidFormKey(PreviewFormKey) {
		t.Fatal("the preview key is a valid key")
	}
}

func sequence(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(i * 7)
	}
	return out
}

func TestAFormKeyComesOnlyFromTheGivenRandomness(t *testing.T) {
	key, err := NewFormKey(bytes.NewReader(bytes.Repeat([]byte{7}, 18)))
	if err != nil || key != "BwcHBwcHBwcHBwcHBwcHBwcH" {
		t.Fatalf("key %q err %v", key, err)
	}
	if _, err := NewFormKey(bytes.NewReader([]byte{1, 2, 3})); err == nil {
		t.Fatal("a short source of randomness must refuse")
	}
	if _, err := NewFormKey(nil); !errors.Is(err, ErrRandomnessMissing) {
		t.Fatalf("no randomness = %v", err)
	}
}
