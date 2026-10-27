package workflow_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/automationneeds"
	"vozko/domain/workflow"
	automationneeds_usecase "vozko/usecases/automationneeds"
)

type scriptedWorkflowPlanner struct {
	plan    automationneeds_usecase.Plan
	err     error
	planned []*workflow.Workflow
	done    []automationneeds_usecase.Plan
}

func (p *scriptedWorkflowPlanner) PlanWorkflow(_ context.Context, draft *workflow.Workflow) (automationneeds_usecase.Plan, error) {
	p.planned = append(p.planned, draft)
	return p.plan, p.err
}

func (p *scriptedWorkflowPlanner) Done(_ context.Context, plan automationneeds_usecase.Plan, _ []string) {
	p.done = append(p.done, plan)
}

func TestAWorkflowPublishThatWouldSwitchChannelsOffWaitsForTheConfirmation(t *testing.T) {
	repo := NewMockWorkflowRepository()
	storedWorkflow(t, repo, workflow.WorkflowStatusDraft, manualGraph("end"))
	planner := &scriptedWorkflowPlanner{err: &automationneeds.Refusal{Cause: automationneeds.ErrPublishSwitchesOff}}
	uc := activation(repo)
	uc.planner = planner
	if _, err := uc.Publish(publisherCtx(), "wf-1"); !errors.Is(err, automationneeds.ErrPublishSwitchesOff) {
		t.Fatalf("err = %v", err)
	}
	if got, _ := repo.FindByID("wf-1"); got.PublishedVersion != nil {
		t.Fatal("a refused publish must not create a version")
	}
}

func TestAConfirmedWorkflowPublishSwitchesOffInTheSameWrite(t *testing.T) {
	repo := NewMockWorkflowRepository()
	storedWorkflow(t, repo, workflow.WorkflowStatusDraft, manualGraph("end"))
	off := automationneeds.SwitchOff{Slot: automationneeds.Slot{Kind: automationneeds.SlotTelegram, ID: "tg"}}
	planner := &scriptedWorkflowPlanner{plan: automationneeds_usecase.Plan{WorkspaceID: "ws1", Offs: []automationneeds.SwitchOff{off}}}
	uc := activation(repo)
	uc.planner = planner
	if _, err := uc.Publish(publisherCtx(), "wf-1"); err != nil {
		t.Fatal(err)
	}
	if len(repo.SwitchedOff) != 1 || repo.SwitchedOff[0].Slot.ID != "tg" || len(planner.done) != 1 {
		t.Fatalf("switched %+v done %d", repo.SwitchedOff, len(planner.done))
	}
}
