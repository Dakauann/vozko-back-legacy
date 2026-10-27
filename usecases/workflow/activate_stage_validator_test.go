package workflow_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/stage"
	"vozko/domain/workflow"
	"vozko/usecases/workflow/node_executors"
)

type stageLookupStub map[string]*stage.Stage

func (s stageLookupStub) FindByID(id string) (*stage.Stage, error) {
	if st, ok := s[id]; ok {
		return st, nil
	}
	return nil, stage.ErrTagNotFound
}

type destinationsStub struct {
	err   error
	reads int
}

func (d *destinationsStub) Destination(workspaceID, stageID string) (stage.FunnelStage, error) {
	d.reads++
	if d.err != nil {
		return stage.FunnelStage{}, d.err
	}
	switch {
	case workspaceID != "ws1":
		return stage.FunnelStage{}, stage.ErrUnauthorized
	case stageID == "st-mine":
		return stage.FunnelStage{StageID: stageID, StageName: "pagamento efetuado", PipelineID: "pl-altiva", PipelineName: "Altiva"}, nil
	case stageID == "st-legacy":
		return stage.FunnelStage{StageID: stageID, StageName: "recebido"}, nil
	case stageID == "st-deal":
		return stage.FunnelStage{}, stage.ErrStageNotConversationFunnel
	case stageID == "st-other":
		return stage.FunnelStage{}, stage.ErrUnauthorized
	}
	return stage.FunnelStage{}, stage.ErrTagNotFound
}

func TestActivationRefusesAStageOutsideTheWorkspace(t *testing.T) {
	v := &stageValidator{repo: stageLookupStub{
		"st-mine":  {ID: "st-mine", WorkspaceID: "ws1"},
		"st-other": {ID: "st-other", WorkspaceID: "ws2"},
	}, destinations: &destinationsStub{}, workspaceID: "ws1"}

	for _, nodeType := range []workflow.NodeType{workflow.NodeTypeActionMoveStage, workflow.NodeTypeConditionCheckStage} {
		for stageID, wantErr := range map[string]bool{"st-mine": false, "st-other": true, "st-gone": true, "": false} {
			err := v.Validate(&workflow.Node{ID: "n1", Type: nodeType, Config: map[string]interface{}{"stage_id": stageID}})
			if got := errors.Is(err, workflow.ErrNodeInvalidStageID); got != wantErr {
				t.Errorf("%s with stage %q: err = %v, want refused = %v", nodeType, stageID, err, wantErr)
			}
		}
	}
}

func TestActivationRefusesMovingAConversationOntoAnOpportunityStage(t *testing.T) {
	v := &stageValidator{repo: stageLookupStub{}, destinations: &destinationsStub{}, workspaceID: "ws1"}
	err := v.Validate(&workflow.Node{ID: "n1", Type: workflow.NodeTypeActionMoveStage, Config: map[string]interface{}{"stage_id": "st-deal"}})
	if !errors.Is(err, workflow.ErrNodeStageNotConversationFunnel) || !workflow.IsValidationError(err) {
		t.Fatalf("err = %v, want the opportunity stage refused as a validation problem", err)
	}
	if workflow.IssueOf(err).Code != workflow.LintInvalidResourceRef {
		t.Fatalf("issue = %+v, want an invalid resource reference", workflow.IssueOf(err))
	}
}

func TestActivationCannotPassAMoveWhoseDestinationCannotBeRead(t *testing.T) {
	for name, v := range map[string]*stageValidator{
		"unreadable":      {repo: stageLookupStub{}, destinations: &destinationsStub{err: errors.New("db down")}, workspaceID: "ws1"},
		"no destinations": {repo: stageLookupStub{}, workspaceID: "ws1"},
	} {
		t.Run(name, func(t *testing.T) {
			err := v.Validate(&workflow.Node{ID: "n1", Type: workflow.NodeTypeActionMoveStage, Config: map[string]interface{}{"stage_id": "st-mine"}})
			if err == nil || workflow.IsValidationError(err) {
				t.Fatalf("err = %v, want an unverifiable destination to stop validation", err)
			}
		})
	}
}

func stageCatalog() []workflow.NodeDefinition {
	return []workflow.NodeDefinition{node_executors.NewMoveStageExecutor(nil, nil, nil).Definition()}
}

func moveStageDraft(stageID string) *workflow.Workflow {
	return &workflow.Workflow{WorkspaceID: "ws1", Type: workflow.WorkflowTypeMessages, Graph: workflow.Graph{Nodes: []workflow.Node{
		{ID: "pago", Type: workflow.NodeTypeActionMoveStage, Config: map[string]interface{}{"stage_id": stageID}},
	}}}
}

func stageNotes(issues []workflow.LintIssue) []workflow.LintIssue {
	var out []workflow.LintIssue
	for _, i := range issues {
		if i.Code == workflow.LintStageDestination {
			out = append(out, i)
		}
	}
	return out
}

func TestTheLintNamesTheFunnelAMoveStageSendsConversationsTo(t *testing.T) {
	uc := &activateWorkflowUseCase{repo: NewMockWorkflowRepository(), catalogFn: stageCatalog, stageRepo: stageLookupStub{}, stageDestinations: &destinationsStub{}}
	report, err := uc.Lint(context.Background(), moveStageDraft("st-mine"))
	if err != nil {
		t.Fatal(err)
	}
	notes := stageNotes(report.Issues)
	if len(notes) != 1 {
		t.Fatalf("issues = %+v, want one destination note", report.Issues)
	}
	note := notes[0]
	if note.Severity != workflow.SeverityInfo || note.NodeID != "pago" || note.Field != "stage_id" {
		t.Fatalf("note = %+v, want a non-blocking note on the stage field", note)
	}
	if note.Values["pipeline"] != "Altiva" || note.Values["stage"] != "pagamento efetuado" || note.Values["stage_id"] != "st-mine" {
		t.Fatalf("values = %v, want the funnel and stage named by the backend", note.Values)
	}
	if note.Message != "Conversas de outro funil vão para o funil Altiva, na etapa pagamento efetuado" {
		t.Fatalf("message = %q", note.Message)
	}
	for _, blocker := range report.Blocking() {
		if blocker.Code == workflow.LintStageDestination {
			t.Fatalf("a destination note must never block publishing: %+v", blocker)
		}
	}
}

func TestTheLintAddsNoFunnelNoteWithoutAFunnel(t *testing.T) {
	for _, stageID := range []string{"st-legacy", "st-deal", "st-gone"} {
		uc := &activateWorkflowUseCase{repo: NewMockWorkflowRepository(), catalogFn: stageCatalog, stageRepo: stageLookupStub{}, stageDestinations: &destinationsStub{}}
		report, err := uc.Lint(context.Background(), moveStageDraft(stageID))
		if err != nil {
			t.Fatal(err)
		}
		if notes := stageNotes(report.Issues); len(notes) != 0 {
			t.Fatalf("%s: notes = %+v, want none", stageID, notes)
		}
	}
}

func TestPublishingAMoveOntoAnOpportunityStageIsBlocked(t *testing.T) {
	uc := &activateWorkflowUseCase{repo: NewMockWorkflowRepository(), catalogFn: stageCatalog, stageRepo: stageLookupStub{}, stageDestinations: &destinationsStub{}}
	report, err := uc.Lint(context.Background(), moveStageDraft("st-deal"))
	if err != nil {
		t.Fatal(err)
	}
	if report.IsGreen() || len(issueCodes(report.Issues)[workflow.LintInvalidResourceRef]) != 1 {
		t.Fatalf("issues = %+v, want the opportunity stage blocked", report.Issues)
	}
}
