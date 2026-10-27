package workflow_usecase

import (
	"errors"
	"testing"

	"vozko/domain/shared"
	"vozko/domain/workflow"
)

type conversationOwnerFlow struct {
	id    string
	err   error
	asked []string
}

func (r *conversationOwnerFlow) ResponsibleWorkflow(workspaceID, entryID, entryType string) (string, bool, error) {
	r.asked = append(r.asked, workspaceID+"|"+entryID+"|"+entryType)
	if r.err != nil {
		return "", false, r.err
	}
	return r.id, r.id != "", nil
}

func responsibleIs(id string) *conversationOwnerFlow { return &conversationOwnerFlow{id: id} }

func scopedEvaluator(runs *startedRuns, responsible workflow.ConversationResponsibility, workflows ...*workflow.Workflow) workflow.CheckedTriggerEvaluator {
	byID := map[string]*workflow.Workflow{}
	for _, w := range workflows {
		byID[w.ID] = w
	}
	return NewTriggerEvaluator(stubWorkflowRepo{byID: byID, active: workflows}, runs, testEngine(runs, stubRunLogRepo{}, NewNodeExecutorRegistry()), nil, responsible)
}

func stageMove(entryID, entryType string) workflow.TriggerEvent {
	event, _ := workflow.StageAddedEvent(workflow.StageEntry{MoveID: "move-" + entryID, WorkspaceID: "ws1", EntryID: entryID, EntryType: entryType, StageID: "s1"})
	return event
}

func formOneWorkflow(id string) *workflow.Workflow {
	return &workflow.Workflow{
		ID:          id,
		WorkspaceID: "ws1",
		Graph: workflow.Graph{
			Nodes: []workflow.Node{{ID: "form", Type: workflow.NodeTypeTriggerFormSubmitted, Config: map[string]interface{}{workflow.FormTriggerFormIDsKey: []interface{}{"form-1"}}}, {ID: "end", Type: workflow.NodeTypeEnd}},
			Edges: []workflow.Edge{{Source: "form", Target: "end"}},
		},
	}
}

func startedWorkflows(runs *startedRuns) []string {
	var ids []string
	for _, run := range runs.runs {
		ids = append(ids, run.WorkflowID)
	}
	return ids
}

func TestAStageMoveStartsOnlyTheWorkflowResponsibleForTheConversation(t *testing.T) {
	runs := &startedRuns{}
	owner := responsibleIs("wf-campaign-a")
	evaluator := scopedEvaluator(runs, owner, stageWorkflow("wf-campaign-a", nil), stageWorkflow("wf-campaign-b", map[string]interface{}{workflow.DataKeyStageID: "s1"}))

	if err := evaluator.EvaluateChecked(stageMove("conv-1", "whatsapp")); err != nil {
		t.Fatal(err)
	}

	if got := startedWorkflows(runs); len(got) != 1 || got[0] != "wf-campaign-a" {
		t.Fatalf("started %v, want only the conversation's own workflow; the other operation shares the stage but not the conversation", got)
	}
	if len(owner.asked) != 1 || owner.asked[0] != "ws1|conv-1|whatsapp" {
		t.Fatalf("asked %v, want the conversation reference carried by the move", owner.asked)
	}
}

func TestAStageMoveStartsNothingWhenNoWorkflowIsResponsible(t *testing.T) {
	runs := &startedRuns{}
	evaluator := scopedEvaluator(runs, responsibleIs(""), stageWorkflow("wf-unlinked", nil))

	if err := evaluator.EvaluateChecked(stageMove("conv-1", "instagram")); err != nil {
		t.Fatal(err)
	}

	if len(runs.runs) != 0 {
		t.Fatalf("started %v, want nothing: a workflow no number or campaign uses does not react to its stages", startedWorkflows(runs))
	}
}

func TestAStageMoveWhoseResponsibilityCannotBeReadStartsNothingAndIsRetried(t *testing.T) {
	runs := &startedRuns{}
	evaluator := scopedEvaluator(runs, &conversationOwnerFlow{err: errors.New("db down")}, stageWorkflow("wf-a", nil))

	if err := evaluator.EvaluateChecked(stageMove("conv-1", "telegram")); err == nil {
		t.Fatal("the responsible workflow was not read, so the caller must learn it failed")
	}
	if len(runs.runs) != 0 {
		t.Fatalf("started %v, want nothing", startedWorkflows(runs))
	}
}

func TestTheResponsibleWorkflowDoesNotRestartFromItsOwnMove(t *testing.T) {
	runs := &startedRuns{}
	evaluator := scopedEvaluator(runs, responsibleIs("wf-a"), stageWorkflow("wf-a", nil))
	event, _ := workflow.StageAddedEvent(workflow.StageEntry{MoveID: "m", WorkspaceID: "ws1", EntryID: "conv-1", EntryType: "whatsapp", StageID: "s1", ByWorkflowID: "wf-a"})

	if err := evaluator.EvaluateChecked(event); err != nil {
		t.Fatal(err)
	}
	if len(runs.runs) != 0 {
		t.Fatalf("started %v, want the loop guard to keep wf-a from restarting on its own move", startedWorkflows(runs))
	}
}

