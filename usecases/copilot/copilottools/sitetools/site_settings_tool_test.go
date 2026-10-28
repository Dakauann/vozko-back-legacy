package sitetools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/domain/site"
)

func TestSettingsReadOneSection(t *testing.T) {
	r := newRig(t)
	res := NewSiteSettingsTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"section":"forms","form_id":"contato"}`))
	data := decoded(t, res.Data)
	value := data["value"].(map[string]interface{})
	if res.Status != copilot.StatusOK || value["notify"] != true || len(r.storage.puts) != 0 {
		t.Fatalf("got %+v", res)
	}
}

func TestSettingsReplaceOneFormKeepingTheRestWithTheCurrentVersion(t *testing.T) {
	r := newRig(t)
	res := NewSiteSettingsTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"section":"forms","form_id":"orcamento","value":"{\"success\":{\"kind\":\"message\",\"message\":\"Obrigado!\"}}"}`))
	if res.Status != copilot.StatusOK {
		t.Fatalf("got %+v", res)
	}
	forms := r.storage.settings.Settings.Forms
	if forms["orcamento"].Success.Message != "Obrigado!" || !forms["contato"].Notify || r.storage.settings.Version != 2 {
		t.Fatalf("got %+v", r.storage.settings)
	}
	if pages := r.storage.settings.Settings.Pages; pages["/"].Title == "" {
		t.Fatal("other sections stay as they were")
	}
}

func TestSettingsReplaceASectionAndClearIt(t *testing.T) {
	r := newRig(t)
	tool := NewSiteSettingsTool(r.deps)
	if res := tool.Execute(context.Background(), r.cc, args(t, `{"section":"consent","value":"{\"banner\":true,\"categories\":[\"analytics\"]}"}`)); res.Status != copilot.StatusOK || !r.storage.settings.Settings.Consent.Banner {
		t.Fatalf("got %+v", res)
	}
	if res := tool.Execute(context.Background(), r.cc, args(t, `{"section":"consent","value":"null"}`)); res.Status != copilot.StatusOK || r.storage.settings.Settings.Consent.Banner {
		t.Fatalf("got %+v", res)
	}
}

func TestSettingsRefuseUnknownSectionsAndInvalidValues(t *testing.T) {
	r := newRig(t)
	tool := NewSiteSettingsTool(r.deps)
	for name, raw := range map[string]string{
		"unknown section": `{"section":"theme"}`,
		"form outside":    `{"section":"pages","form_id":"contato"}`,
		"bad json":        `{"section":"pages","value":"{titulo"}`,
	} {
		if res := tool.Execute(context.Background(), r.cc, args(t, raw)); res.Status != copilot.StatusError {
			t.Errorf("%s: got %+v", name, res)
		}
	}
	res := tool.Execute(context.Background(), r.cc, args(t, `{"section":"integrations","value":"{\"ga4\":\"UA-1\"}"}`))
	if res.Status != copilot.StatusError || !strings.Contains(res.Message, "não foram salvas") || r.storage.settings.Version != 1 {
		t.Fatalf("the backend rules decide, got %+v", res)
	}
}

func TestSettingsRetryOnceOnAConcurrentSave(t *testing.T) {
	r := newRig(t)
	r.storage.settings.Version = 2
	stale := *r.storage.settings
	stale.Version = 1
	tool := &siteSettingsTool{deps: r.deps}
	saved, err := tool.save(context.Background(), r.cc, projectID, &stale, settingsChange{section: "consent", value: rawJSON(`{"banner":true}`)})
	if err != nil || saved.Version != 3 || len(r.storage.puts) != 2 {
		t.Fatalf("got %+v %v", saved, err)
	}
}

func TestSettingsAskOnlyInAskModeAndNeedEditing(t *testing.T) {
	tool := NewSiteSettingsTool(Deps{})
	if tool.Meta() != editMeta() || !tool.(copilot.Graded).NeedsApproval(copilot.ModeAsk) || tool.(copilot.Graded).NeedsApproval(copilot.ModeEdit) {
		t.Fatalf("got %+v", tool.Meta())
	}
	if !strings.Contains(tool.Definition().Description, string(site.IntegrationTikTokPixel)) {
		t.Fatal("the tool names the integration keys the backend accepts")
	}
}

func rawJSON(text string) *json.RawMessage {
	raw := json.RawMessage(text)
	return &raw
}
