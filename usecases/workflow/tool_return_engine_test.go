package workflow_usecase

import (
	"errors"
	"strings"
	"testing"
	"time"

	"vozko/domain/workflow"
)

type toolReturn struct {
	agent        string
	result       map[string]interface{}
	previous     string
	continuation map[string]interface{}
}

type toolAgents struct {
	visits  []string
	returns []toolReturn
	forever bool
}

func (a *toolAgents) execute(ctx *workflow.NodeContext) (*workflow.NodeResult, error) {
	me := ctx.Node.ID
	raw, pending := ctx.State.Get(workflow.PendingToolCallKey)
	call, _ := raw.(map[string]interface{})
	if pending && call["source_ai_node"] == me && !a.forever {
		previous := ctx.State.GetString("_prev_node_id")
		target, _ := call["target_node"].(string)
		a.returns = append(a.returns, toolReturn{agent: me, result: workflow.NodeOutput(ctx.State, target), previous: previous, continuation: workflow.NodeOutput(ctx.State, previous)})
		a.visits = append(a.visits, me+":answered")
		ctx.State.Delete(workflow.PendingToolCallKey)
		for _, e := range ctx.Graph.OutgoingEdges(me) {
			if e.Label == "default" {
				return &workflow.NodeResult{NextNodeID: e.Target, Output: map[string]interface{}{"response_text": "ok"}}, nil
			}
		}
		return &workflow.NodeResult{Complete: true}, nil
	}
	for _, e := range ctx.Graph.OutgoingEdges(me) {
		if workflow.IsToolOutput(e.Label) {
			a.visits = append(a.visits, me+":called "+e.Label)
			ctx.State.Set(workflow.PendingToolCallKey, map[string]interface{}{"name": e.Label, "source_ai_node": me, "target_node": e.Target})
			return &workflow.NodeResult{NextNodeID: e.Target, Output: map[string]interface{}{"tool_name": e.Label}}, nil
		}
	}
	return nil, errors.New("the agent has no tool")
}

func toolWorkflow(nodes []workflow.Node, edges []workflow.Edge) *workflow.Workflow {
	all := append([]workflow.Node{
		{ID: "t1", Type: workflow.NodeTypeTriggerWebhook},
		{ID: "agent", Type: workflow.NodeTypeActionAIAgent},
		{ID: "reply", Type: workflow.NodeTypeActionSendText},
		{ID: "end", Type: workflow.NodeTypeEnd},
	}, nodes...)
	return &workflow.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Type: workflow.WorkflowTypeMessages, Graph: workflow.Graph{
		Nodes: all,
		Edges: append([]workflow.Edge{
			{Source: "t1", Target: "agent"},
			{Source: "agent", Target: "reply", Label: "default"},
			{Source: "reply", Target: "end"},
		}, edges...),
	}}
}

func toolHarness(agents *toolAgents) *traceHarness {
	h := newTraceHarness()
	h.register(workflow.NodeTypeActionAIAgent, agents.execute)
	h.register(workflow.NodeTypeActionSendText, func(*workflow.NodeContext) (*workflow.NodeResult, error) {
		return &workflow.NodeResult{Output: map[string]interface{}{"sent": true}}, nil
	})
	return h
}

func answering(output map[string]interface{}) func(*workflow.NodeContext) (*workflow.NodeResult, error) {
	return func(*workflow.NodeContext) (*workflow.NodeResult, error) {
		return &workflow.NodeResult{Output: output}, nil
	}
}

func TestAToolPathEndingOnAnOrdinaryStepReturnsTheResultToTheAgent(t *testing.T) {
	agents := &toolAgents{}
	h := toolHarness(agents)
	h.register(workflow.NodeTypeActionHTTPRequest, answering(map[string]interface{}{"status_code": 200, "cidade": "Recife"}))
	run := h.start(toolWorkflow(
		[]workflow.Node{{ID: "http", Type: workflow.NodeTypeActionHTTPRequest}},
		[]workflow.Edge{{Source: "agent", Target: "http", Label: "buscar_cep"}},
	))
	if strings.Join(agents.visits, ",") != "agent:called buscar_cep,agent:answered" || run.Status != workflow.RunStatusCompleted {
		t.Fatalf("visits = %v status = %s", agents.visits, run.Status)
	}
	if got := agents.returns[0]; got.agent != "agent" || got.result["cidade"] != "Recife" || got.previous != "http" {
		t.Fatalf("tool return = %+v", got)
	}
	if h.step(t, "reply").Status != workflow.LogStatusExecuted {
		t.Fatal("the agent's answer must continue through its default output")
	}
}