func TestAStageMoveOnEveryChannelAsksForThatConversationsWorkflow(t *testing.T) {
	for _, entryType := range shared.KnownEntryTypes() {
		t.Run(string(entryType), func(t *testing.T) {
			runs := &startedRuns{}
			owner := responsibleIs("wf-own")
			evaluator := scopedEvaluator(runs, owner, stageWorkflow("wf-own", nil), stageWorkflow("wf-other", nil))

			if err := evaluator.EvaluateChecked(stageMove("conv-9", string(entryType))); err != nil {
				t.Fatal(err)
			}

			if got := startedWorkflows(runs); len(got) != 1 || got[0] != "wf-own" {
				t.Fatalf("started %v, want only wf-own", got)
			}
			if len(owner.asked) != 1 || owner.asked[0] != "ws1|conv-9|"+string(entryType) {
				t.Fatalf("asked %v", owner.asked)
			}
		})
	}
}

func TestAGlobalTriggerKeepsStartingEveryMatchingWorkflow(t *testing.T) {
	runs := &startedRuns{}
	owner := responsibleIs("wf-a")
	evaluator := scopedEvaluator(runs, owner, formOneWorkflow("wf-a"), formOneWorkflow("wf-b"))

	err := evaluator.EvaluateChecked(workflow.TriggerEvent{
		WorkspaceID: "ws1", EntryID: "conv-1", EntryType: "whatsapp", TriggerType: workflow.TriggerFormSubmitted,
		Data: workflow.FormSubmissionData(nil, "form-1", "resp-1"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := startedWorkflows(runs); len(got) != 2 {
		t.Fatalf("started %v, want both: a form trigger is chosen by its form, not by the conversation's workflow", got)
	}
	if len(owner.asked) != 0 {
		t.Fatalf("asked %v: a global trigger never looks up the conversation's workflow", owner.asked)
	}
}

func TestATriggerWithNoKnownScopeStartsNothing(t *testing.T) {
	runs := &startedRuns{}
	owner := responsibleIs("wf-a")
	odd := &workflow.Workflow{ID: "wf-a", WorkspaceID: "ws1", Graph: workflow.Graph{
		Nodes: []workflow.Node{{ID: "t", Type: "trigger_lead_created"}, {ID: "end", Type: workflow.NodeTypeEnd}},
		Edges: []workflow.Edge{{Source: "t", Target: "end"}},
	}}
	evaluator := scopedEvaluator(runs, owner, odd)

	if err := evaluator.EvaluateChecked(workflow.TriggerEvent{WorkspaceID: "ws1", EntryID: "conv-1", EntryType: "whatsapp", TriggerType: "trigger_lead_created"}); err != nil {
		t.Fatalf("an unknown trigger is refused, not retried: %v", err)
	}
	if len(runs.runs) != 0 || len(owner.asked) != 0 {
		t.Fatalf("started %v, asked %v: an unclassified trigger fails closed", startedWorkflows(runs), owner.asked)
	}
}

func messageWorkflow(id string) *workflow.Workflow {
	return &workflow.Workflow{
		ID:          id,
		WorkspaceID: "ws1",
		Graph: workflow.Graph{
			Nodes: []workflow.Node{{ID: "msg", Type: workflow.NodeTypeTriggerMessageReceived}, {ID: "end", Type: workflow.NodeTypeEnd}},
			Edges: []workflow.Edge{{Source: "msg", Target: "end"}},
		},
	}
}

func TestAMessageNamingItsWorkflowStartsOnlyThatOneWithoutAnotherLookup(t *testing.T) {
	runs := &startedRuns{}
	owner := responsibleIs("wf-stored")
	evaluator := scopedEvaluator(runs, owner, messageWorkflow("wf-named"), messageWorkflow("wf-stored"))
	event := workflow.InboundTrigger{WorkspaceID: "ws1", EntryID: "conv-1", EntryType: "telegram", Text: "oi", WorkflowID: "wf-named"}.Event()
	event.Reply = &workflow.ReplyRouting{}

	if err := evaluator.EvaluateChecked(event); err != nil {
		t.Fatal(err)
	}

	if got := startedWorkflows(runs); len(got) != 1 || got[0] != "wf-named" {
		t.Fatalf("started %v, want the workflow the inbound path resolved", got)
	}
	if len(owner.asked) != 0 {
		t.Fatalf("asked %v: the inbound path already resolved the conversation's workflow", owner.asked)
	}
}

func TestAMessageNamingNoWorkflowAsksTheConversationInsteadOfStartingEveryWorkflow(t *testing.T) {
	cases := []struct {
		name        string
		responsible string
		want        []string
	}{
		{"the conversation has its own workflow", "wf-b", []string{"wf-b"}},
		{"the conversation has none", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runs := &startedRuns{}
			owner := responsibleIs(tc.responsible)
			evaluator := scopedEvaluator(runs, owner, messageWorkflow("wf-a"), messageWorkflow("wf-b"))
			event := workflow.InboundTrigger{WorkspaceID: "ws1", EntryID: "conv-1", EntryType: "whatsapp", Text: "oi"}.Event()
			event.Reply = &workflow.ReplyRouting{}

			if err := evaluator.EvaluateChecked(event); err != nil {
				t.Fatal(err)
			}

			got := startedWorkflows(runs)
			if len(got) != len(tc.want) || (len(got) == 1 && got[0] != tc.want[0]) {
				t.Fatalf("started %v, want %v", got, tc.want)
			}
			if len(owner.asked) != 1 {
				t.Fatalf("asked %v, want one lookup", owner.asked)
			}
		})
	}
}

func TestTheEvaluatorRefusesToBuildWithoutAResponsibilityResolver(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("an evaluator that cannot scope conversation triggers must not be built")
		}
	}()
	runs := &startedRuns{}
	NewTriggerEvaluator(stubWorkflowRepo{}, runs, testEngine(runs, stubRunLogRepo{}, NewNodeExecutorRegistry()), nil, nil)
}
