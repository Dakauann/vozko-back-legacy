package node_executors

import (
	"strings"
	"testing"

	"vozko/domain/ai"
	"vozko/domain/workflow"
)

func toolMessageOf(input ai.GenerateInput) *ai.Message {
	for i := range input.Messages {
		if input.Messages[i].Role == ai.RoleTool {
			return &input.Messages[i]
		}
	}
	return nil
}

func TestAFailedToolStepReachesTheAgentAsTheToolResult(t *testing.T) {
	service := &toolRoundTripAI{}
	executor := NewAIAgentExecutor(service, nil, &entryTranscript{history: agentNodeTranscript(2)}, nil, nil, nil, quietAttendance{}, nil)
	state := workflow.NewRunState()
	ctx := toolAgentContext(&state)
	if first, err := executor.Execute(ctx); err != nil || first.NextNodeID != "calendar" {
		t.Fatalf("tool call: %+v %v", first, err)
	}
	workflow.RecordNodeOutput(ctx.State, "calendar", map[string]interface{}{"success": false, "error": "a agenda do Google recusou o acesso"})
	ctx.State.Set("_prev_node_id", "calendar")
	if _, err := executor.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	tool := toolMessageOf(service.inputs[1])
	if tool == nil || !strings.Contains(tool.Content, "a agenda do Google recusou o acesso") {
		t.Fatalf("the agent must read the failure as the tool result: %+v", service.inputs[1].Messages)
	}
}

func TestAToolPathEndingOnAWaitStartsAFreshTurn(t *testing.T) {
	service := &toolRoundTripAI{}
	executor := NewAIAgentExecutor(service, nil, &entryTranscript{history: agentNodeTranscript(2)}, nil, nil, nil, quietAttendance{}, nil)
	state := workflow.NewRunState()
	ctx := toolAgentContext(&state)
	if first, err := executor.Execute(ctx); err != nil || first.NextNodeID != "calendar" {
		t.Fatalf("tool call: %+v %v", first, err)
	}
	workflow.RecordNodeOutput(ctx.State, "wait", map[string]interface{}{"reply": "pode ser amanhã"})
	ctx.State.Set("_prev_node_id", "wait")
	if _, err := executor.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if tool := toolMessageOf(service.inputs[1]); tool != nil {
		t.Fatalf("after the contact answers, the old tool call is dropped: %+v", tool)
	}
	if history, err := readToolHistory(ctx); err != nil || len(history) != 1 {
		t.Fatalf("the tool history must restart with the new turn: %+v %v", history, err)
	}
}
