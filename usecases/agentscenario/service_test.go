package agentscenario_usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"vozko/domain/agent"
	"vozko/domain/agentrun"
	"vozko/domain/agentscenario"
	"vozko/domain/ai"
	aa "vozko/domain/ai_attendance"
	"vozko/domain/shared"
)

type memScenarios struct {
	agentscenario.Repository
	mu        sync.Mutex
	scenarios map[string]*agentscenario.Scenario
	runs      []*agentscenario.Run
}

func (m *memScenarios) List(string, string) ([]*agentscenario.Scenario, error) {
	out := []*agentscenario.Scenario{}
	for _, s := range m.scenarios {
		out = append(out, s)
	}
	return out, nil
}

func (m *memScenarios) Find(ws, agentID, id string) (*agentscenario.Scenario, error) {
	s, ok := m.scenarios[id]
	if !ok || s.WorkspaceID != ws || s.AgentID != agentID {
		return nil, agentscenario.ErrNotFound
	}
	copied := *s
	return &copied, nil
}

func (m *memScenarios) Create(s *agentscenario.Scenario) error {
	m.scenarios[s.ID] = s
	return nil
}

func (m *memScenarios) RecordRun(run *agentscenario.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs = append(m.runs, run)
	return nil
}

type scriptedSimulator struct {
	mu     sync.Mutex
	inputs []agent.SimulateTurnInput
	reply  func(in agent.SimulateTurnInput) (*agent.SimulateTurnOutput, error)
}

func (s *scriptedSimulator) Execute(_ context.Context, in agent.SimulateTurnInput) (*agent.SimulateTurnOutput, error) {
	s.mu.Lock()
	s.inputs = append(s.inputs, in)
	s.mu.Unlock()
	return s.reply(in)
}

type oneAgent struct {
	agent.Repository
}

func (oneAgent) FindByID(id string) (*agent.Agent, error) {
	return &agent.Agent{ID: id, WorkspaceID: "ws-1", MessagingModel: "modelo/x"}, nil
}

type fixedRun struct {
	detail *agentrun.Detail
	person shared.Person
}

func (f *fixedRun) Get(person shared.Person, _, _, _ string) (*agentrun.Detail, error) {
	f.person = person
	if f.detail == nil {
		return nil, agentrun.ErrRunNotFound
	}
	return f.detail, nil
}

type recordedJudge struct {
	req     JudgeRequest
	verdict agentscenario.Verdict
	err     error
}

func (j *recordedJudge) Judge(_ context.Context, req JudgeRequest) (agentscenario.Verdict, error) {
	j.req = req
	return j.verdict, j.err
}

func orderScenario() *agentscenario.Scenario {
	return &agentscenario.Scenario{
		ID: "8d1f4c3e-2b1a-4c5d-9e8f-7a6b5c4d3e2f", WorkspaceID: "ws-1", AgentID: "ag-1", Name: "Pedido",
		Messages: []agentscenario.Message{{Content: "Oi"}, {Content: "Cadê o pedido 42?"}},
		Mocks:    map[string]agentscenario.Mock{"t1": {Result: map[string]interface{}{"status": "enviado"}}},
		Assertions: []agentscenario.Assertion{
			{Kind: agentscenario.KindToolCalled, ToolID: "t1", Match: agentscenario.MatchSubset, Arguments: map[string]interface{}{"numero": "42"}},
			{Kind: agentscenario.KindToolOutputMatches, ToolID: "t1", Path: "status", Match: agentscenario.MatchExact, Value: "enviado"},
			{Kind: agentscenario.KindReplyMatches, Match: agentscenario.MatchContains, Value: "enviado"},
			{Kind: agentscenario.KindReplyJudged, Rubric: "Informa o status"},
		},
	}
}

func orderSimulator(argument string) *scriptedSimulator {
	return &scriptedSimulator{reply: func(in agent.SimulateTurnInput) (*agent.SimulateTurnOutput, error) {
		if in.Message == "Oi" {
			return &agent.SimulateTurnOutput{RunID: in.RunID, Replies: []string{"Olá!"}}, nil
		}
		mock := in.Mocks["t1"]
		return &agent.SimulateTurnOutput{RunID: in.RunID, Replies: []string{"Seu pedido foi enviado."}, ToolCalls: []agent.SimulatedToolCall{
			{ToolID: "t1", Name: "consultar_pedido", Arguments: map[string]interface{}{"numero": argument}, Output: mock.Result, Mocked: true},
		}}, nil
	}}
}

