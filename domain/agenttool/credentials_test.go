package agenttool

import (
	"reflect"
	"testing"

	"vozko/domain/redact"
)

var headerParams = []string{"headers"}

func TestPlainCredentialsFindsCredentialHeadersAndURLQueryValues(t *testing.T) {
	config := map[string]interface{}{
		"url": "https://api.example.com/v1/orders?api_key=abc123&page=2&token={{secret.t}}",
		"headers": map[string]interface{}{
			"Authorization": "Bearer live-token",
			"Content-Type":  "application/json",
			"X-API-Key":     "{{secret.api_key}}",
			"X-Signature":   "",
			"Cookie":        redact.Mask,
		},
	}
	got := PlainCredentials(headerParams, config)
	want := []Credential{
		{Param: "headers", Key: "Authorization", Value: "Bearer live-token"},
		{Param: "url", Key: "api_key", Value: "abc123"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestWithReferencesReplacesOnlyTheFoundValuesAndKeepsTheInputIntact(t *testing.T) {
	config := map[string]interface{}{
		"url":     "https://api.example.com/v1?api_key=abc123&page=2",
		"headers": map[string]interface{}{"Authorization": "Bearer live", "Accept": "application/json"},
		"method":  "GET",
	}
	creds := PlainCredentials(headerParams, config)
	out := WithReferences(config, creds, []string{"agent_a_auth", "agent_a_key"})
	headers := out["headers"].(map[string]interface{})
	if headers["Authorization"] != "{{secret.agent_a_auth}}" || headers["Accept"] != "application/json" {
		t.Fatalf("headers %v", headers)
	}
	if out["url"] != "https://api.example.com/v1?api_key={{secret.agent_a_key}}&page=2" {
		t.Fatalf("url %v", out["url"])
	}
	if config["headers"].(map[string]interface{})["Authorization"] != "Bearer live" {
		t.Fatal("the input config must not change")
	}
	if len(PlainCredentials(headerParams, out)) != 0 {
		t.Fatal("a sealed config has no plain credential left")
	}
}

func TestConcealPlainMasksLegacyPlainValuesAndLeavesReferences(t *testing.T) {
	config := map[string]interface{}{
		"url":     "https://api.example.com/v1?api_key=abc123&page=2",
		"headers": map[string]interface{}{"Authorization": "Bearer live", "X-API-Key": "{{secret.k}}", "Accept": "json"},
	}
	out := ConcealPlain(headerParams, config)
	headers := out["headers"].(map[string]interface{})
	if headers["Authorization"] != redact.Mask || headers["X-API-Key"] != "{{secret.k}}" || headers["Accept"] != "json" {
		t.Fatalf("headers %v", headers)
	}
	if out["url"] != "https://api.example.com/v1?api_key="+redact.Mask+"&page=2" {
		t.Fatalf("url %v", out["url"])
	}
}

func TestRestoreMaskedPutsBackThePreviousValueAndDropsUnknownMasks(t *testing.T) {
	previous := map[string]interface{}{
		"url":     "https://api.example.com/v1?api_key=abc123",
		"headers": map[string]interface{}{"Authorization": "{{secret.agent_a_auth}}"},
	}
	next := map[string]interface{}{
		"url":     "https://api.example.com/v2?api_key=" + redact.Mask,
		"headers": map[string]interface{}{"Authorization": redact.Mask, "Cookie": redact.Mask, "Accept": "json"},
	}
	out := RestoreMasked(headerParams, next, previous)
	headers := out["headers"].(map[string]interface{})
	if headers["Authorization"] != "{{secret.agent_a_auth}}" || headers["Accept"] != "json" {
		t.Fatalf("headers %v", headers)
	}
	if _, kept := headers["Cookie"]; kept {
		t.Fatalf("a mask without a previous value must be dropped, got %v", headers)
	}
	if out["url"] != "https://api.example.com/v2?api_key=abc123" {
		t.Fatalf("url %v", out["url"])
	}
}

func TestValueAtReadsHeadersURLQueryAndPlainParams(t *testing.T) {
	config := map[string]interface{}{
		"url":     "https://x.test/a?api_key={{secret.k}}&p=1",
		"headers": map[string]interface{}{"Authorization": "Bearer z"},
		"token":   "t",
	}
	if ValueAt(config, "url", "api_key") != "{{secret.k}}" || ValueAt(config, "headers", "Authorization") != "Bearer z" || ValueAt(config, "token", "") != "t" || ValueAt(config, "headers", "nope") != "" {
		t.Fatal("unexpected value")
	}
}

func canonicalConfig(token string, fieldValue string) map[string]interface{} {
	return map[string]interface{}{
		"url":    "https://api.test/x",
		"method": "GET",
		"fields": []interface{}{
			map[string]interface{}{"name": "X-API-Key", "location": "header", "source": "fixed", "value": fieldValue},
			map[string]interface{}{"name": "Accept", "location": "header", "source": "fixed", "value": "json"},
			map[string]interface{}{"name": "pedido", "location": "path", "source": "model"},
		},
		"auth": map[string]interface{}{"mode": "bearer", "token": token},
	}
}

func TestCanonicalFieldsAndAuthTokensAreCredentialLocations(t *testing.T) {
	config := canonicalConfig("live-token", "k-123")
	creds := PlainCredentials(nil, config)
	if len(creds) != 2 || creds[0].Param != FieldsParam || creds[0].Key != "X-API-Key" || creds[1].Param != AuthParam || creds[1].Key != AuthTokenKey {
		t.Fatalf("creds %+v", creds)
	}
	sealed := WithReferences(config, creds, []string{"a_key", "a_token"})
	if ValueAt(sealed, FieldsParam, "X-API-Key") != "{{secret.a_key}}" || ValueAt(sealed, AuthParam, AuthTokenKey) != "{{secret.a_token}}" {
		t.Fatalf("sealed %+v", sealed)
	}
	if ValueAt(config, FieldsParam, "X-API-Key") != "k-123" {
		t.Fatal("the input must not change")
	}
	if len(PlainCredentials(nil, sealed)) != 0 {
		t.Fatal("nothing plain left")
	}
	concealed := ConcealPlain(nil, config)
	if ValueAt(concealed, FieldsParam, "X-API-Key") != redact.Mask || ValueAt(concealed, AuthParam, AuthTokenKey) != redact.Mask || ValueAt(concealed, FieldsParam, "Accept") != "json" {
		t.Fatalf("concealed %+v", concealed)
	}
	restored := RestoreMasked(nil, concealed, sealed)
	if ValueAt(restored, FieldsParam, "X-API-Key") != "{{secret.a_key}}" || ValueAt(restored, AuthParam, AuthTokenKey) != "{{secret.a_token}}" {
		t.Fatalf("restored %+v", restored)
	}
}
