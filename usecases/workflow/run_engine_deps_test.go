package workflow_usecase

import (
	"errors"
	"testing"

	"vozko/domain/shared/deps"
	"vozko/domain/workflow"
)

func testEngine(runs workflow.WorkflowRunRepository, logs workflow.WorkflowRunLogRepository, registry *NodeExecutorRegistry) *RunEngine {
	return &RunEngine{
		runRepo:        runs,
		logRepo:        logs,
		registry:       registry,
		wakeScheduler:  sandboxRuntime{},
		runLocker:      sandboxRuntime{},
		automationGate: SandboxAutomationGate{},
		metrics:        sandboxRuntime{},
		statuses:       sandboxRuntime{},
		agentRunEnds:   sandboxRuntime{},
		alerts:         sandboxRuntime{},
		gate:           workflow.PassThroughStepGate{},
		choices:        sandboxRuntime{},
	}
}

func TestTheRunEngineRefusesEveryMissingDependency(t *testing.T) {
	engine, err := NewRunEngine(RunEngineDeps{})
	var missing *deps.MissingError
	if engine != nil || !errors.As(err, &missing) || len(missing.Missing) != 12 {
		t.Fatalf("engine = %v err = %v, want every dependency refused", engine, err)
	}
}
