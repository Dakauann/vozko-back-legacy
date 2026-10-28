package workflow

import (
	"errors"
	"reflect"
	"testing"
)

func recordedRun() *WorkflowRun {
	state := NewRunState()
	state.Set(DataKeyCampaignVars, map[string]interface{}{"matricula": "123", "valor_pendencia": "R$ 10"})
	state.Set("message", "oi")
	state.Set("_last_observacoes", "da etapa que falhou")
	state.Set("_node_n8_error", "registrada depois")
	state.Set(StateKeyGateAttempts, 3)
	return &WorkflowRun{ID: "run-1", EntryID: "conv-1", EntryType: "whatsapp", State: state}
}

func recordedSteps() []*WorkflowRunLog {
	return []*WorkflowRunLog{
		{ID: "s0", NodeID: "t", NodeType: NodeTypeTriggerFirstMessage, Status: LogStatusExecuted, Output: map[string]interface{}{"message": "oi"}},
		{ID: "s1", NodeID: "n1", Status: LogStatusExecuted, Output: map[string]interface{}{"observacoes": "cliente pediu PIX"}},
		{ID: "s2", NodeID: "n2", Status: LogStatusFailed, Output: map[string]interface{}{"observacoes": "falhou"}},
		{ID: "s8", NodeID: "n8", Status: LogStatusExecuted, Output: map[string]interface{}{"error": "não exportada"}},
	}
}

func TestRecordedTestStateIsTheRunStateAsTheStepSawIt(t *testing.T) {
	state, err := RecordedTestState(recordedRun(), recordedSteps(), "n8", "s8")
	if err != nil {
		t.Fatalf("RecordedTestState: %v", err)
	}
	campvars, _ := state.Get(DataKeyCampaignVars)
	if !reflect.DeepEqual(campvars, map[string]interface{}{"matricula": "123", "valor_pendencia": "R$ 10"}) {
		t.Fatalf("campvars = %v, want the run's campaign values", campvars)
	}
	if got, _ := ResolveVariable("node.n1.observacoes", &state); got != "cliente pediu PIX" {
		t.Fatalf("node.n1.observacoes = %v, want the output recorded before the step", got)
	}
	if got, _ := ResolveVariable("last.observacoes", &state); got != "cliente pediu PIX" {
		t.Fatalf("last.observacoes = %v, want the last finished output before the step, not a later or failed one", got)
	}
	if _, ok := state.Get("_node_n8_error"); ok {
		t.Fatal("the step's own output is not part of what it saw")
	}
	if _, ok := state.Get(StateKeyGateAttempts); ok {
		t.Fatal("engine bookkeeping is not carried into a test")
	}
}

func TestRecordedTestStateRefusesAStepOfAnotherNode(t *testing.T) {
	cases := []struct{ name, node, step string }{
		{"a step that is not in the run", "n8", "nope"},
		{"a step of another node", "n2", "s8"},
		{"no step at all", "n8", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := RecordedTestState(recordedRun(), recordedSteps(), tc.node, tc.step); !errors.Is(err, ErrRecordedStepMissing) {
				t.Fatalf("err = %v, want ErrRecordedStepMissing", err)
			}
		})
	}
	if _, err := RecordedTestState(nil, recordedSteps(), "n8", "s8"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("err = %v, want ErrRunNotFound without a run", err)
	}
}

func TestTestSeedPickReadsTheStepAndWhatRanBeforeIt(t *testing.T) {
	if got := TestSeedPick(" s8 "); got.StepID != "s8" || !got.UpTo || got.Trigger || len(got.LatestOfNodes) > 0 {
		t.Fatalf("pick = %+v", got)
	}
}
