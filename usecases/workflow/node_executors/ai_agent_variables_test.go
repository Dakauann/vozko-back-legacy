package node_executors

import (
	"testing"

	"vozko/domain/agent"
	"vozko/domain/workflow"
)

type promptedAgentStore struct {
	agent.Repository
	prompt string
}

func (s promptedAgentStore) FindByID(id string) (*agent.Agent, error) {
	return &agent.Agent{ID: id, WorkspaceID: "ws-1", Name: "Escola", MessagingPrompt: s.prompt, MessagingModel: "openai/gpt-4o-mini"}, nil
}

func TestASavedAgentMissingAVariableTakesTheErrorBranchInsteadOfBlanks(t *testing.T) {
	service := &recordingAgentAI{}
	executor := NewAIAgentExecutor(service, promptedAgentStore{prompt: "Turma {{turma}}"}, &entryTranscript{history: agentNodeTranscript(2)}, nil, nil, nil, quietAttendance{}, nil)
	state := workflow.NewRunState()
	state.Set(workflow.DataKeyCampaignVars, map[string]interface{}{"escola": "Prisma"})

	result, err := executor.Execute(agentNodeContext(&state, map[string]interface{}{"source": "agent", "agent_id": "ag-9"}))
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Error == "" || len(service.inputs) != 0 {
		t.Fatalf("result = %+v, model calls %d; want the error branch and no call", result, len(service.inputs))
	}
}

func TestASavedAgentWithEveryVariableRunsWithTheValues(t *testing.T) {
	service := &recordingAgentAI{}
	executor := NewAIAgentExecutor(service, promptedAgentStore{prompt: "Turma {{turma}}"}, &entryTranscript{history: agentNodeTranscript(2)}, nil, nil, nil, quietAttendance{}, nil)
	state := workflow.NewRunState()
	state.Set(workflow.DataKeyCampaignVars, map[string]interface{}{"turma": "B"})

	if _, err := executor.Execute(agentNodeContext(&state, map[string]interface{}{"source": "agent", "agent_id": "ag-9"})); err != nil {
		t.Fatal(err)
	}
	if len(service.inputs) != 1 {
		t.Fatalf("model calls = %d", len(service.inputs))
	}
}