func TestAToolPathEndingOnAClosingOrHandOffStepEndsTheRunThere(t *testing.T) {
	for _, terminal := range []workflow.NodeType{
		workflow.NodeTypeEnd, workflow.NodeTypeActionFinishConversation, workflow.NodeTypeActionTransferDepartment,
		workflow.NodeTypeActionAssignMember, workflow.NodeTypeActionReplyToAgent,
	} {
		agents := &toolAgents{}
		h := toolHarness(agents)
		h.register(terminal, answering(map[string]interface{}{"success": true}))
		run := h.start(toolWorkflow(
			[]workflow.Node{{ID: "out", Type: terminal}},
			[]workflow.Edge{{Source: "agent", Target: "out", Label: "encerrar"}},
		))
		if len(agents.visits) != 1 || run.Status != workflow.RunStatusCompleted {
			t.Fatalf("%s: visits = %v status = %s, want the run to end at the step", terminal, agents.visits, run.Status)
		}
	}
}

func TestAFailingStepWithoutAnErrorEdgeReturnsTheErrorAndIsStillRecorded(t *testing.T) {
	failures := map[string]func(*workflow.NodeContext) (*workflow.NodeResult, error){
		"unconnected error output": func(*workflow.NodeContext) (*workflow.NodeResult, error) {
			return &workflow.NodeResult{Error: `workflow: the output the node produced is not connected: "erro": HTTP 500`, ErrorKind: workflow.ErrorKindConfig, Output: map[string]interface{}{"success": false}}, nil
		},
		"executor error": func(*workflow.NodeContext) (*workflow.NodeResult, error) {
			return nil, workflow.Fail(workflow.ErrorKindValidation, errors.New("script crashed"))
		},
	}
	for name, fail := range failures {
		agents := &toolAgents{}
		h := toolHarness(agents)
		h.register(workflow.NodeTypeActionCode, fail)
		h.register(workflow.NodeTypeActionSetVariable, answering(map[string]interface{}{"saved": true}))
		run := h.start(toolWorkflow(
			[]workflow.Node{{ID: "code", Type: workflow.NodeTypeActionCode}, {ID: "save", Type: workflow.NodeTypeActionSetVariable}},
			[]workflow.Edge{{Source: "agent", Target: "code", Label: "calcular"}, {Source: "code", Target: "save", Label: "sucesso"}},
		))
		if run.Status != workflow.RunStatusCompleted || len(agents.returns) != 1 {
			t.Fatalf("%s: status = %s returns = %+v, want the agent to get the failure", name, run.Status, agents.returns)
		}
		message, _ := agents.returns[0].result["error"].(string)
		if message == "" {
			t.Fatalf("%s: the agent got no error: %+v", name, agents.returns[0])
		}
		if step := h.step(t, "code"); step.Status != workflow.LogStatusFailed || step.Error == "" {
			t.Fatalf("%s: the failure must stay recorded for the node alerts: %+v", name, step)
		}
	}
}

func TestAHandledFailureOnALeafIsRecordedAndReturned(t *testing.T) {
	agents := &toolAgents{}
	h := toolHarness(agents)
	h.register(workflow.NodeTypeActionHTTPRequest, func(*workflow.NodeContext) (*workflow.NodeResult, error) {
		return &workflow.NodeResult{ErrorKind: workflow.ErrorKindHandled, Output: map[string]interface{}{"success": false, "error": "HTTP 503"}}, nil
	})
	run := h.start(toolWorkflow(
		[]workflow.Node{{ID: "http", Type: workflow.NodeTypeActionHTTPRequest}},
		[]workflow.Edge{{Source: "agent", Target: "http", Label: "buscar_cep"}},
	))
	if run.Status != workflow.RunStatusCompleted || len(agents.returns) != 1 || agents.returns[0].result["error"] != "HTTP 503" {
		t.Fatalf("status = %s returns = %+v", run.Status, agents.returns)
	}
	if step := h.step(t, "http"); step.Status != workflow.LogStatusFailed {
		t.Fatalf("a handled failure at the end of a path must be recorded as failed: %+v", step)
	}
}

