package agenttool_usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"vozko/domain/redact"
	"vozko/domain/workspacesecret"
)

type memorySecrets struct {
	values  map[string]string
	puts    []string
	putErr  error
	readErr error
}

func newMemorySecrets() *memorySecrets { return &memorySecrets{values: map[string]string{}} }

func (m *memorySecrets) Resolve(_ context.Context, _ string, names []string) (map[string]string, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	out := map[string]string{}
	for _, n := range names {
		if v, ok := m.values[n]; ok {
			out[n] = v
		}
	}
	return out, nil
}

func (m *memorySecrets) Put(_ context.Context, _, _, name, value string) (workspacesecret.Summary, error) {
	if m.putErr != nil {
		return workspacesecret.Summary{}, m.putErr
	}
	m.values[name] = value
	m.puts = append(m.puts, name)
	return workspacesecret.Summary{Name: name}, nil
}

func sealRequest(config, previous map[string]interface{}) SealRequest {
	return SealRequest{
		WorkspaceID: "ws-1",
		UserID:      "u-1",
		AgentID:     "3f2a9c11-0000-0000-0000-000000000000",
		ToolName:    "http_request",
		Sensitive:   []string{"headers"},
		Config:      config,
		Previous:    previous,
	}
}

func TestSealMovesPlainCredentialsIntoTheStoreAndKeepsReferences(t *testing.T) {
	store := newMemorySecrets()
	out, err := NewSealer(store).Seal(context.Background(), sealRequest(map[string]interface{}{
		"url":     "https://api.example.com/v1?api_key=abc&page=1",
		"headers": map[string]interface{}{"Authorization": "Bearer live", "Accept": "json"},
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	headers := out["headers"].(map[string]interface{})
	if headers["Authorization"] != "{{secret.agent_3f2a9c11_http_request_authorization}}" || headers["Accept"] != "json" {
		t.Fatalf("headers %v", headers)
	}
	if out["url"] != "https://api.example.com/v1?api_key={{secret.agent_3f2a9c11_http_request_api_key}}&page=1" {
		t.Fatalf("url %v", out["url"])
	}
	if store.values["agent_3f2a9c11_http_request_authorization"] != "Bearer live" || store.values["agent_3f2a9c11_http_request_api_key"] != "abc" {
		t.Fatalf("stored %v", store.values)
	}
}

func TestSealIsANoOpOnAnAlreadySealedConfig(t *testing.T) {
	store := newMemorySecrets()
	config := map[string]interface{}{"headers": map[string]interface{}{"Authorization": "{{secret.mine}}"}}
	out, err := NewSealer(store).Seal(context.Background(), sealRequest(config, nil))
	if err != nil || len(store.puts) != 0 || out["headers"].(map[string]interface{})["Authorization"] != "{{secret.mine}}" {
		t.Fatalf("out %v err %v puts %v", out, err, store.puts)
	}
}

func TestSealRotatesTheAgentsOwnSecretWhenANewValueIsTyped(t *testing.T) {
	store := newMemorySecrets()
	store.values["agent_3f2a9c11_http_request_authorization"] = "Bearer old"
	previous := map[string]interface{}{"headers": map[string]interface{}{"Authorization": "{{secret.agent_3f2a9c11_http_request_authorization}}"}}
	out, err := NewSealer(store).Seal(context.Background(), sealRequest(map[string]interface{}{
		"headers": map[string]interface{}{"Authorization": "Bearer new"},
	}, previous))
	if err != nil {
		t.Fatal(err)
	}
	if out["headers"].(map[string]interface{})["Authorization"] != "{{secret.agent_3f2a9c11_http_request_authorization}}" || store.values["agent_3f2a9c11_http_request_authorization"] != "Bearer new" {
		t.Fatalf("out %v stored %v", out, store.values)
	}
}

func TestSealNeverOverwritesASecretItDoesNotOwn(t *testing.T) {
	store := newMemorySecrets()
	store.values["agent_3f2a9c11_http_request_authorization"] = "someone else"
	out, err := NewSealer(store).Seal(context.Background(), sealRequest(map[string]interface{}{
		"headers": map[string]interface{}{"Authorization": "Bearer mine"},
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if out["headers"].(map[string]interface{})["Authorization"] != "{{secret.agent_3f2a9c11_http_request_authorization_2}}" || store.values["agent_3f2a9c11_http_request_authorization"] != "someone else" {
		t.Fatalf("out %v stored %v", out, store.values)
	}
}

func TestSealReusesASecretThatAlreadyHoldsTheSameValue(t *testing.T) {
	store := newMemorySecrets()
	store.values["agent_3f2a9c11_http_request_authorization"] = "Bearer same"
	_, err := NewSealer(store).Seal(context.Background(), sealRequest(map[string]interface{}{
		"headers": map[string]interface{}{"Authorization": "Bearer same"},
	}, nil))
	if err != nil || len(store.puts) != 0 {
		t.Fatalf("err %v puts %v", err, store.puts)
	}
}

func TestSealKeepsTheStoredReferenceWhenTheMaskComesBack(t *testing.T) {
	store := newMemorySecrets()
	previous := map[string]interface{}{"headers": map[string]interface{}{"Authorization": "{{secret.kept}}"}}
	out, err := NewSealer(store).Seal(context.Background(), sealRequest(map[string]interface{}{
		"headers": map[string]interface{}{"Authorization": redact.Mask},
	}, previous))
	if err != nil || out["headers"].(map[string]interface{})["Authorization"] != "{{secret.kept}}" || len(store.puts) != 0 {
		t.Fatalf("out %v err %v", out, err)
	}
}

func TestSealFailsClosedWithoutAStore(t *testing.T) {
	_, err := NewSealer(nil).Seal(context.Background(), sealRequest(map[string]interface{}{
		"headers": map[string]interface{}{"Authorization": "Bearer live"},
	}, nil))
	if !errors.Is(err, ErrSecretStoreUnavailable) {
		t.Fatalf("err %v", err)
	}
}

func TestSealSurfacesStoreFailuresWithoutLeakingTheValue(t *testing.T) {
	store := newMemorySecrets()
	store.putErr = errors.New("vault down")
	_, err := NewSealer(store).Seal(context.Background(), sealRequest(map[string]interface{}{
		"headers": map[string]interface{}{"Authorization": "Bearer live-value"},
	}, nil))
	if err == nil || strings.Contains(err.Error(), "live-value") {
		t.Fatalf("err %v", err)
	}
}
