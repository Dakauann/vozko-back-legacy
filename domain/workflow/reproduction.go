package workflow

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"vozko/domain/redact"
	"vozko/domain/tracepayload"
)

var ErrNoTriggerData = errors.New("workflow: this run kept no trigger data (it started before trigger data was stored and its trigger step has no output), so it cannot be reproduced")

func TriggerDataOf(state RunState) map[string]interface{} {
	start := userVars(state)
	if len(start) == 0 {
		return nil
	}
	return tracepayload.Map(redact.New().Map(start))
}

func userVars(state RunState) map[string]interface{} {
	out := make(map[string]interface{}, len(state.Vars))
	for k, v := range state.Vars {
		if strings.HasPrefix(k, "_") {
			continue
		}
		out[k] = v
	}
	return out
}

var ErrRecordedStepMissing = fmt.Errorf("%w: the run has no recorded step of this node with that id", ErrRunNotFound)

func RecordedTestState(run *WorkflowRun, steps []*WorkflowRunLog, nodeID, stepID string) (RunState, error) {
	if run == nil {
		return RunState{}, ErrRunNotFound
	}
	target := recordedStepIndex(steps, nodeID, stepID)
	if target < 0 {
		return RunState{}, fmt.Errorf("%w: node %s, step %q in run %s", ErrRecordedStepMissing, nodeID, stepID, run.ID)
	}
	state := RunState{Vars: cloneConfigMap(userVars(run.State))}
	for _, step := range steps[:target] {
		if step.Pinnable() {
			RecordNodeOutput(&state, step.NodeID, cloneConfigMap(step.Output))
		}
	}
	return state, nil
}

func recordedStepIndex(steps []*WorkflowRunLog, nodeID, stepID string) int {
	if stepID == "" {
		return -1
	}
	for i, step := range steps {
		if step != nil && step.ID == stepID && step.NodeID == nodeID {
			return i
		}
	}
	return -1
}

func ReproductionSeed(run *WorkflowRun, steps []*WorkflowRunLog) (map[string]interface{}, error) {
	if run != nil && len(run.TriggerData) > 0 {
		return cloneConfigMap(run.TriggerData), nil
	}
	for _, step := range steps {
		if step != nil && step.NodeType.IsTrigger() && len(step.Output) > 0 {
			return cloneConfigMap(step.Output), nil
		}
	}
	return nil, ErrNoTriggerData
}

func replayedFromRun(n Node, behaviors TestBehaviors) bool {
	return n.Type == NodeTypeActionHTTPRequest || n.Type == NodeTypeActionCode || SideEffectNode(n, behaviors)
}

func ReproductionPins(draft *Graph, steps []*WorkflowRunLog, runID string, at time.Time, behaviors TestBehaviors) map[string]NodePin {
	replayed := make(map[string]bool)
	for _, id := range replayedNodeIDs(draft, behaviors) {
		replayed[id] = true
	}
	pins := make(map[string]NodePin)
	for _, step := range steps {
		if step == nil || !replayed[step.NodeID] {
			continue
		}
		if pin, err := PinFromStep(step, runID, at); err == nil {
			pins[step.NodeID] = pin
		}
	}
	return pins
}
