package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/domain/agent"
)

type recordingAgentUpdate struct {
	got    agent.UpdateAgentInput
	called bool
}

func (f *recordingAgentUpdate) Execute(_ context.Context, id string, in agent.UpdateAgentInput) (*agent.Agent, error) {
	f.got, f.called = in, true
	return &agent.Agent{ID: id, WorkspaceID: "ws1"}, nil
}

type recordingAgentCreate struct{ got agent.CreateAgentInput }

func (f *recordingAgentCreate) Execute(_ context.Context, in agent.CreateAgentInput) (*agent.Agent, error) {
	f.got = in
	return &agent.Agent{ID: "ag-new", WorkspaceID: "ws1"}, nil
}

func agentFieldsBody(t *testing.T, extra map[string]interface{}) []byte {
	t.Helper()
	body := map[string]interface{}{"name": "Ana", "messagingPrompt": "p", "messagingModel": "openai/gpt-4o", "provider": "platform", "isActive": false}
	for k, v := range extra {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestUpdatingAnAgentWithToolsIsRefusedBecauseToolsHaveTheirOwnEndpoints(t *testing.T) {
	update := &recordingAgentUpdate{}
	h := &AgentHandler{updateUseCase: update, getUseCase: ownAgent{}}
	rec := httptest.NewRecorder()
	h.Update(rec, agentRequestAs(http.MethodPut, agentFieldsBody(t, map[string]interface{}{
		"internalTools": []map[string]interface{}{{"name": "http_request"}},
	})))
	var resp struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if rec.Code != http.StatusBadRequest || resp.Code != "agent_tools_moved" || update.called {
		t.Fatalf("status %d code %q called %v body %s", rec.Code, resp.Code, update.called, rec.Body.String())
	}
}

func TestUpdatingAnAgentWithoutToolsKeepsItsTools(t *testing.T) {
	update := &recordingAgentUpdate{}
	h := &AgentHandler{updateUseCase: update, getUseCase: ownAgent{}}
	rec := httptest.NewRecorder()
	h.Update(rec, agentRequestAs(http.MethodPut, agentFieldsBody(t, nil)))
	if rec.Code != http.StatusOK || !update.called || update.got.InternalTools != nil {
		t.Fatalf("status %d tools %+v body %s", rec.Code, update.got.InternalTools, rec.Body.String())
	}
}

func TestCreatingAnAgentKeepsEachToolNameAndDescription(t *testing.T) {
	create := &recordingAgentCreate{}
	h := &AgentHandler{createUseCase: create}
	rec := httptest.NewRecorder()
	h.Create(rec, agentRequestAs(http.MethodPost, agentFieldsBody(t, map[string]interface{}{
		"internalTools": []map[string]interface{}{{"name": "http_request", "function": "consultar_pedido", "description": "Consulta o pedido.", "config": map[string]interface{}{"method": "GET"}}},
	})))
	if rec.Code != http.StatusCreated || len(create.got.InternalTools) != 1 {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	got := create.got.InternalTools[0]
	if got.Name != "http_request" || got.Function != "consultar_pedido" || got.Description != "Consulta o pedido." {
		t.Fatalf("binding %+v", got)
	}
}

func TestMCPCollectionsOnAnAgentBodyAreRefusedBecauseMCPServersAreTools(t *testing.T) {
	update := &recordingAgentUpdate{}
	create := &recordingAgentCreate{}
	h := &AgentHandler{updateUseCase: update, createUseCase: create, getUseCase: ownAgent{}}
	for method, call := range map[string]func(http.ResponseWriter, *http.Request){http.MethodPut: h.Update, http.MethodPost: h.Create} {
		rec := httptest.NewRecorder()
		call(rec, agentRequestAs(method, agentFieldsBody(t, map[string]interface{}{"internalTools": []interface{}{}, "mcpCollectionIds": []string{"col-1"}})))
		var resp struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if rec.Code != http.StatusBadRequest || (resp.Code != "agent_mcp_collections_moved" && resp.Code != "agent_tools_moved") {
			t.Fatalf("%s: status %d body %s", method, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.Create(rec, agentRequestAs(http.MethodPost, agentFieldsBody(t, map[string]interface{}{"internalTools": []interface{}{}, "mcpCollectionIds": []string{"col-1"}})))
	var resp struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Code != "agent_mcp_collections_moved" || update.called || create.got.Name != "" {
		t.Fatalf("code %q update %v create %+v", resp.Code, update.called, create.got)
	}
}
