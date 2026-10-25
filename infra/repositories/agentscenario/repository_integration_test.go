package agentscenario_repository

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/agentscenario"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func scenarioFixture(ws, agentID string, at time.Time) *agentscenario.Scenario {
	return &agentscenario.Scenario{
		ID: uuid.NewString(), WorkspaceID: ws, AgentID: agentID, Name: "Pedido atrasado",
		Messages:   []agentscenario.Message{{Content: "Cadê meu pedido?"}},
		Mocks:      map[string]agentscenario.Mock{"t1": {Result: map[string]interface{}{"status": "enviado"}}},
		Assertions: []agentscenario.Assertion{{Kind: agentscenario.KindToolCalled, ToolID: "t1"}},
		CreatedBy:  "u-1", CreatedAt: at, UpdatedAt: at,
	}
}

func TestPostgresScenariosKeepTheirContentAndTheLastTwoResults(t *testing.T) {
	db := repotest.IsolatedDB(t, "agscen", &schema.AgentScenarioSchema{}, &schema.AgentScenarioRunSchema{})
	repo := NewRepository(db)
	ws, agentID := uuid.NewString(), uuid.NewString()
	at := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	s := scenarioFixture(ws, agentID, at)
	if err := repo.Create(s); err != nil {
		t.Fatal(err)
	}
	for i, passed := range []bool{true, false, true} {
		started := at.Add(time.Duration(i) * time.Minute)
		run := &agentscenario.Run{ID: uuid.NewString(), ScenarioID: s.ID, WorkspaceID: ws, AgentID: agentID, Passed: passed,
			Results: []agentscenario.AssertionResult{{Index: 0, Kind: agentscenario.KindToolCalled, Passed: passed, Reason: "r"}},
			RunIDs:  []string{uuid.NewString()}, StartedAt: started, FinishedAt: started.Add(time.Second)}
		if err := repo.RecordRun(run); err != nil {
			t.Fatal(err)
		}
	}
	list, err := repo.List(ws, agentID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list %+v %v", list, err)
	}
	got := list[0]
	if got.Mocks["t1"].Result.(map[string]interface{})["status"] != "enviado" || got.Assertions[0].ToolID != "t1" || got.Messages[0].Content != "Cadê meu pedido?" {
		t.Fatalf("content %+v", got)
	}
	if got.Last == nil || !got.Last.Passed || got.Previous == nil || got.Previous.Passed {
		t.Fatalf("last %+v previous %+v", got.Last, got.Previous)
	}
	runs, err := repo.Runs(ws, agentID, s.ID, 10)
	if err != nil || len(runs) != 3 || len(runs[0].Results) != 1 || len(runs[0].RunIDs) != 1 {
		t.Fatalf("runs %+v %v", runs, err)
	}
	if _, err := repo.Find(uuid.NewString(), agentID, s.ID); err != agentscenario.ErrNotFound {
		t.Fatalf("another workspace reads the scenario: %v", err)
	}
	s.Name = "Pedido entregue"
	s.UpdatedAt = at.Add(time.Hour)
	if err := repo.Update(s); err != nil {
		t.Fatal(err)
	}
	if found, _ := repo.Find(ws, agentID, s.ID); found.Name != "Pedido entregue" {
		t.Fatalf("found %+v", found)
	}
	if n, err := repo.DeleteRunsStartedBefore(at.Add(90*time.Second), 100); err != nil || n != 2 {
		t.Fatalf("pruned %d %v", n, err)
	}
	if err := repo.Delete(ws, agentID, s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Find(ws, agentID, s.ID); err != agentscenario.ErrNotFound {
		t.Fatalf("deleted scenario still found: %v", err)
	}
	var left int64
	db.Model(&schema.AgentScenarioRunSchema{}).Where("scenario_id = ?", s.ID).Count(&left)
	if left != 0 {
		t.Fatalf("runs of a deleted scenario left: %d", left)
	}
}
