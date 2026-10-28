package workflow

import (
	"slices"
	"sort"
	"strings"

	"vozko/domain/shared"
)

type StepPick struct {
	StepID        string
	UpTo          bool
	LatestOfNodes []string
	Trigger       bool
}

type RunStepPicker interface {
	PickSteps(runID string, pick StepPick) ([]*WorkflowRunLog, error)
}

type RunStepPicks interface {
	PickSteps(viewer shared.Person, workspaceID, runID string, pick StepPick) (*RunDetail, error)
}

func PinPick(nodeID, stepID string) StepPick {
	if stepID = strings.TrimSpace(stepID); stepID != "" {
		return StepPick{StepID: stepID}
	}
	return StepPick{LatestOfNodes: []string{nodeID}}
}

func TestSeedPick(stepID string) StepPick {
	return StepPick{StepID: strings.TrimSpace(stepID), UpTo: true}
}

func ReproductionPick(draft *Graph, behaviors TestBehaviors) StepPick {
	return StepPick{LatestOfNodes: replayedNodeIDs(draft, behaviors), Trigger: true}
}

func replayedNodeIDs(draft *Graph, behaviors TestBehaviors) []string {
	var ids []string
	if draft == nil {
		return ids
	}
	for _, n := range draft.Nodes {
		if pinnable(n.Type) && replayedFromRun(n, behaviors) {
			ids = append(ids, n.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

var pinnableStepStatuses = []LogStatus{LogStatusExecuted, LogStatusSkipped}

func PinnableStepStatuses() []LogStatus {
	return slices.Clone(pinnableStepStatuses)
}
