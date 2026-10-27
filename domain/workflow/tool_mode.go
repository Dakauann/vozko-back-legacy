package workflow

import "strings"

type ToolMode string

const (
	ToolModeRoute   ToolMode = "route"
	ToolModeExecute ToolMode = "execute"
)

const (
	ToolModeKey        = "tool_mode"
	PendingToolCallKey = "_pending_tool_call"
)

func ValidToolMode(raw string) bool {
	switch ToolMode(strings.TrimSpace(raw)) {
	case "", ToolModeRoute, ToolModeExecute:
		return true
	}
	return false
}

func ToolModeOf(n *Node) ToolMode {
	if n == nil {
		return ToolModeRoute
	}
	if raw, _ := n.Config[ToolModeKey].(string); ToolMode(strings.TrimSpace(raw)) == ToolModeExecute {
		return ToolModeExecute
	}
	return ToolModeRoute
}

func ExecuteToolReturn(g *Graph, state *RunState) (string, bool) {
	if g == nil || state == nil {
		return "", false
	}
	raw, present := state.Get(PendingToolCallKey)
	pending, ok := raw.(map[string]interface{})
	if !present || !ok {
		return "", false
	}
	source, _ := pending["source_ai_node"].(string)
	agent := g.FindNode(source)
	if agent == nil || agent.Type != NodeTypeActionAIAgent || ToolModeOf(agent) != ToolModeExecute {
		return "", false
	}
	return agent.ID, true
}
