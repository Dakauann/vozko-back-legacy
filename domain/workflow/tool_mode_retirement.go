package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	retiredToolModeKey     = "tool_mode"
	retiredToolModeExecute = "execute"
	toolLeafEndOffsetX     = 280
)

type ToolModeRetirement struct {
	Graph   []byte
	Changed bool
	Held    []string
}

type toolModePlan struct {
	nodes   []Node
	edges   []Edge
	cleared map[string]bool
	held    []string
}

func ToolModeRetirementGraphPath() string {
	return nodeTypeGraphPath([]string{string(NodeTypeActionAIAgent)})
}

func RetireToolModeJSON(raw []byte, outputs DynamicHandleResolver) (ToolModeRetirement, error) {
	var g Graph
	if err := json.Unmarshal(raw, &g); err != nil {
		return ToolModeRetirement{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var doc map[string]interface{}
	if err := decoder.Decode(&doc); err != nil {
		return ToolModeRetirement{}, err
	}
	plan, err := planToolModeRetirement(&g, outputs)
	if err != nil {
		return ToolModeRetirement{}, err
	}
	if len(plan.nodes) == 0 && len(plan.edges) == 0 && len(plan.cleared) == 0 {
		return ToolModeRetirement{Graph: raw, Held: plan.held}, nil
	}
	if err := applyToolModePlan(doc, plan); err != nil {
		return ToolModeRetirement{}, err
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return ToolModeRetirement{}, err
	}
	return ToolModeRetirement{Graph: out, Changed: true, Held: plan.held}, nil
}

func planToolModeRetirement(g *Graph, outputs DynamicHandleResolver) (toolModePlan, error) {
	plan := toolModePlan{cleared: make(map[string]bool)}
	var routed, executing []string
	for _, n := range g.Nodes {
		if n.Type != NodeTypeActionAIAgent {
			continue
		}
		if retiredToolModeOf(n) == retiredToolModeExecute {
			executing = append(executing, n.ID)
		} else {
			routed = append(routed, n.ID)
		}
	}
	sort.Strings(routed)
	sort.Strings(executing)
	for _, agentID := range routed {
		for _, leafID := range openToolLeaves(g, agentID) {
			end := Node{ID: freeNodeID(g, "end_"+leafID), Type: NodeTypeEnd, Config: map[string]interface{}{}}
			leaf := g.FindNode(leafID)
			end.Position = Position{X: leaf.Position.X + toolLeafEndOffsetX, Y: leaf.Position.Y}
			if err := plan.connect(g, *leaf, end.ID, outputs); err != nil {
				return plan, err
			}
			g.Nodes = append(g.Nodes, end)
			plan.nodes = append(plan.nodes, end)
		}
	}
	for _, agentID := range executing {
		leaves := openToolLeaves(g, agentID)
		if !onlyReachedThroughToolsOf(g, agentID, leaves) {
			plan.held = append(plan.held, agentID)
			continue
		}
		for _, leafID := range leaves {
			if err := plan.connect(g, *g.FindNode(leafID), agentID, outputs); err != nil {
				return plan, err
			}
		}
	}
	held := make(map[string]bool, len(plan.held))
	for _, id := range plan.held {
		held[id] = true
	}
	for _, n := range g.Nodes {
		if _, carries := n.Config[retiredToolModeKey]; carries && n.Type == NodeTypeActionAIAgent && !held[n.ID] {
			plan.cleared[n.ID] = true
		}
	}
	return plan, nil
}

func (p *toolModePlan) connect(g *Graph, from Node, to string, outputs DynamicHandleResolver) error {
	handles, known := outputs(from)
	if !known {
		return fmt.Errorf("workflow: node %q (%s) has no known outputs", from.ID, from.Type)
	}
	labels := []string{""}
	if len(handles) > 0 {
		labels = make([]string, len(handles))
		for i, h := range handles {
			labels[i] = h.ID
		}
	}
	for _, label := range labels {
		e := Edge{Source: from.ID, Target: to, Label: label}
		g.Edges = append(g.Edges, e)
		p.edges = append(p.edges, e)
	}
	return nil
}

func retiredToolModeOf(n Node) string {
	raw, _ := n.Config[retiredToolModeKey].(string)
	return strings.TrimSpace(raw)
}

func openToolLeaves(g *Graph, agentID string) []string {
	var leaves []string
	for id := range ToolPathNodes(g, agentID) {
		n := g.FindNode(id)
		if n != nil && !n.Type.EndsAutomationOwnership() && len(g.OutgoingEdges(id)) == 0 {
			leaves = append(leaves, id)
		}
	}
	sort.Strings(leaves)
	return leaves
}

func onlyReachedThroughToolsOf(g *Graph, agentID string, leaves []string) bool {
	if len(leaves) == 0 {
		return true
	}
	agents := agentIDs(g)
	var starts []string
	for _, n := range g.Nodes {
		if n.Type.IsTrigger() || (agents[n.ID] && n.ID != agentID) {
			starts = append(starts, n.ID)
		}
	}
	elsewhere := reachableAlong(g, starts, func(e Edge) bool { return e.Source != agentID || !IsToolOutput(e.Label) })
	for _, id := range leaves {
		if elsewhere[id] {
			return false
		}
	}
	return true
}

func freeNodeID(g *Graph, base string) string {
	id := base
	for i := 2; g.FindNode(id) != nil; i++ {
		id = fmt.Sprintf("%s_%d", base, i)
	}
	return id
}

func applyToolModePlan(doc map[string]interface{}, plan toolModePlan) error {
	nodes, isList := doc["nodes"].([]interface{})
	if !isList {
		return fmt.Errorf("workflow: graph nodes are not a list")
	}
	for _, item := range nodes {
		node, _ := item.(map[string]interface{})
		id, _ := node["id"].(string)
		config, _ := node["config"].(map[string]interface{})
		if plan.cleared[id] && config != nil {
			delete(config, retiredToolModeKey)
		}
	}
	for _, n := range plan.nodes {
		nodes = append(nodes, map[string]interface{}{
			"id":       n.ID,
			"type":     string(n.Type),
			"position": map[string]interface{}{"x": n.Position.X, "y": n.Position.Y},
			"config":   map[string]interface{}{},
		})
	}
	doc["nodes"] = nodes
	edges, _ := doc["edges"].([]interface{})
	for _, e := range plan.edges {
		edge := map[string]interface{}{"source": e.Source, "target": e.Target}
		if e.Label != "" {
			edge["label"] = e.Label
		}
		edges = append(edges, edge)
	}
	if edges != nil {
		doc["edges"] = edges
	}
	return nil
}
