package workflow

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func retirementOutputs(n Node) ([]HandleDefinition, bool) {
	switch n.Type {
	case NodeTypeActionHTTPRequest:
		return []HandleDefinition{{ID: "sucesso"}, {ID: "falha", Optional: true}}, true
	case NodeTypeActionSetVariable, NodeTypeActionSendText, NodeTypeWaitForReply, NodeTypeActionTransferDepartment:
		return nil, true
	}
	return nil, false
}

func agentConfig(mode string) map[string]interface{} {
	config := map[string]interface{}{"custom_tools": []interface{}{map[string]interface{}{"name": "buscar_cep"}}}
	if mode != "" {
		config[retiredToolModeKey] = mode
	}
	return config
}

func storedGraph(t *testing.T, g Graph) []byte {
	t.Helper()
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func retire(t *testing.T, raw []byte) (ToolModeRetirement, Graph) {
	t.Helper()
	result, err := RetireToolModeJSON(raw, retirementOutputs)
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	var g Graph
	if err := json.Unmarshal(result.Graph, &g); err != nil {
		t.Fatalf("read back: %v", err)
	}
	return result, g
}

func edgesFrom(g Graph, source string) []string {
	var out []string
	for _, e := range g.OutgoingEdges(source) {
		out = append(out, e.Label+">"+e.Target)
	}
	sort.Strings(out)
	return out
}

func routeAgentGraph(mode string) Graph {
	return Graph{
		Nodes: []Node{
			{ID: "t", Type: NodeTypeTriggerWebhook, Config: map[string]interface{}{}},
			{ID: "agent", Type: NodeTypeActionAIAgent, Config: agentConfig(mode)},
			{ID: "http", Type: NodeTypeActionHTTPRequest, Position: Position{X: 100, Y: 40}, Config: map[string]interface{}{}},
			{ID: "handoff", Type: NodeTypeActionTransferDepartment, Config: map[string]interface{}{}},
			{ID: "save", Type: NodeTypeActionSetVariable, Config: map[string]interface{}{}},
			{ID: "back", Type: NodeTypeActionSetVariable, Config: map[string]interface{}{}},
			{ID: "reply", Type: NodeTypeActionSendText, Config: map[string]interface{}{}},
			{ID: "end", Type: NodeTypeEnd, Config: map[string]interface{}{}},
		},
		Edges: []Edge{
			{Source: "t", Target: "agent"},
			{Source: "agent", Target: "http", Label: "buscar_cep"},
			{Source: "agent", Target: "handoff", Label: "humano"},
			{Source: "agent", Target: "save", Label: "anotar"},
			{Source: "save", Target: "back"},
			{Source: "back", Target: "agent"},
			{Source: "agent", Target: "reply", Label: "default"},
			{Source: "reply", Target: "end"},
		},
	}
}

func TestARouteAgentsOpenToolLeafGetsAnEndOnEveryOutput(t *testing.T) {
	for _, mode := range []string{"route", ""} {
		result, g := retire(t, storedGraph(t, routeAgentGraph(mode)))
		if !result.Changed {
			t.Fatalf("mode %q: nothing changed", mode)
		}
		end := g.FindNode("end_http")
		if end == nil || end.Type != NodeTypeEnd || end.Position.X != 100+toolLeafEndOffsetX || end.Position.Y != 40 {
			t.Fatalf("mode %q: end node = %+v", mode, end)
		}
		if got, want := edgesFrom(g, "http"), []string{"falha>end_http", "sucesso>end_http"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("mode %q: http edges = %v, want %v", mode, got, want)
		}
		for _, untouched := range []string{"handoff", "back", "reply"} {
			if g.FindNode("end_"+untouched) != nil {
				t.Fatalf("mode %q: %s needs no end", mode, untouched)
			}
		}
		if len(edgesFrom(g, "handoff")) != 0 {
			t.Fatalf("mode %q: a hand off already ends the run", mode)
		}
		if _, carries := g.FindNode("agent").Config[retiredToolModeKey]; carries {
			t.Fatalf("mode %q: tool_mode kept", mode)
		}
	}
}

func TestAnExecuteAgentsOpenLeafIsDrawnBackToTheAgent(t *testing.T) {
	result, g := retire(t, storedGraph(t, routeAgentGraph("execute")))
	if !result.Changed || len(result.Held) != 0 {
		t.Fatalf("result = %+v", result)
	}
	if got, want := edgesFrom(g, "http"), []string{"falha>agent", "sucesso>agent"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("http edges = %v, want %v", got, want)
	}
	if g.FindNode("end_http") != nil || len(edgesFrom(g, "handoff")) != 0 {
		t.Fatal("an execute agent gets no end nodes and its hand off now ends the run")
	}
	if _, carries := g.FindNode("agent").Config[retiredToolModeKey]; carries {
		t.Fatal("tool_mode kept")
	}
}

func TestAnExecuteLeafAlsoReachedWithoutTheToolKeepsItsModeAndIsReported(t *testing.T) {
	stored := routeAgentGraph("execute")
	stored.Edges = append(stored.Edges, Edge{Source: "reply", Target: "http"})
	result, g := retire(t, storedGraph(t, stored))
	if result.Changed || !reflect.DeepEqual(result.Held, []string{"agent"}) {
		t.Fatalf("result = %+v, want the agent held and nothing rewritten", result)
	}
	if mode, _ := g.FindNode("agent").Config[retiredToolModeKey].(string); mode != "execute" {
		t.Fatalf("held agent lost its mode: %v", g.FindNode("agent").Config)
	}
}

func mixedModesGraph() Graph {
	g := routeAgentGraph("route")
	g.Nodes = append(g.Nodes,
		Node{ID: "exec", Type: NodeTypeActionAIAgent, Config: agentConfig("execute")},
		Node{ID: "lookup", Type: NodeTypeActionHTTPRequest, Config: map[string]interface{}{}},
		Node{ID: "shared", Type: NodeTypeActionSetVariable, Config: map[string]interface{}{}},
		Node{ID: "held", Type: NodeTypeActionAIAgent, Config: agentConfig("execute")},
	)
	for i := range g.Edges {
		if g.Edges[i].Source == "reply" {
			g.Edges[i].Target = "exec"
		}
	}
	g.Edges = append(g.Edges,
		Edge{Source: "exec", Target: "lookup", Label: "consultar"},
		Edge{Source: "exec", Target: "held", Label: "default"},
		Edge{Source: "held", Target: "shared", Label: "anotar"},
		Edge{Source: "held", Target: "shared", Label: "default"},
	)
	return g
}

func TestMixedModesRetireTogether(t *testing.T) {
	result, g := retire(t, storedGraph(t, mixedModesGraph()))
	if !result.Changed || !reflect.DeepEqual(result.Held, []string{"held"}) {
		t.Fatalf("result = %+v", result)
	}
	if got, want := edgesFrom(g, "lookup"), []string{"falha>exec", "sucesso>exec"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lookup edges = %v, want %v", got, want)
	}
	if got, want := edgesFrom(g, "http"), []string{"falha>end_http", "sucesso>end_http"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("http edges = %v, want %v", got, want)
	}
	if len(edgesFrom(g, "shared")) != 0 {
		t.Fatal("a leaf the held agent shares with its answer stays open")
	}
	for id, wantMode := range map[string]bool{"agent": false, "exec": false, "held": true} {
		if _, carries := g.FindNode(id).Config[retiredToolModeKey]; carries != wantMode {
			t.Fatalf("%s carries tool_mode = %v, want %v", id, carries, wantMode)
		}
	}
}

func TestASecondRetirementChangesNothing(t *testing.T) {
	for _, stored := range []Graph{routeAgentGraph("route"), routeAgentGraph(""), routeAgentGraph("execute"), mixedModesGraph()} {
		first, _ := retire(t, storedGraph(t, stored))
		second, _ := retire(t, first.Graph)
		if second.Changed || string(second.Graph) != string(first.Graph) {
			t.Fatalf("second run changed the graph:\n%s\n%s", first.Graph, second.Graph)
		}
	}
}

func TestAGraphWithoutAgentsOrModesIsLeftAsStored(t *testing.T) {
	raw := []byte(`{"nodes":[{"id":"t","type":"trigger_webhook","config":{}},{"id":"e","type":"end","config":{}}],"edges":[{"source":"t","target":"e"}]}`)
	result, err := RetireToolModeJSON(raw, retirementOutputs)
	if err != nil || result.Changed || string(result.Graph) != string(raw) {
		t.Fatalf("result = %+v %v", result, err)
	}
}

func TestTheRetirementKeepsFieldsItDoesNotKnow(t *testing.T) {
	raw := storedGraph(t, routeAgentGraph("route"))
	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["future"] = map[string]interface{}{"kept": true}
	doc["nodes"].([]interface{})[1].(map[string]interface{})["extra"] = "kept"
	withExtras, _ := json.Marshal(doc)
	result, err := RetireToolModeJSON(withExtras, retirementOutputs)
	if err != nil || !result.Changed {
		t.Fatalf("result = %+v %v", result, err)
	}
	if !strings.Contains(string(result.Graph), `"future":{"kept":true}`) || !strings.Contains(string(result.Graph), `"extra":"kept"`) {
		t.Fatalf("unknown fields dropped: %s", result.Graph)
	}
}

func TestAnUnreadableGraphOrAnUnknownLeafIsRefusedWhole(t *testing.T) {
	if _, err := RetireToolModeJSON([]byte("{not json"), retirementOutputs); err == nil {
		t.Fatal("unreadable graph accepted")
	}
	stored := routeAgentGraph("route")
	stored.Nodes[2].Type = NodeTypeActionRunTool
	if result, err := RetireToolModeJSON(storedGraph(t, stored), retirementOutputs); err == nil {
		t.Fatalf("a leaf with unknown outputs must refuse the whole graph, got %s", result.Graph)
	}
}
