package workflow

import "testing"

func agentGraph(mode string) *Graph {
	config := map[string]interface{}{"custom_tools": []interface{}{map[string]interface{}{"name": "buscar_cep"}}}
	if mode != "" {
		config[ToolModeKey] = mode
	}
	return &Graph{Nodes: []Node{
		{ID: "agent", Type: NodeTypeActionAIAgent, Config: config},
		{ID: "http", Type: NodeTypeActionHTTPRequest},
	}}
}

func pendingFrom(source string) RunState {
	state := NewRunState()
	state.Set(PendingToolCallKey, map[string]interface{}{"name": "buscar_cep", "source_ai_node": source, "target_node": "http"})
	return state
}

func TestToolModeDefaultsToRoute(t *testing.T) {
	cases := map[string]ToolMode{"": ToolModeRoute, "route": ToolModeRoute, "execute": ToolModeExecute}
	for raw, want := range cases {
		node := agentGraph(raw).FindNode("agent")
		if got := ToolModeOf(node); got != want {
			t.Errorf("tool_mode %q = %q, want %q", raw, got, want)
		}
	}
	if got := ToolModeOf(nil); got != ToolModeRoute {
		t.Errorf("no node = %q", got)
	}
}

func TestAnExecuteToolPathThatEndsGoesBackToItsAgent(t *testing.T) {
	state := pendingFrom("agent")
	target, ok := ExecuteToolReturn(agentGraph("execute"), &state)
	if !ok || target != "agent" {
		t.Fatalf("return = %q, %v; want the agent that called the tool", target, ok)
	}
}

func TestOnlyExecuteToolPathsReturnToTheAgent(t *testing.T) {
	cases := []struct {
		name  string
		graph *Graph
		state RunState
	}{
		{"route mode ends where the path ends", agentGraph("route"), pendingFrom("agent")},
		{"default mode is route", agentGraph(""), pendingFrom("agent")},
		{"no pending tool call", agentGraph("execute"), NewRunState()},
		{"the caller is not in the graph", agentGraph("execute"), pendingFrom("gone")},
		{"the caller is not an agent", agentGraph("execute"), pendingFrom("http")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if target, ok := ExecuteToolReturn(tc.graph, &tc.state); ok {
				t.Fatalf("returned to %q", target)
			}
		})
	}
}

func TestToolModeIsADeclaredConfigValue(t *testing.T) {
	if !ValidToolMode("") || !ValidToolMode("route") || !ValidToolMode("execute") || ValidToolMode("parallel") {
		t.Fatal("tool_mode accepts only route and execute, empty meaning route")
	}
}
