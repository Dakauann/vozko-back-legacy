package agent_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/redact"
	toolsdomain "vozko/domain/tools"
	"vozko/domain/workspace"
	"vozko/domain/workspacesecret"
	agenttool_usecase "vozko/usecases/agenttool"
)

type httpRegistry struct{ toolsdomain.Service }

func (httpRegistry) Definitions() []toolsdomain.Definition {
	return []toolsdomain.Definition{{
		Name: "http_request",
		ConfigSchema: map[string]toolsdomain.ConfigParameter{
			"url":     {Type: "string"},
			"method":  {Type: "string"},
			"headers": {Type: "object", Sensitive: true},
		},
		RequiredConfig: []string{"url", "method"},
	}}
}

func (httpRegistry) Handler(string) (toolsdomain.Handler, bool) { return nil, false }

type vault struct{ values map[string]string }

func (v *vault) Resolve(_ context.Context, _ string, names []string) (map[string]string, error) {
	out := map[string]string{}
	for _, n := range names {
		if val, ok := v.values[n]; ok {
			out[n] = val
		}
	}
	return out, nil
}

func (v *vault) Put(_ context.Context, _, _, name, value string) (workspacesecret.Summary, error) {
	v.values[name] = value
	return workspacesecret.Summary{Name: name}, nil
}

func httpBinding(headers map[string]interface{}) agent.ToolBinding {
	return agent.ToolBinding{Name: "http_request", Config: map[string]interface{}{"url": "https://api.example.com/x", "method": "GET", "headers": headers}}
}

func sealingFixture(store agenttool_usecase.SecretStore) (agent.CreateAgentUseCase, agent.UpdateAgentUseCase, *agentStore) {
	agents := &agentStore{byID: map[string]*agent.Agent{
		"ag-http": {ID: "ag-http", WorkspaceID: "ws1", Name: "Ana", MessagingPrompt: "p", MessagingModel: "m", Provider: agent.AgentProviderAI,
			InternalTools: []agent.ToolBinding{httpBinding(map[string]interface{}{"Authorization": "{{secret.agent_aghttp_http_request_authorization}}"})}},
	}}
	create := NewCreateAgentUseCase(agents, httpRegistry{}, nil)
	update := NewUpdateAgentUseCase(agents, httpRegistry{}, nil)
	validated(create, update)
	if store != nil {
		sealer := agenttool_usecase.NewSealer(store)
		create.(ToolSecretSealed).SealToolSecretsWith(sealer)
		update.(ToolSecretSealed).SealToolSecretsWith(sealer)
	}
	return create, update, agents
}

func storedHeader(t *testing.T, a *agent.Agent, name string) interface{} {
	t.Helper()
	for _, b := range a.InternalTools {
		if b.Name == "http_request" {
			return b.Config["headers"].(map[string]interface{})[name]
		}
	}
	t.Fatal("no http_request binding stored")
	return nil
}

func TestCreatingAnAgentStoresHeaderCredentialsAsSecretReferences(t *testing.T) {
	store := &vault{values: map[string]string{}}
	create, _, agents := sealingFixture(store)
	in := createInput("u-1", httpBinding(map[string]interface{}{"Authorization": "Bearer live", "Accept": "json"}))
	created, err := create.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	stored := agents.byID[created.ID]
	ref, _ := storedHeader(t, stored, "Authorization").(string)
	if ref == "Bearer live" || len(store.values) != 1 {
		t.Fatalf("stored %q vault %v", ref, store.values)
	}
	if storedHeader(t, stored, "Accept") != "json" {
		t.Fatal("non credential headers stay as typed")
	}
	for name, value := range store.values {
		if ref != "{{secret."+name+"}}" || value != "Bearer live" {
			t.Fatalf("ref %q name %q value %q", ref, name, value)
		}
	}
}

func TestUpdatingWithTheMaskKeepsTheStoredReference(t *testing.T) {
	store := &vault{values: map[string]string{}}
	_, update, agents := sealingFixture(store)
	_, err := update.Execute(context.Background(), "ag-http", agent.UpdateAgentInput{
		InternalTools: []agent.ToolBinding{httpBinding(map[string]interface{}{"Authorization": redact.Mask})},
		RequestedBy:   workspace.Requester{UserID: "u-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := storedHeader(t, agents.byID["ag-http"], "Authorization"); got != "{{secret.agent_aghttp_http_request_authorization}}" || len(store.values) != 0 {
		t.Fatalf("got %v vault %v", got, store.values)
	}
}

func TestAPlainCredentialIsRefusedWhenNoSecretStoreIsWired(t *testing.T) {
	create, _, agents := sealingFixture(nil)
	_, err := create.Execute(context.Background(), createInput("u-1", httpBinding(map[string]interface{}{"Authorization": "Bearer live"})))
	if !errors.Is(err, agenttool_usecase.ErrSecretStoreUnavailable) || agents.writes != 0 {
		t.Fatalf("err %v writes %d", err, agents.writes)
	}
}

func newTestSealer(store agenttool_usecase.SecretStore) ToolSecretSealer {
	return agenttool_usecase.NewSealer(store)
}

type greenValidator struct{}

func (greenValidator) Validate(context.Context, *agent.Agent, []agent.ToolBinding) agenttool.Report {
	return agenttool.Report{}
}

func validated(ucs ...interface{}) {
	for _, uc := range ucs {
		uc.(ToolValidated).ValidateToolsWith(greenValidator{}, nil)
	}
}
