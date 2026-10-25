package agent

import (
	"context"
	"errors"

	"vozko/domain/tools"
)

const MaxSimulationHistory = 60

var (
	ErrSimulationMessageRequired = errors.New("agent simulation: message is required")
	ErrSimulationHistoryTooLong  = errors.New("agent simulation: history is too long")
	ErrSimulationMemoriesTooLong = errors.New("agent simulation: too many session memories")
)

type SimulationRole string

const (
	SimulationRoleUser      SimulationRole = "user"
	SimulationRoleAssistant SimulationRole = "assistant"
)

type SimulationMessage struct {
	Role    SimulationRole
	Content string
}

type SessionMemory struct {
	ID       string
	Content  string
	Category string
}

type SimulatedToolCall struct {
	ToolID    string
	Name      string
	Arguments map[string]interface{}
	Result    string
	Output    interface{}
	IsError   bool
	Stubbed   bool
	Mocked    bool
}

type SimulationDebug struct {
	Model            string
	SystemPrompt     string
	ToolNames        []string
	MemoryInjected   bool
	RAGInjected      bool
	PromptTokens     int
	CompletionTokens int
	FinishReason     string
}

type SimulateTurnInput struct {
	WorkspaceID string
	AgentID     string

	History []SimulationMessage
	Message string

	LeadID string

	SessionMemories []SessionMemory

	Scenario bool
	RunID    string
	Mocks    map[string]tools.Mock
}

type SimulateTurnOutput struct {
	RunID     string
	Replies   []string
	ToolCalls []SimulatedToolCall
	Debug     SimulationDebug
}

type SimulateTurnUseCase interface {
	Execute(ctx context.Context, in SimulateTurnInput) (*SimulateTurnOutput, error)
}