func harness(t *testing.T, sim *scriptedSimulator, judge *recordedJudge, runs *fixedRun) (*Service, *memScenarios) {
	t.Helper()
	store := &memScenarios{scenarios: map[string]*agentscenario.Scenario{}}
	s := orderScenario()
	store.scenarios[s.ID] = s
	svc, err := New(Deps{Scenarios: store, Simulate: sim, Agents: oneAgent{}, Runs: runs, Judge: judge})
	if err != nil {
		t.Fatal(err)
	}
	return svc, store
}

func TestAScenarioRunsTurnByTurnWithItsMocksAndPasses(t *testing.T) {
	sim := orderSimulator("42")
	judge := &recordedJudge{verdict: agentscenario.Verdict{Passed: true, Reason: "Informou o status."}}
	svc, store := harness(t, sim, judge, &fixedRun{})
	run, err := svc.Run(context.Background(), "ws-1", "ag-1", orderScenario().ID)
	if err != nil {
		t.Fatal(err)
	}
	if !run.Passed || len(run.Results) != 4 || len(run.Turns) != 2 || len(run.RunIDs) != 2 || len(store.runs) != 1 {
		t.Fatalf("run %+v", run)
	}
	second := sim.inputs[1]
	if !second.Scenario || second.RunID != run.RunIDs[1] || len(second.History) != 2 || second.History[1].Content != "Olá!" || second.Mocks["t1"].Result == nil {
		t.Fatalf("second turn %+v", second)
	}
	if judge.req.WorkspaceID != "ws-1" || judge.req.Model != "modelo/x" || !strings.Contains(judge.req.Transcript, "Cadê o pedido 42?") {
		t.Fatalf("judge request %+v", judge.req)
	}
}

