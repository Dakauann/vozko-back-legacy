package workflow_usecase

import (
	"testing"
	"time"

	"vozko/domain/stage"
	"vozko/domain/workflow"
)

type parkedStageRun struct {
	startedRuns
	parked *workflow.WorkflowRun
}

func (r *parkedStageRun) FindActiveByEntryAndTrigger(string, string, string) (*workflow.WorkflowRun, error) {
	return r.parked, nil
}

func waitingStageWorkflow() *workflow.Workflow {
	version := 1
	return &workflow.Workflow{
		ID:           "wf-wait",
		WorkspaceID:  "ws1",
		GraphVersion: &version,
		Graph: workflow.Graph{
			Nodes: []workflow.Node{
				{ID: "etapa", Type: workflow.NodeTypeTriggerStageAdded},
				{ID: "espera", Type: workflow.NodeTypeWaitDuration},
				{ID: "respondeu", Type: workflow.NodeTypeEnd},
				{ID: "passou", Type: workflow.NodeTypeEnd},
			},
			Edges: []workflow.Edge{
				{Source: "etapa", Target: "espera"},
				{Source: "espera", Target: "respondeu", Label: "message_received"},
				{Source: "espera", Target: "passou", Label: "timeout"},
			},
		},
	}
}

func stageRunParkedOn(reason workflow.WaitReason) *workflow.WorkflowRun {
	version := 1
	run := &workflow.WorkflowRun{
		ID: "run-parked", WorkflowID: "wf-wait", WorkspaceID: "ws1", EntryID: "conv-1", EntryType: "whatsapp",
		TriggerNodeID: "etapa", CurrentNodeID: "espera", TriggerType: workflow.TriggerStageAdded,
		State: workflow.NewRunState(), WorkflowVersion: &version,
	}
	run.SetWaiting(time.Now().Add(time.Hour).UnixMilli(), reason)
	return run
}

func TestAStageMoveNeverWakesAParkedRunAsIfTheCustomerHadReplied(t *testing.T) {
	for _, reason := range []workflow.WaitReason{workflow.WaitReasonDuration, workflow.WaitReasonReply} {
		t.Run(string(reason), func(t *testing.T) {
			parked := stageRunParkedOn(reason)
			runs := &parkedStageRun{parked: parked}
			wf := waitingStageWorkflow()
			evaluator := NewTriggerEvaluator(stubWorkflowRepo{byID: map[string]*workflow.Workflow{wf.ID: wf}, active: []*workflow.Workflow{wf}}, runs, testEngine(runs, stubRunLogRepo{}, NewNodeExecutorRegistry()), nil, responsibleIs("wf-wait"))
			f := startStageConsumer(t, evaluator)

			f.deliver(t, publishedMove(t, entered(stage.MoveCause{})))

			if parked.CurrentNodeID != "espera" || parked.Status != workflow.RunStatusWaiting {
				t.Fatalf("run moved to %q with status %s: a stage move is not a reply", parked.CurrentNodeID, parked.Status)
			}
			if _, woke := parked.State.Get("_wait_outcome"); woke {
				t.Fatal("the stage move was recorded as the wait outcome")
			}
			if runs.updates != 0 || len(runs.runs) != 0 {
				t.Fatalf("updates %d, new runs %d: the parked run keeps the conversation", runs.updates, len(runs.runs))
			}
		})
	}
}
