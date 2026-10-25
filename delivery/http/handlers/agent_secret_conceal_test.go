package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/redact"
	"vozko/domain/tools"
)

type legacyPlainAgent struct{}

func (legacyPlainAgent) Execute(id string) (*agent.Agent, error) {
	return &agent.Agent{ID: id, WorkspaceID: "ws1", InternalTools: []agent.ToolBinding{{
		Name: "http_request",
		Config: map[string]interface{}{
			"url":     "https://api.example.com/x?api_key=plain-key",
			"method":  "GET",
			"headers": map[string]interface{}{"Authorization": "Bearer plain-token", "X-API-Key": "{{secret.k}}", "Accept": "json"},
		},
	}}}, nil
}

func TestGettingAnAgentNeverReturnsAPlainCredential(t *testing.T) {
	h := &AgentHandler{getUseCase: legacyPlainAgent{}, toolRegistry: catalogOnlyTools{defs: []tools.Definition{{
		Name:         "http_request",
		ConfigSchema: map[string]tools.ConfigParameter{"headers": {Type: "object", Sensitive: true}},
	}}}}
	rec := httptest.NewRecorder()
	h.Get(rec, agentRequestAs(http.MethodGet, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, leak := range []string{"plain-token", "plain-key"} {
		if strings.Contains(body, leak) {
			t.Fatalf("%q returned: %s", leak, body)
		}
	}
	for _, kept := range []string{`{{secret.k}}`, `"Accept":"json"`, redact.Mask} {
		if !strings.Contains(body, kept) {
			t.Fatalf("%q missing from %s", kept, body)
		}
	}
}
