package agent_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agent"
)

type legacyBindingsPage struct {
	rows  []agent.StoredAgentTools
	saved map[string][]agent.ToolBinding
	fail  error
}

func (p *legacyBindingsPage) AgentToolsAfter(_ context.Context, afterID string, limit int) ([]agent.StoredAgentTools, error) {
	if p.fail != nil {
		return nil, p.fail
	}
	var out []agent.StoredAgentTools
	for _, row := range p.rows {
		if row.AgentID > afterID && len(out) < limit {
			stored := row
			if saved, ok := p.saved[row.AgentID]; ok {
				stored.Bindings = saved
			}
			out = append(out, stored)
		}
	}
	return out, nil
}

func (p *legacyBindingsPage) SaveAgentTools(_ context.Context, agentID string, bindings []agent.ToolBinding) error {
	p.saved[agentID] = bindings
	return nil
}

func legacyRows() []agent.StoredAgentTools {
	return []agent.StoredAgentTools{
		{AgentID: "a1", WorkspaceID: "ws1", Bindings: []agent.ToolBinding{httpBinding(map[string]interface{}{"Authorization": "Bearer one"})}},
		{AgentID: "a2", WorkspaceID: "ws1", Bindings: []agent.ToolBinding{httpBinding(map[string]interface{}{"Authorization": "{{secret.kept}}"})}},
		{AgentID: "a3", WorkspaceID: "ws2", Bindings: []agent.ToolBinding{httpBinding(map[string]interface{}{"X-API-Key": "two", "Accept": "json"})}},
	}
}

func TestTheSecretRepairSealsPlainCredentialsOnceAndIsIdempotent(t *testing.T) {
	page := &legacyBindingsPage{rows: legacyRows(), saved: map[string][]agent.ToolBinding{}}
	store := &vault{values: map[string]string{}}
	repair := NewToolSecretRepair(page, httpRegistry{}, newTestSealer(store), 2)

	moved, err := repair.Run(context.Background())
	if err != nil || moved != 2 {
		t.Fatalf("moved %d err %v", moved, err)
	}
	if _, touched := page.saved["a2"]; touched {
		t.Fatal("an agent without plain credentials must not be rewritten")
	}
	if got := storedHeaderOf(page.saved["a1"], "Authorization"); got == "Bearer one" || len(store.values) != 2 {
		t.Fatalf("a1 %v vault %v", got, store.values)
	}
	if storedHeaderOf(page.saved["a3"], "Accept") != "json" {
		t.Fatal("non credential headers stay")
	}

	again, err := repair.Run(context.Background())
	if err != nil || again != 0 || len(store.values) != 2 {
		t.Fatalf("second run moved %d err %v vault %v", again, err, store.values)
	}
}

func TestTheSecretRepairReportsAReadFailure(t *testing.T) {
	page := &legacyBindingsPage{fail: errors.New("db down"), saved: map[string][]agent.ToolBinding{}}
	if _, err := NewToolSecretRepair(page, httpRegistry{}, newTestSealer(&vault{values: map[string]string{}}), 10).Run(context.Background()); err == nil {
		t.Fatal("expected the read failure")
	}
}

func storedHeaderOf(bindings []agent.ToolBinding, name string) interface{} {
	for _, b := range bindings {
		if headers, ok := b.Config["headers"].(map[string]interface{}); ok {
			return headers[name]
		}
	}
	return nil
}