func TestTheCallingAgentsOwnFailureStillFailsTheRun(t *testing.T) {
	h := newTraceHarness()
	h.register(workflow.NodeTypeActionAIAgent, func(ctx *workflow.NodeContext) (*workflow.NodeResult, error) {
		ctx.State.Set(workflow.PendingToolCallKey, map[string]interface{}{"name": "x", "source_ai_node": "agent", "target_node": "http"})
		return &workflow.NodeResult{Error: "workflow: tool loop detected", ErrorKind: workflow.ErrorKindGuard}, nil
	})
	run := h.start(toolWorkflow(nil, nil))
	if run.Status != workflow.RunStatusError {
		t.Fatalf("status = %s, the agent's own failure must not loop back to itself", run.Status)
	}
}

func TestAnEdgeDrawnBackToTheAgentKeepsWorking(t *testing.T) {
	agents := &toolAgents{}
	h := toolHarness(agents)
	h.register(workflow.NodeTypeActionHTTPRequest, answering(map[string]interface{}{"status_code": 200}))
	h.register(workflow.NodeTypeActionSetVariable, answering(map[string]interface{}{"saved": true}))
	run := h.start(toolWorkflow(
		[]workflow.Node{{ID: "http", Type: workflow.NodeTypeActionHTTPRequest}, {ID: "save", Type: workflow.NodeTypeActionSetVariable}},
		[]workflow.Edge{{Source: "agent", Target: "http", Label: "buscar_cep"}, {Source: "http", Target: "save"}, {Source: "save", Target: "agent"}},
	))
	if run.Status != workflow.RunStatusCompleted || len(agents.returns) != 1 {
		t.Fatalf("status = %s returns = %+v", run.Status, agents.returns)
	}
	if got := agents.returns[0]; got.result["status_code"] != 200 || got.previous != "save" || got.continuation["saved"] != true {
		t.Fatalf("tool return = %+v", got)
	}
}

func TestANestedAgentsToolReturnsToTheNestedAgent(t *testing.T) {
	agents := &toolAgents{}
	h := toolHarness(agents)
	h.register(workflow.NodeTypeActionHTTPRequest, answering(map[string]interface{}{"found": true}))
	h.register(workflow.NodeTypeActionSetVariable, answering(map[string]interface{}{"saved": true}))
	run := h.start(toolWorkflow(
		[]workflow.Node{
			{ID: "inner", Type: workflow.NodeTypeActionAIAgent},
			{ID: "lookup", Type: workflow.NodeTypeActionHTTPRequest},
			{ID: "after", Type: workflow.NodeTypeActionSetVariable},
		},
		[]workflow.Edge{
			{Source: "agent", Target: "inner", Label: "perguntar"},
			{Source: "inner", Target: "lookup", Label: "buscar"},
			{Source: "inner", Target: "after", Label: "default"},
		},
	))
	if run.Status != workflow.RunStatusCompleted || len(agents.returns) != 1 || agents.returns[0].agent != "inner" || agents.returns[0].result["found"] != true {
		t.Fatalf("status = %s visits = %v returns = %+v", run.Status, agents.visits, agents.returns)
	}
}

