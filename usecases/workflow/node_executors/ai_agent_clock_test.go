package node_executors

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/workflow"
)

type nodeWorkspaceZones struct {
	loc   *time.Location
	err   error
	asked []string
}

func (z *nodeWorkspaceZones) Location(_ context.Context, workspaceID, _ string) (*time.Location, error) {
	z.asked = append(z.asked, workspaceID)
	return z.loc, z.err
}

func TestASavedAgentInTheNodeSpeaksTheWorkspaceTime(t *testing.T) {
	recife, err := time.LoadLocation("America/Recife")
	if err != nil {
		t.Fatal(err)
	}
	zones := &nodeWorkspaceZones{loc: recife}
	service := &recordingAgentAI{}
	executor := NewAIAgentExecutor(service, publishedAgentStore{}, &entryTranscript{history: agentNodeTranscript(2)}, nil, nil, nil, quietAttendance{}, zones)
	state := workflow.NewRunState()

	if _, err := executor.Execute(agentNodeContext(&state, map[string]interface{}{"source": "agent", "agent_id": "ag-9"})); err != nil {
		t.Fatal(err)
	}

	if len(service.inputs) != 1 || service.inputs[0].Zone != recife {
		t.Fatalf("inputs = %d, want one call in America/Recife", len(service.inputs))
	}
	if len(zones.asked) != 1 || zones.asked[0] != "ws-1" {
		t.Fatalf("zone lookups = %v, want one for ws-1", zones.asked)
	}
}

func TestTheNodeFallsBackToUTCWhenTheWorkspaceZoneIsUnreadable(t *testing.T) {
	service := &recordingAgentAI{}
	executor := NewAIAgentExecutor(service, publishedAgentStore{}, &entryTranscript{history: agentNodeTranscript(2)}, nil, nil, nil, quietAttendance{}, &nodeWorkspaceZones{err: errors.New("config down")})
	state := workflow.NewRunState()

	if _, err := executor.Execute(agentNodeContext(&state, map[string]interface{}{"source": "agent", "agent_id": "ag-9"})); err != nil {
		t.Fatal(err)
	}

	if len(service.inputs) != 1 || service.inputs[0].Zone != time.UTC {
		t.Fatalf("want one call labeled UTC, got %d calls", len(service.inputs))
	}
}
