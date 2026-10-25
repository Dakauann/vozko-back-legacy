package tools_usecase

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"vozko/domain/redact"
)

type secretStore map[string]string

func (s secretStore) Resolve(_ context.Context, workspaceID string, names []string) (map[string]string, error) {
	out := map[string]string{}
	if workspaceID != "ws-1" {
		return out, nil
	}
	for _, n := range names {
		if v, ok := s[n]; ok {
			out[n] = v
		}
	}
	return out, nil
}

type capturedRequest struct {
	url     string
	headers http.Header
	body    string
	calls   int
}

func capturingClient(c *capturedRequest, status int, respHeaders http.Header, respBody string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		c.calls++
		c.url = req.URL.String()
		c.headers = req.Header.Clone()
		if req.Body != nil {
			b, _ := io.ReadAll(req.Body)
			c.body = string(b)
		}
		if respHeaders == nil {
			respHeaders = http.Header{}
		}
		return &http.Response{StatusCode: status, Header: respHeaders, Body: io.NopCloser(strings.NewReader(respBody)), Request: req}, nil
	})}
}

func secretConfig(extra map[string]interface{}) map[string]interface{} {
	config := map[string]interface{}{
		"url":            "https://api.example.com/orders?api_key={{secret.api_key}}",
		"method":         "POST",
		"headers":        map[string]interface{}{"Authorization": "{{secret.token}}", "Accept": "application/json"},
		"__workspace_id": "ws-1",
		"fields":         []interface{}{map[string]interface{}{"name": "note", "location": "body", "source": "model"}},
	}
	for k, v := range extra {
		config[k] = v
	}
	return config
}

func TestHTTPRequestToolResolvesSecretReferencesOnlyFromTheConfig(t *testing.T) {
	captured := &capturedRequest{}
	tool := NewHTTPRequestToolUseCase(clientEgress(capturingClient(captured, 200, nil, `{"ok":true}`)), secretStore{"token": "Bearer s3cr3t", "api_key": "k&1"})
	_, err := tool.ExecuteWithConfig(context.Background(), secretConfig(nil), map[string]interface{}{"note": "{{secret.token}}"})
	if err != nil {
		t.Fatal(err)
	}
	if captured.headers.Get("Authorization") != "Bearer s3cr3t" || captured.headers.Get("Accept") != "application/json" {
		t.Fatalf("headers %v", captured.headers)
	}
	if captured.url != "https://api.example.com/orders?api_key=k%261" {
		t.Fatalf("url %q", captured.url)
	}
	if !strings.Contains(captured.body, "{{secret.token}}") || strings.Contains(captured.body, "s3cr3t") {
		t.Fatalf("a value written by the model must never be rendered as a secret: %s", captured.body)
	}
}

func TestHTTPRequestToolRefusesWithoutSendingWhenASecretIsMissing(t *testing.T) {
	captured := &capturedRequest{}
	tool := NewHTTPRequestToolUseCase(clientEgress(capturingClient(captured, 200, nil, "")), secretStore{"token": "x"})
	res, err := tool.ExecuteWithConfig(context.Background(), secretConfig(nil), nil)
	if err != nil || !res.IsError || captured.calls != 0 {
		t.Fatalf("res %+v err %v calls %d", res, err, captured.calls)
	}
	if message, _ := res.Result.(string); !strings.Contains(message, "api_key") {
		t.Fatalf("the error must name the missing secret: %v", res.Result)
	}
}

func TestHTTPRequestToolRefusesWithoutSendingWhenSecretsAreUnavailable(t *testing.T) {
	captured := &capturedRequest{}
	tool := NewHTTPRequestToolUseCase(clientEgress(capturingClient(captured, 200, nil, "")), nil)
	res, err := tool.ExecuteWithConfig(context.Background(), secretConfig(nil), nil)
	if err != nil || !res.IsError || captured.calls != 0 {
		t.Fatalf("res %+v err %v calls %d", res, err, captured.calls)
	}
}

func TestHTTPRequestToolNeverHandsResponseHeadersToTheModel(t *testing.T) {
	captured := &capturedRequest{}
	headers := http.Header{"Set-Cookie": {"session=abcdef"}, "Authorization": {"Bearer leaked"}, "Content-Type": {"application/json"}}
	tool := NewHTTPRequestToolUseCase(clientEgress(capturingClient(captured, 200, headers, `{"status":"enviado"}`)), nil)
	res, err := tool.ExecuteWithConfig(context.Background(), map[string]interface{}{"url": "https://api.example.com/x", "method": "GET"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(res)
	for _, leak := range []string{"abcdef", "leaked", "Set-Cookie", "headers"} {
		if strings.Contains(string(encoded), leak) {
			t.Fatalf("%q reached the model: %s", leak, encoded)
		}
	}
	body := res.Result.(map[string]interface{})
	if body["status"] != "enviado" {
		t.Fatalf("body %v", body)
	}
}

func TestHTTPRequestToolCapsTheBodyTheModelReads(t *testing.T) {
	captured := &capturedRequest{}
	big := `{"items":"` + strings.Repeat("a", 40000) + `"}`
	tool := NewHTTPRequestToolUseCase(clientEgress(capturingClient(captured, 200, nil, big)), nil)
	res, err := tool.ExecuteWithConfig(context.Background(), map[string]interface{}{"url": "https://api.example.com/x", "method": "GET"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := res.Result.(string)
	if !ok || len([]rune(body)) > ModelResultCharLimit+len([]rune(truncatedMarker)) || !strings.HasSuffix(body, truncatedMarker) {
		t.Fatalf("body of %d runes, ok=%v", len([]rune(body)), ok)
	}
}

func TestHTTPRequestToolScrubsSecretValuesTheServerEchoes(t *testing.T) {
	captured := &capturedRequest{}
	tool := NewHTTPRequestToolUseCase(clientEgress(capturingClient(captured, 401, nil, `{"echo":"Bearer s3cr3t is invalid"}`)), secretStore{"token": "Bearer s3cr3t", "api_key": "k"})
	res, _ := tool.ExecuteWithConfig(context.Background(), secretConfig(nil), nil)
	encoded, _ := json.Marshal(res)
	if strings.Contains(string(encoded), "s3cr3t") || !strings.Contains(string(encoded), redact.Mask) {
		t.Fatalf("result %s", encoded)
	}
}
