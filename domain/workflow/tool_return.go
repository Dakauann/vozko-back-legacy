package workflow

import "strings"

const PendingToolCallKey = "_pending_tool_call"

const agentErrorOutput = "erro"

func ToolReturnWhenPathEnds(g *Graph, state *RunState, ended *Node) (string, bool) {
	if ended == nil || ended.Type.EndsAutomationOwnership() {
		return "", false
	}
	return toolCaller(g, state, ended)
}

func ToolReturnWhenStepFails(g *Graph, state *RunState, failed *Node) (string, bool) {
	return toolCaller(g, state, failed)
}

func toolCaller(g *Graph, state *RunState, at *Node) (string, bool) {
	if g == nil || state == nil || at == nil {
		return "", false
	}
	raw, present := state.Get(PendingToolCallKey)
	pending, ok := raw.(map[string]interface{})
	if !present || !ok {
		return "", false
	}
	source, _ := pending["source_ai_node"].(string)
	agent := g.FindNode(source)
	if agent == nil || agent.Type != NodeTypeActionAIAgent || agent.ID == at.ID {
		return "", false
	}
	return agent.ID, true
}

func IsToolOutput(label string) bool {
	switch strings.TrimSpace(label) {
	case "", defaultOutput, agentErrorOutput:
		return false
	}
	return true
}

func agentIDs(g *Graph) map[string]bool {
	agents := make(map[string]bool)
	for _, n := range g.Nodes {
		if n.Type == NodeTypeActionAIAgent {
			agents[n.ID] = true
		}
	}
	return agents
}

func isToolEdge(agents map[string]bool, e Edge) bool {
	return agents[e.Source] && IsToolOutput(e.Label)
}

func ToolPathNodes(g *Graph, agentID string) map[string]bool {
	agents := agentIDs(g)
	var queue []string
	for _, e := range g.OutgoingEdges(agentID) {
		if isToolEdge(agents, e) {
			queue = append(queue, e.Target)
		}
	}
	nodes := make(map[string]bool)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == agentID || nodes[id] {
			continue
		}
		nodes[id] = true
		for _, e := range g.OutgoingEdges(id) {
			if !isToolEdge(agents, e) {
				queue = append(queue, e.Target)
			}
		}
	}
	return nodes
}

func ToolReturnLeaves(g *Graph) map[string]bool {
	agents := agentIDs(g)
	if len(agents) == 0 {
		return nil
	}
	var triggers []string
	for _, n := range g.Nodes {
		if n.Type.IsTrigger() {
			triggers = append(triggers, n.ID)
		}
	}
	everywhere := reachableAlong(g, triggers, func(Edge) bool { return true })
	withoutTools := reachableAlong(g, triggers, func(e Edge) bool { return !isToolEdge(agents, e) })
	leaves := make(map[string]bool)
	for id := range everywhere {
		if !withoutTools[id] && len(g.OutgoingEdges(id)) == 0 {
			leaves[id] = true
		}
	}
	return leaves
}

func reachableAlong(g *Graph, starts []string, follow func(Edge) bool) map[string]bool {
	seen := make(map[string]bool, len(starts))
	queue := append([]string(nil), starts...)
	for _, id := range starts {
		seen[id] = true
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, e := range g.OutgoingEdges(id) {
			if follow(e) && !seen[e.Target] {
				seen[e.Target] = true
				queue = append(queue, e.Target)
			}
		}
	}
	return seen
}

func ToolReturnRule() string {
	names := make([]string, len(ownershipEndingNodeTypes))
	for i, nodeType := range ownershipEndingNodeTypes {
		names[i] = string(nodeType)
	}
	last := len(names) - 1
	return "Quando o caminho de uma ferramenta termina sem próxima aresta, o fluxo volta sozinho ao agente com o resultado, " +
		"e quando um passo do caminho falha sem a saída erro conectada, volta com o erro; se o caminho terminar em " +
		strings.Join(names[:last], ", ") + " ou " + names[last] + ", a conversa sai do agente ali."
}