func TestAChangedBehaviourTurnsTheScenarioRed(t *testing.T) {
	svc, _ := harness(t, orderSimulator("43"), &recordedJudge{verdict: agentscenario.Verdict{Passed: true}}, &fixedRun{})
	run, err := svc.Run(context.Background(), "ws-1", "ag-1", orderScenario().ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Passed || run.Results[0].Passed {
		t.Fatalf("run %+v", run.Results)
	}
}

func TestAJudgeThatCannotAnswerFailsItsAssertion(t *testing.T) {
	svc, _ := harness(t, orderSimulator("42"), &recordedJudge{err: errors.New("provider down")}, &fixedRun{})
	run, _ := svc.Run(context.Background(), "ws-1", "ag-1", orderScenario().ID)
	if run.Passed || run.Results[3].Passed {
		t.Fatalf("results %+v", run.Results)
	}
}

func TestASimulationErrorFailsEveryAssertionAndIsRecorded(t *testing.T) {
	sim := &scriptedSimulator{reply: func(agent.SimulateTurnInput) (*agent.SimulateTurnOutput, error) {
		return nil, errors.New("model unavailable")
	}}
	svc, store := harness(t, sim, &recordedJudge{}, &fixedRun{})
	run, err := svc.Run(context.Background(), "ws-1", "ag-1", orderScenario().ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Passed || run.Error == "" || len(run.Results) != 4 || run.Results[2].Passed || len(store.runs) != 1 {
		t.Fatalf("run %+v", run)
	}
}

func TestRunAllRunsEveryScenario(t *testing.T) {
	svc, store := harness(t, orderSimulator("42"), &recordedJudge{verdict: agentscenario.Verdict{Passed: true}}, &fixedRun{})
	second := orderScenario()
	second.ID = "1d1f4c3e-2b1a-4c5d-9e8f-7a6b5c4d3e2f"
	store.scenarios[second.ID] = second
	runs, err := svc.RunAll(context.Background(), "ws-1", "ag-1")
	if err != nil || len(runs) != 2 || len(store.runs) != 2 {
		t.Fatalf("runs %+v err %v", runs, err)
	}
}

func TestAnInvalidDraftIsRefusedBeforeSaving(t *testing.T) {
	svc, store := harness(t, orderSimulator("42"), &recordedJudge{}, &fixedRun{})
	_, err := svc.Create("ws-1", "ag-1", Draft{Name: "x", Messages: []agentscenario.Message{{Content: "oi"}}}, "u-1")
	if !errors.Is(err, agentscenario.ErrInvalid) || len(store.scenarios) != 1 {
		t.Fatalf("err %v scenarios %d", err, len(store.scenarios))
	}
}

func TestReplayBuildsMocksAndAssertionsFromTheRunSpans(t *testing.T) {
	runs := &fixedRun{detail: &agentrun.Detail{
		Run: agentrun.Run{ID: "run-1", UserMessage: "Meu cpf é 123.456.789-09, cadê o pedido?", StartedAt: time.Date(2026, 10, 24, 9, 30, 0, 0, time.UTC)},
		Spans: []aa.ToolCall{
			{ToolInstanceID: "t1", Outcome: aa.ToolCallOK, MappedOutput: map[string]interface{}{"status": "enviado"}},
			{ToolInstanceID: "t2", Outcome: aa.ToolCallError, ModelResult: `{"is_error":true,"result":"fora do ar"}`},
			{ToolInstanceID: "t1", Outcome: aa.ToolCallOK, MappedOutput: map[string]interface{}{"status": "outro"}},
			{ToolName: "mcp__x", Outcome: aa.ToolCallOK},
		},
	}}
	svc, _ := harness(t, orderSimulator("42"), &recordedJudge{}, runs)
	draft, err := svc.FromRun(shared.Person{UserID: "u-1"}, "ws-1", "ag-1", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if runs.person.UserID != "u-1" || draft.SourceRunID != "run-1" || len(draft.Messages) != 1 || strings.Contains(draft.Messages[0].Content, "123.456.789-09") {
		t.Fatalf("draft %+v", draft)
	}
	if draft.Mocks["t1"].Result.(map[string]interface{})["status"] != "enviado" || !draft.Mocks["t2"].IsError || draft.Mocks["t2"].Result != "fora do ar" {
		t.Fatalf("mocks %+v", draft.Mocks)
	}
	if len(draft.Assertions) != 2 || draft.Assertions[0].ToolID != "t1" || draft.Assertions[1].ToolID != "t2" {
		t.Fatalf("assertions %+v", draft.Assertions)
	}
}

func TestMissingDependenciesAreNamed(t *testing.T) {
	_, err := New(Deps{})
	if err == nil || !strings.Contains(err.Error(), "judge") || !strings.Contains(err.Error(), "simulator") {
		t.Fatalf("err %v", err)
	}
}

type capturedAI struct {
	ai.Service
	input ai.GenerateInput
	reply string
}

func (c *capturedAI) Generate(_ context.Context, in ai.GenerateInput) (*ai.GenerateOutput, error) {
	c.input = in
	return &ai.GenerateOutput{Message: ai.Message{Content: c.reply}}, nil
}

func TestTheJudgeIsBilledToTheWorkspaceAndFailsClosedOnAnUnreadableVerdict(t *testing.T) {
	service := &capturedAI{reply: `{"passed":true,"reason":"Cumpre."}`}
	judge, err := NewJudge(service)
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := judge.Judge(context.Background(), JudgeRequest{WorkspaceID: "ws-1", AgentID: "ag-1", Model: "modelo/x", Rubric: "r", Transcript: "t"})
	if err != nil || !verdict.Passed || verdict.Reason != "Cumpre." {
		t.Fatalf("verdict %+v err %v", verdict, err)
	}
	if service.input.WorkspaceID != "ws-1" || service.input.BillingReference != "agent_scenario_judge:ag-1" || service.input.Model != "modelo/x" {
		t.Fatalf("input %+v", service.input)
	}
	service.reply = "sim"
	if _, err := judge.Judge(context.Background(), JudgeRequest{WorkspaceID: "ws-1"}); !errors.Is(err, ErrJudgeUnreadable) {
		t.Fatalf("err %v", err)
	}
}
