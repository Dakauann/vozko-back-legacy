package workflow

import (
	"reflect"
	"testing"
)

func toolGraph() *Graph {
	return &Graph{
		Nodes: []Node{
			{ID: "t", Type: NodeTypeTriggerWebhook},
			{ID: "agent", Type: NodeTypeActionAIAgent},
			{ID: "http", Type: NodeTypeActionHTTPRequest},
			{ID: "save", Type: NodeTypeActionSetVariable},
			{ID: "handoff", Type: NodeTypeActionTransferDepartment},
			{ID: "reply", Type: NodeTypeActionSendText},
			{ID: "end", Type: NodeTypeEnd},
		},
		Edges: []Edge{
			{Source: "t", Target: "agent"},
			{Source: "agent", Target: "http", Label: "buscar_cep"},
			{Source: "http", Target: "save", Label: "sucesso"},
			{Source: "agent", Target: "handoff", Label: "falar_com_humano"},
			{Source: "agent", Target: "reply", Label: "default"},
			{Source: "reply", Target: "end"},
		},
	}
}

func pendingFrom(source string) RunState {
	state := NewRunState()
	state.Set(PendingToolCallKey, map[string]interface{}{"name": "buscar_cep", "source_ai_node": source, "target_node": "http"})
	return state
}

func TestAToolPathEndingOnAnOrdinaryStepReturnsToTheCallingAgent(t *testing.T) {
	g := toolGraph()
	state := pendingFrom("agent")
	target, ok := ToolReturnWhenPathEnds(g, &state, g.FindNode("save"))
	if !ok || target != "agent" {
		t.Fatalf("return = %q %v, want the agent", target, ok)
	}
}

func TestAToolPathEndingOnAHandOffEndsTheRunThere(t *testing.T) {
	g := toolGraph()
	state := pendingFrom("agent")
	if target, ok := ToolReturnWhenPathEnds(g, &state, g.FindNode("handoff")); ok {
		t.Fatalf("a transfer must end the run, got a return to %q", target)
	}
}

func TestAPathWithoutAPendingToolCallNeverReturns(t *testing.T) {
	g := toolGraph()
	cases := map[string]RunState{
		"no pending call":        NewRunState(),
		"unknown source":         pendingFrom("ghost"),
		"source is not an agent": pendingFrom("http"),
	}
	for name, state := range cases {
		state := state
		if target, ok := ToolReturnWhenPathEnds(g, &state, g.FindNode("save")); ok {
			t.Errorf("%s: returned to %q", name, target)
		}
		if target, ok := ToolReturnWhenStepFails(g, &state, g.FindNode("http")); ok {
			t.Errorf("%s: failure returned to %q", name, target)
		}
	}
}

func TestAFailingStepOnAToolPathReturnsEvenWhenItWasAHandOff(t *testing.T) {
	g := toolGraph()
	state := pendingFrom("agent")
	for _, id := range []string{"http", "handoff"} {
		if target, ok := ToolReturnWhenStepFails(g, &state, g.FindNode(id)); !ok || target != "agent" {
			t.Fatalf("%s failure return = %q %v, want the agent", id, target, ok)
		}
	}
}

func TestTheCallingAgentsOwnFailureNeverReturnsToItself(t *testing.T) {
	g := toolGraph()
	state := pendingFrom("agent")
	if target, ok := ToolReturnWhenStepFails(g, &state, g.FindNode("agent")); ok {
		t.Fatalf("the agent's own failure returned to %q", target)
	}
}

func TestToolPathNodesFollowTheToolOutputsButNotTheAnswer(t *testing.T) {
	got := ToolPathNodes(toolGraph(), "agent")
	want := map[string]bool{"http": true, "save": true, "handoff": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tool path = %v, want %v", got, want)
	}
}

func TestANestedAgentsToolsBelongToTheNestedAgent(t *testing.T) {
	g := &Graph{
		Nodes: []Node{
			{ID: "t", Type: NodeTypeTriggerWebhook},
			{ID: "outer", Type: NodeTypeActionAIAgent},
			{ID: "inner", Type: NodeTypeActionAIAgent},
			{ID: "lookup", Type: NodeTypeActionHTTPRequest},
			{ID: "after", Type: NodeTypeActionSetVariable},
		},
		Edges: []Edge{
			{Source: "t", Target: "outer"},
			{Source: "outer", Target: "inner", Label: "perguntar"},
			{Source: "inner", Target: "lookup", Label: "buscar"},
			{Source: "inner", Target: "after", Label: "default"},
		},
	}
	if got, want := ToolPathNodes(g, "outer"), map[string]bool{"inner": true, "after": true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("outer tool path = %v, want %v", got, want)
	}
	if got, want := ToolPathNodes(g, "inner"), map[string]bool{"lookup": true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("inner tool path = %v, want %v", got, want)
	}
}

func TestOnlyLeavesReachedThroughAToolOutputAreToolReturns(t *testing.T) {
	g := toolGraph()
	g.Nodes = append(g.Nodes, Node{ID: "shared", Type: NodeTypeActionSetVariable})
	g.Edges = append(g.Edges, Edge{Source: "save", Target: "shared"}, Edge{Source: "reply", Target: "shared"})
	g.Edges = g.Edges[:len(g.Edges)-1]
	got := ToolReturnLeaves(g)
	want := map[string]bool{"shared": true, "handoff": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tool return leaves = %v, want %v", got, want)
	}
	g.Edges = append(g.Edges, Edge{Source: "reply", Target: "shared"})
	if leaves := ToolReturnLeaves(g); leaves["shared"] {
		t.Fatalf("a leaf also reached by the answer path is a dead end there: %v", leaves)
	}
}
