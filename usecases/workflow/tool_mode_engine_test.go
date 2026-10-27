package workflow_usecase

import (
	"testing"

	"vozko/domain/workflow"
)

func toolAgentWorkflow(mode string) *workflow.Workflow {
	return &workflow.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Type: workflow.WorkflowTypeMessages, Graph: workflow.Graph{
		Nodes: []workflow.Node{
			{ID: "t1", Type: workflow.NodeTypeTriggerWebhook},
			{ID: "agent", Type: workflow.NodeTypeActionAIAgent, Config: map[string]interface{}{workflow.ToolModeKey: mode}},
			{ID: "http", Type: workflow.NodeTypeActionHTTPRequest},
			{ID: "end", Type: workflow.NodeTypeEnd},
		},
		Edges: []workflow.Edge{
			{Source: "t1", Target: "agent"},
			{Source: "agent", Target: "http", Label: "buscar_cep"},
			{Source: "agent", Target: "end", Label: "default"},
		},
	}}
}

func runToolAgent(t *testing.T, mode string) (*traceHarness, *workflow.WorkflowRun, []string) {
	t.Helper()
	h := newTraceHarness()
	var visits []string
	h.register(workflow.NodeTypeActionAIAgent, func(ctx *workflow.NodeContext) (*workflow.NodeResult, error) {
		if _, pending := ctx.State.Get(workflow.PendingToolCallKey); pending {
			visits = append(visits, "agent:answered")
			ctx.State.Delete(workflow.PendingToolCallKey)
			return &workflow.NodeResult{NextNodeID: "end"}, nil
		}
		visits = append(visits, "agent:called_tool")
		ctx.State.Set(workflow.PendingToolCallKey, map[string]interface{}{"name": "buscar_cep", "source_ai_node": "agent", "target_node": "http"})
		return &workflow.NodeResult{NextNodeID: "http"}, nil
	})
	h.register(workflow.NodeTypeActionHTTPRequest, func(*workflow.NodeContext) (*workflow.NodeResult, error) {
		visits = append(visits, "http")
		return &workflow.NodeResult{Output: map[string]interface{}{"status_code": 200}}, nil
	})
	run := h.start(toolAgentWorkflow(mode))
	return h, run, visits
}

func TestAnExecuteModeToolReturnsToTheAgentWhenItsPathEnds(t *testing.T) {
	_, run, visits := runToolAgent(t, "execute")
	if len(visits) != 3 || visits[2] != "agent:answered" {
		t.Fatalf("visits = %v, want the agent to get the tool result and answer", visits)
	}
	if run.Status != workflow.RunStatusCompleted {
		t.Fatalf("status = %s", run.Status)
	}
}

func TestARouteModeToolPathEndsTheRunWhereItEnds(t *testing.T) {
	_, run, visits := runToolAgent(t, "route")
	if len(visits) != 2 || visits[1] != "http" {
		t.Fatalf("visits = %v, want route mode to follow only the drawn edges", visits)
	}
	if run.Status != workflow.RunStatusCompleted {
		t.Fatalf("status = %s", run.Status)
	}
}

func TestToolModeIsAKnownAgentSettingSoTheLintDoesNotCallItIgnored(t *testing.T) {
	catalog := productionCatalog(t)
	for _, mode := range []string{"route", "execute"} {
		w := toolAgentWorkflow(mode)
		w.Graph.Nodes[1].Config["custom_tools"] = []interface{}{map[string]interface{}{"name": "buscar_cep"}}
		for _, issue := range LintWorkflowGraph(catalog, w).Issues {
			if issue.Code == workflow.LintUnknownConfigKey && issue.Field == workflow.ToolModeKey {
				t.Fatalf("tool_mode %q flagged as unknown: %+v", mode, issue)
			}
		}
	}
	def, ok := workflow.NodeCatalogMap(catalog)[workflow.NodeTypeActionAIAgent]
	if !ok {
		t.Fatal("no agent definition")
	}
	for _, f := range def.ConfigSchema {
		if f.Key == workflow.ToolModeKey {
			if f.Type != "select" || len(f.Options) != 2 || f.Options[0].Value != string(workflow.ToolModeRoute) || f.Options[1].Value != string(workflow.ToolModeExecute) {
				t.Fatalf("tool_mode field = %+v, want a route or execute select", f)
			}
			if mode, _ := def.DefaultConfig[workflow.ToolModeKey].(string); mode != string(workflow.ToolModeRoute) {
				t.Fatalf("default tool_mode = %q, want route", mode)
			}
			return
		}
	}
	t.Fatal("tool_mode is not declared in the agent schema")
}
