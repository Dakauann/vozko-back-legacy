package agent_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/lint"
)

type blockingValidator struct{ seenStored []agent.ToolBinding }

func (b *blockingValidator) Validate(_ context.Context, _ *agent.Agent, stored []agent.ToolBinding) agenttool.Report {
	b.seenStored = stored
	return agenttool.Report{Issues: []agenttool.Issue{{Code: agenttool.CodeHTTPURLInvalid, Severity: lint.SeverityBlocking, ToolID: "t1", Message: "x"}}}
}

type publishedAgents struct {
	byID map[string]*agent.Agent
	err  error
}

func (p publishedAgents) FindPublished(id string) (*agent.Agent, error) {
	if p.err != nil {
		return nil, p.err
	}
	a, ok := p.byID[id]
	if !ok {
		return nil, agent.ErrAgentNotPublished
	}
	return a, nil
}

func gateFixture(v ToolValidator, published agent.PublishedReader) (agent.UpdateAgentUseCase, *agentStore) {
	_, update, store := sealingFixture(&vault{values: map[string]string{}})
	update.(ToolValidated).ValidateToolsWith(v, published)
	return update, store
}

func activate() agent.UpdateAgentInput {
	on := true
	return agent.UpdateAgentInput{IsActive: &on}
}

func TestEditingTheDraftOfAnActiveAgentIsNeverBlocked(t *testing.T) {
	update, store := gateFixture(&blockingValidator{}, publishedAgents{})
	store.byID["ag-http"].IsActive = true
	if _, err := update.Execute(context.Background(), "ag-http", agent.UpdateAgentInput{InternalTools: []agent.ToolBinding{httpBinding(nil)}}); err != nil {
		t.Fatalf("a draft save must not touch the live agent: %v", err)
	}
}

func TestActivatingRefusesAPublishedVersionWithBlockingIssues(t *testing.T) {
	v := &blockingValidator{}
	live := &agent.Agent{ID: "ag-http", InternalTools: []agent.ToolBinding{{ID: "t1", Name: "http_request"}}}
	update, store := gateFixture(v, publishedAgents{byID: map[string]*agent.Agent{"ag-http": live}})
	writes := store.writes
	_, err := update.Execute(context.Background(), "ag-http", activate())
	var blocked *agenttool.BlockedError
	if !errors.As(err, &blocked) || !errors.Is(err, agenttool.ErrNotActivatable) || store.writes != writes {
		t.Fatalf("err %v", err)
	}
	if len(v.seenStored) != 1 || v.seenStored[0].ID != "t1" {
		t.Fatalf("the published tools are the stored baseline: %+v", v.seenStored)
	}
}

func TestActivatingAnUnpublishedAgentWaitsForPublish(t *testing.T) {
	update, _ := gateFixture(&blockingValidator{}, publishedAgents{})
	if _, err := update.Execute(context.Background(), "ag-http", activate()); err != nil {
		t.Fatalf("err %v", err)
	}
}

func TestActivationFailsClosedWhenThePublishedVersionCannotBeRead(t *testing.T) {
	update, _ := gateFixture(greenValidator{}, publishedAgents{err: errors.New("db down")})
	if _, err := update.Execute(context.Background(), "ag-http", activate()); !errors.Is(err, agenttool.ErrNotActivatable) {
		t.Fatalf("err %v", err)
	}
	unwired, _ := gateFixture(nil, nil)
	if _, err := unwired.Execute(context.Background(), "ag-http", activate()); !errors.Is(err, agenttool.ErrNotActivatable) {
		t.Fatalf("err %v", err)
	}
}
