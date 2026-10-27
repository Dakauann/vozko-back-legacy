package agent_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/automationneeds"
	automationneeds_usecase "vozko/usecases/automationneeds"
)

type scriptedPlanner struct {
	plan    automationneeds_usecase.Plan
	err     error
	planned []*agent.Agent
	done    []automationneeds_usecase.Plan
	cached  []string
}

func (p *scriptedPlanner) PlanAgent(_ context.Context, draft *agent.Agent) (automationneeds_usecase.Plan, error) {
	p.planned = append(p.planned, draft)
	return p.plan, p.err
}

func (p *scriptedPlanner) Done(_ context.Context, plan automationneeds_usecase.Plan, containers []string) {
	p.done = append(p.done, plan)
	p.cached = append(p.cached, containers...)
}

func (b *versionBook) PublishVersionSwitchingOff(ctx context.Context, agentID string, s agent.Snapshot, by, workspaceID string, offs []automationneeds.SwitchOff) (agent.Version, []string, error) {
	b.switched = append(b.switched, offs...)
	v, err := b.PublishVersion(ctx, agentID, s, by)
	return v, []string{"container-" + workspaceID}, err
}

func publishWithPlanner(planner *scriptedPlanner) (AgentPublishing, *versionBook) {
	_, update, store := sealingFixture(&vault{values: map[string]string{}})
	update.(ToolValidated).ValidateToolsWith(greenValidator{}, nil)
	book := &versionBook{pinned: map[agent.ToolPin]agent.ToolBinding{}}
	return NewPublishAgentUseCase(PublishDeps{Agents: store, Versions: book, Switching: book, Planner: planner, Published: publishedAgents{}, Update: update, Validator: greenValidator{}}), book
}

func TestAPublishThatWouldSwitchChannelsOffIsRefusedUntilConfirmed(t *testing.T) {
	refusal := &automationneeds.Refusal{Cause: automationneeds.ErrPublishSwitchesOff}
	planner := &scriptedPlanner{err: refusal}
	uc, book := publishWithPlanner(planner)
	if _, err := uc.Publish(context.Background(), "ag-http", publisher); !errors.Is(err, automationneeds.ErrPublishSwitchesOff) {
		t.Fatalf("err = %v", err)
	}
	if len(book.versions) != 0 || len(planner.planned) != 1 || planner.planned[0].ID != "ag-http" {
		t.Fatalf("versions %d planned %+v", len(book.versions), planner.planned)
	}
}

func TestAConfirmedPublishSwitchesTheChannelsOffInTheSameWrite(t *testing.T) {
	off := automationneeds.SwitchOff{Slot: automationneeds.Slot{Kind: automationneeds.SlotInstagram, ID: "ig"}}
	planner := &scriptedPlanner{plan: automationneeds_usecase.Plan{WorkspaceID: "ws-1", Offs: []automationneeds.SwitchOff{off}}}
	uc, book := publishWithPlanner(planner)
	if _, err := uc.Publish(context.Background(), "ag-http", publisher); err != nil {
		t.Fatal(err)
	}
	if len(book.versions) != 1 || len(book.switched) != 1 || book.switched[0].Slot.ID != "ig" {
		t.Fatalf("versions %d switched %+v", len(book.versions), book.switched)
	}
	if len(planner.done) != 1 || len(planner.cached) != 1 || planner.cached[0] != "container-ws-1" {
		t.Fatalf("done %+v cached %v", planner.done, planner.cached)
	}
}