func TestAToolPathThroughAWaitReturnsAfterTheContactAnswers(t *testing.T) {
	agents := &toolAgents{}
	h := toolHarness(agents)
	h.register(workflow.NodeTypeWaitForReply, func(ctx *workflow.NodeContext) (*workflow.NodeResult, error) {
		if _, parked := ctx.State.Get("_test_parked"); !parked {
			ctx.State.Set("_test_parked", true)
			return &workflow.NodeResult{Wait: &workflow.WaitInstruction{WakeAt: time.Now().Add(time.Hour).UnixMilli(), Reason: workflow.WaitReasonReply}}, nil
		}
		return &workflow.NodeResult{Output: map[string]interface{}{"reply": "sim"}}, nil
	})
	w := toolWorkflow(
		[]workflow.Node{{ID: "wait", Type: workflow.NodeTypeWaitForReply}},
		[]workflow.Edge{{Source: "agent", Target: "wait", Label: "confirmar"}},
	)
	run := h.start(w)
	if run.Status != workflow.RunStatusWaiting {
		t.Fatalf("status = %s, want the run parked at the wait", run.Status)
	}
	run.Status = workflow.RunStatusRunning
	_ = h.runs.Update(run)
	if err := h.engine.Execute(run, w); err != nil {
		t.Fatal(err)
	}
	if run.Status != workflow.RunStatusCompleted || len(agents.returns) != 1 || agents.returns[0].previous != "wait" {
		t.Fatalf("status = %s returns = %+v, want the agent re-entered right after the wait", run.Status, agents.returns)
	}
}

func TestAnAgentCallingToolsForeverIsStoppedByTheLoopGuard(t *testing.T) {
	agents := &toolAgents{forever: true}
	h := toolHarness(agents)
	h.register(workflow.NodeTypeActionHTTPRequest, answering(map[string]interface{}{"status_code": 200}))
	run := h.start(toolWorkflow(
		[]workflow.Node{{ID: "http", Type: workflow.NodeTypeActionHTTPRequest}},
		[]workflow.Edge{{Source: "agent", Target: "http", Label: "buscar_cep"}},
	))
	if run.Status != workflow.RunStatusError || !strings.Contains(run.Error, "visited") {
		t.Fatalf("status = %s error = %q, want the loop guard to stop it", run.Status, run.Error)
	}
	if calls := len(agents.visits); calls <= maxNodeRevisits {
		t.Fatalf("calls = %d, a tool path must get the agent loop allowance, not the plain node limit", calls)
	}
}

func TestToolsAreNoLongerAModeTheLintKnowsAbout(t *testing.T) {
	catalog := productionCatalog(t)
	def, ok := workflow.NodeCatalogMap(catalog)[workflow.NodeTypeActionAIAgent]
	if !ok {
		t.Fatal("no agent definition")
	}
	for _, f := range def.ConfigSchema {
		if f.Key == "tool_mode" {
			t.Fatalf("tool_mode is still declared: %+v", f)
		}
	}
	if _, declared := def.DefaultConfig["tool_mode"]; declared {
		t.Fatal("tool_mode is still a default")
	}
	if !strings.Contains(def.Guidance.Behavior, workflow.ToolReturnRule()) {
		t.Fatal("the agent spec must state the tool return rule")
	}
}

func TestAnOpenToolLeafIsNotADeadEndForTheLint(t *testing.T) {
	catalog := productionCatalog(t)
	w := toolWorkflow(
		[]workflow.Node{{ID: "save", Type: workflow.NodeTypeActionSetVariable, Config: map[string]interface{}{}}},
		[]workflow.Edge{{Source: "agent", Target: "save", Label: "anotar"}},
	)
	w.Graph.Nodes[1].Config = map[string]interface{}{"custom_tools": []interface{}{map[string]interface{}{"name": "anotar"}}}
	for _, issue := range LintWorkflowGraph(catalog, w).Issues {
		if issue.Code == workflow.LintNodeNoOutgoing {
			t.Fatalf("an open tool leaf returns to the agent, it is not a dead end: %+v", issue)
		}
	}
}

func TestTheBootOutputCatalogMatchesTheProductionCatalog(t *testing.T) {
	outputs := CatalogNodeOutputs()
	catalog := productionCatalog(t)
	for _, def := range catalog {
		node := workflow.Node{ID: "n", Type: def.Type, Config: map[string]interface{}{"custom_tools": []interface{}{map[string]interface{}{"name": "buscar_cep"}}}}
		got, known := outputs(node)
		if !known {
			t.Fatalf("%s has no outputs at boot", def.Type)
		}
		if want := ResolveNodeHandles(catalog, node); len(got) != len(want) {
			t.Fatalf("%s outputs = %+v, want %+v", def.Type, got, want)
		}
	}
	if _, known := outputs(workflow.Node{ID: "n", Type: workflow.NodeTypeActionRunTool}); known {
		t.Fatal("a type without an executor must stay unknown so the repair refuses its graph")
	}
}
