package automationneeds

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/notification"
	"vozko/domain/shared"
	"vozko/domain/workflow"
)

type recordedPublisher struct {
	drafts   []notification.Draft
	resolved []string
}

func (p *recordedPublisher) Publish(_ context.Context, d notification.Draft) (notification.Record, error) {
	p.drafts = append(p.drafts, d)
	return notification.Record{}, nil
}

func (p *recordedPublisher) Resolve(_ context.Context, _ string, keys ...string) error {
	p.resolved = append(p.resolved, keys...)
	return nil
}

func newRuntime(t *testing.T, values fakeValues) (*Runtime, *recordedPublisher) {
	t.Helper()
	publisher := &recordedPublisher{}
	alerts, err := NewAlerts(publisher)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRuntime(newService(t, values), alerts)
	if err != nil {
		t.Fatal(err)
	}
	return r, publisher
}

func TestAnAgentMissingAVariableDoesNotActAndRaisesTheNotice(t *testing.T) {
	r, publisher := newRuntime(t, fakeValues{values: map[string]string{"lead_name": "Ana"}})
	if _, ok := r.AgentValues(context.Background(), "ws", "e1", shared.EntryTypeInstagram, "escola"); ok {
		t.Fatal("the agent must not act without turma")
	}
	if len(publisher.drafts) != 1 || publisher.drafts[0].EpisodeKey != "automation_variables_unmet:agent:escola" || publisher.drafts[0].Category != notification.CategoryAutomations || !publisher.drafts[0].Email {
		t.Fatalf("drafts = %+v", publisher.drafts)
	}
	values, ok := r.AgentValues(context.Background(), "ws", "e1", shared.EntryTypeInstagram, "livre")
	if !ok || values["lead_name"] != "Ana" {
		t.Fatalf("an agent without needs acts with the values = %v %v", values, ok)
	}
}

func triggeredRun(entryID, entryType string) (*workflow.WorkflowRun, *workflow.Node) {
	node := &workflow.Node{ID: "t1", Type: workflow.NodeTypeTriggerMessageReceived}
	return &workflow.WorkflowRun{ID: "r1", WorkflowID: "cobranca", WorkspaceID: "ws", EntryID: entryID, EntryType: entryType, TriggerNodeID: "t1", CurrentNodeID: "t1", State: workflow.NewRunState()}, node
}

func TestAWorkflowRunStartsOnlyWithEveryValueAndSeesThemAsCampvars(t *testing.T) {
	r, publisher := newRuntime(t, fakeValues{values: map[string]string{"valor": "10", "lead_name": "Ana"}})
	run, node := triggeredRun("e1", "instagram")
	if _, err := r.BeforeStep(run, node); err != nil {
		t.Fatalf("BeforeStep = %v", err)
	}
	got, _ := run.State.Get(workflow.DataKeyCampaignVars)
	if vars, ok := got.(map[string]interface{}); !ok || vars["valor"] != "10" || vars["lead_name"] != "Ana" {
		t.Fatalf("campvars = %#v", got)
	}
	if len(publisher.drafts) != 0 {
		t.Fatalf("no notice expected, got %+v", publisher.drafts)
	}

	blocked, _ := newRuntime(t, fakeValues{values: map[string]string{}})
	run, node = triggeredRun("e1", "instagram")
	if _, err := blocked.BeforeStep(run, node); err == nil {
		t.Fatal("a run without valor must not start")
	}
}

func TestOnlyTheTriggerStepIsChecked(t *testing.T) {
	r, _ := newRuntime(t, fakeValues{err: errors.New("down")})
	run, _ := triggeredRun("e1", "instagram")
	if _, err := r.BeforeStep(run, &workflow.Node{ID: "n2"}); err != nil {
		t.Fatalf("a later step = %v", err)
	}
}

func TestARunWithoutAConversationCannotNeedVariables(t *testing.T) {
	r, publisher := newRuntime(t, fakeValues{})
	run, node := triggeredRun("lead-1", workflow.EntryTypeLead)
	if _, err := r.BeforeStep(run, node); err == nil {
		t.Fatal("a lead run of a workflow that needs valor must not start")
	}
	if len(publisher.drafts) != 1 {
		t.Fatalf("drafts = %+v", publisher.drafts)
	}
}
