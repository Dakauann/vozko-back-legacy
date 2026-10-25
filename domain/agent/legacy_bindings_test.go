package agent

import "testing"

func TestLegacyBindingsRoundTripWithoutInstanceFields(t *testing.T) {
	raw := EncodeLegacyBindings([]ToolBinding{{ID: "t1", Name: "http_request", Function: "pedido", Visibility: []ToolVisibility{ToolVisibilityMessaging}, Config: map[string]interface{}{"url": "https://a"}}})
	if raw != `{"bindings":[{"name":"http_request","visibility":["messaging"],"config":{"url":"https://a"}}]}` {
		t.Fatalf("raw %s", raw)
	}
	back := DecodeLegacyBindings(raw)
	if len(back) != 1 || back[0].Name != "http_request" || back[0].Config["url"] != "https://a" || back[0].ID != "" {
		t.Fatalf("back %+v", back)
	}
	if len(DecodeLegacyBindings(`{"finish_conversation":""}`)) != 1 || len(DecodeLegacyBindings("")) != 0 {
		t.Fatal("old map shape and empty values")
	}
}
