package whatsapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/whatsapp/template"
)

func TestMetaRefusalsThatMeanTheAccountIsGoneAreLostAccess(t *testing.T) {
	cases := []struct {
		name string
		body string
		lost bool
	}{
		{"the account is no longer visible to our token", `{"error":{"code":100,"error_subcode":33,"message":"Unsupported get request"}}`, true},
		{"the token itself is invalid", `{"error":{"code":190,"error_subcode":464,"message":"Error validating access token"}}`, true},
		{"permission was revoked", `{"error":{"code":10,"message":"Permission denied"}}`, true},
		{"a bad parameter is not lost access", `{"error":{"code":100,"message":"Invalid parameter"}}`, false},
		{"a rate limit is not lost access", `{"error":{"code":4,"message":"Too many calls"}}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := template.AccountAccessLost(newTemplateAPIError("list templates", http.StatusBadRequest, []byte(c.body))); got != c.lost {
				t.Fatalf("AccountAccessLost = %v, want %v", got, c.lost)
			}
		})
	}
}

func TestListingTemplatesOfAnUnreachableAccountSaysAccessWasLost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":100,"error_subcode":33,"message":"Unsupported get request"}}`))
	}))
	defer server.Close()
	client := NewClient(Config{AccessToken: "token", PhoneNumberID: "phone-1", WABAId: "waba-1", BaseURL: server.URL})

	_, err := client.ListTemplates(context.Background(), conversation.ListTemplatesInput{})

	if !template.AccountAccessLost(err) {
		t.Fatalf("err = %v", err)
	}
}
