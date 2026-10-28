package sitetools

import (
	"testing"

	"vozko/domain/copilot"
)

func TestSettingsChangesToRedirectsAndIntegrationsAlwaysAsk(t *testing.T) {
	tool := NewSiteSettingsTool(Deps{})
	value := `[{"from":"/promo","to":"https://loja.example.com/","status":302}]`
	clear := "null"
	for _, c := range []struct {
		args map[string]interface{}
		asks bool
	}{
		{map[string]interface{}{"section": "redirects", "value": value}, true},
		{map[string]interface{}{"section": " integrations ", "value": `{"ga4":"G-ABC1234"}`}, true},
		{map[string]interface{}{"section": "redirects", "value": clear}, true},
		{map[string]interface{}{"section": "redirects"}, false},
		{map[string]interface{}{"section": "integrations"}, false},
		{map[string]interface{}{"section": "consent", "value": `{"banner":true}`}, false},
		{map[string]interface{}{"section": "forms", "form_id": "contato", "value": `{"notify":true}`}, false},
	} {
		for _, mode := range []copilot.Mode{copilot.ModeAsk, copilot.ModeEdit, copilot.ModeFull} {
			got := copilot.NeedsApprovalFor(tool, mode, c.args)
			want := c.asks || mode == copilot.ModeAsk
			if got != want {
				t.Fatalf("%v in %s: asks %v, want %v", c.args, mode, got, want)
			}
		}
	}
}
