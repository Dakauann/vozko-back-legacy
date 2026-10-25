package agentscenario_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"vozko/domain/agent"
	"vozko/domain/agentrun"
	"vozko/domain/agentscenario"
	aa "vozko/domain/ai_attendance"
	"vozko/domain/redact"
	"vozko/domain/shared"
	"vozko/domain/tools"
	"vozko/domain/tracepayload"
)

const (
	runHistoryLimit   = 20
	parallelScenarios = 3
	maxRunErrorBytes  = 4 * 1024
)

var messageRedactor = redact.New()

type Draft struct {
	Name       string
	Messages   []agentscenario.Message
	Mocks      map[string]agentscenario.Mock
	Assertions []agentscenario.Assertion
	SourceRun  string
}

type Judge interface {
	Judge(ctx context.Context, req JudgeRequest) (agentscenario.Verdict, error)
}

type JudgeRequest struct {
	WorkspaceID string
	AgentID     string
	Model       string
	Rubric      string
	Transcript  string
}

type RunReader interface {
	Get(person shared.Person, workspaceID, agentID, runID string) (*agentrun.Detail, error)
}

type Deps struct {
	Scenarios agentscenario.Repository
	Simulate  agent.SimulateTurnUseCase
	Agents    agent.Repository
	Runs      RunReader
	Judge     Judge
}

type Service struct {
	scenarios agentscenario.Repository
	simulate  agent.SimulateTurnUseCase
	agents    agent.Repository
	runs      RunReader
	judge     Judge
	now       func() time.Time
}

func New(d Deps) (*Service, error) {
	missing := []string{}
	for name, dep := range map[string]bool{
		"scenario store": d.Scenarios == nil, "simulator": d.Simulate == nil, "agent repository": d.Agents == nil,
		"run reader": d.Runs == nil, "judge": d.Judge == nil,
	} {
		if dep {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("agent scenarios: missing %s", strings.Join(missing, ", "))
	}
	return &Service{scenarios: d.Scenarios, simulate: d.Simulate, agents: d.Agents, runs: d.Runs, judge: d.Judge, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *Service) List(workspaceID, agentID string) ([]*agentscenario.Scenario, error) {
	return s.scenarios.List(workspaceID, agentID)
}

func (s *Service) Get(workspaceID, agentID, scenarioID string) (*agentscenario.Scenario, error) {
	return s.scenarios.Find(workspaceID, agentID, scenarioID)
}

func (s *Service) Create(workspaceID, agentID string, draft Draft, by string) (*agentscenario.Scenario, error) {
	now := s.now()
	scenario := &agentscenario.Scenario{
		ID: uuid.NewString(), WorkspaceID: workspaceID, AgentID: agentID, CreatedBy: by, CreatedAt: now, UpdatedAt: now, SourceRunID: draft.SourceRun,
	}
	apply(scenario, draft)
	if err := scenario.Check(); err != nil {
		return nil, err
	}
	if err := s.scenarios.Create(scenario); err != nil {
		return nil, err
	}
	return scenario, nil
}

func (s *Service) Update(workspaceID, agentID, scenarioID string, draft Draft) (*agentscenario.Scenario, error) {
	scenario, err := s.scenarios.Find(workspaceID, agentID, scenarioID)
	if err != nil {
		return nil, err
	}
	apply(scenario, draft)
	scenario.UpdatedAt = s.now()
	if err := scenario.Check(); err != nil {
		return nil, err
	}
	if err := s.scenarios.Update(scenario); err != nil {
		return nil, err
	}
	return scenario, nil
}

func (s *Service) Delete(workspaceID, agentID, scenarioID string) error {
	return s.scenarios.Delete(workspaceID, agentID, scenarioID)
}

func (s *Service) Runs(workspaceID, agentID, scenarioID string) ([]*agentscenario.Run, error) {
	if _, err := s.scenarios.Find(workspaceID, agentID, scenarioID); err != nil {
		return nil, err
	}
	return s.scenarios.Runs(workspaceID, agentID, scenarioID, runHistoryLimit)
}

func apply(scenario *agentscenario.Scenario, draft Draft) {
	scenario.Name = strings.TrimSpace(draft.Name)
	scenario.Messages = draft.Messages
	scenario.Mocks = draft.Mocks
	if scenario.Mocks == nil {
		scenario.Mocks = map[string]agentscenario.Mock{}
	}
	scenario.Assertions = draft.Assertions
}

func (s *Service) Run(ctx context.Context, workspaceID, agentID, scenarioID string) (*agentscenario.Run, error) {
	scenario, err := s.scenarios.Find(workspaceID, agentID, scenarioID)
	if err != nil {
		return nil, err
	}
	owner, err := s.agents.FindByID(agentID)
	if err != nil || owner == nil || owner.WorkspaceID != workspaceID {
		return nil, agent.ErrAgentNotFound
	}
	run := s.execute(ctx, owner, scenario)
	if err := s.scenarios.RecordRun(run); err != nil {
		return nil, err
	}
	return run, nil
}

func (s *Service) RunAll(ctx context.Context, workspaceID, agentID string) ([]*agentscenario.Run, error) {
	scenarios, err := s.scenarios.List(workspaceID, agentID)
	if err != nil {
		return nil, err
	}
	runs := make([]*agentscenario.Run, len(scenarios))
	errs := make([]error, len(scenarios))
	slots := make(chan struct{}, parallelScenarios)
	var wg sync.WaitGroup
	for i, scenario := range scenarios {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			runs[i], errs[i] = s.Run(ctx, workspaceID, agentID, id)
		}(i, scenario.ID)
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return runs, nil
}

func (s *Service) execute(ctx context.Context, owner *agent.Agent, scenario *agentscenario.Scenario) *agentscenario.Run {
	run := &agentscenario.Run{
		ID: uuid.NewString(), ScenarioID: scenario.ID, WorkspaceID: scenario.WorkspaceID, AgentID: scenario.AgentID,
		Turns: []agentscenario.Turn{}, RunIDs: []string{}, Results: []agentscenario.AssertionResult{}, StartedAt: s.now(),
	}
	mocks := toolMocks(scenario.Mocks)
	history := []agent.SimulationMessage{}
	seen := agentscenario.Observation{}
	for _, message := range scenario.Messages {
		runID := uuid.NewString()
		out, err := s.simulate.Execute(ctx, agent.SimulateTurnInput{
			WorkspaceID: scenario.WorkspaceID, AgentID: scenario.AgentID, History: history, Message: message.Content,
			Scenario: true, RunID: runID, Mocks: mocks,
		})
		if err != nil {
			run.Error = tracepayload.MarkedTextWithin(messageRedactor.String(err.Error()), maxRunErrorBytes)
			run.FinishedAt = s.now()
			run.Results = failedResults(scenario.Assertions, "a conversa simulada parou com erro")
			return run
		}
		turn := agentscenario.Turn{Message: message.Content, Replies: out.Replies, Calls: observedCalls(out.ToolCalls), RunID: runID}
		run.Turns = append(run.Turns, turn)
		run.RunIDs = append(run.RunIDs, runID)
		seen.Calls = append(seen.Calls, turn.Calls...)
		seen.Reply = strings.Join(out.Replies, "\n")
		history = append(history, agent.SimulationMessage{Role: agent.SimulationRoleUser, Content: message.Content})
		if seen.Reply != "" {
			history = append(history, agent.SimulationMessage{Role: agent.SimulationRoleAssistant, Content: seen.Reply})
		}
	}
	run.Results = agentscenario.Evaluate(scenario.Assertions, seen, s.verdicts(ctx, owner, scenario, run.Turns))
	run.Passed = agentscenario.Passed(run.Results)
	run.FinishedAt = s.now()
	return run
}

func (s *Service) verdicts(ctx context.Context, owner *agent.Agent, scenario *agentscenario.Scenario, turns []agentscenario.Turn) map[int]agentscenario.Verdict {
	out := map[int]agentscenario.Verdict{}
	transcript := transcriptOf(turns)
	for i, a := range scenario.Assertions {
		if a.Kind != agentscenario.KindReplyJudged {
			continue
		}
		verdict, err := s.judge.Judge(ctx, JudgeRequest{WorkspaceID: scenario.WorkspaceID, AgentID: owner.ID, Model: owner.MessagingModel, Rubric: a.Rubric, Transcript: transcript})
		if err != nil {
			slog.Warn("agent_scenario.judge_failed", slog.String("scenario_id", scenario.ID), slog.String("error", err.Error()))
			continue
		}
		out[i] = verdict
	}
	return out
}

func transcriptOf(turns []agentscenario.Turn) string {
	var b strings.Builder
	for _, turn := range turns {
		b.WriteString("Cliente: " + turn.Message + "\n")
		for _, call := range turn.Calls {
			b.WriteString("(ferramenta " + call.Name + ")\n")
		}
		for _, reply := range turn.Replies {
			b.WriteString("Agente: " + reply + "\n")
		}
	}
	return b.String()
}

func failedResults(assertions []agentscenario.Assertion, reason string) []agentscenario.AssertionResult {
	out := make([]agentscenario.AssertionResult, len(assertions))
	for i, a := range assertions {
		out[i] = agentscenario.AssertionResult{Index: i, Kind: a.Kind, Passed: false, Reason: reason}
	}
	return out
}

func toolMocks(mocks map[string]agentscenario.Mock) map[string]tools.Mock {
	out := make(map[string]tools.Mock, len(mocks))
	for id, m := range mocks {
		out[id] = tools.Mock{Result: m.Result, IsError: m.IsError}
	}
	return out
}

func observedCalls(calls []agent.SimulatedToolCall) []agentscenario.ObservedCall {
	out := make([]agentscenario.ObservedCall, 0, len(calls))
	for _, c := range calls {
		out = append(out, agentscenario.ObservedCall{ToolID: c.ToolID, Name: c.Name, Arguments: c.Arguments, Output: c.Output, IsError: c.IsError, Mocked: c.Mocked})
	}
	return out
}

func (s *Service) FromRun(person shared.Person, workspaceID, agentID, runID string) (*agentscenario.Scenario, error) {
	detail, err := s.runs.Get(person, workspaceID, agentID, runID)
	if err != nil {
		return nil, err
	}
	scenario := &agentscenario.Scenario{
		WorkspaceID: workspaceID, AgentID: agentID, SourceRunID: detail.Run.ID,
		Name:       "Execução de " + detail.Run.StartedAt.Format("02/01/2006 15:04"),
		Messages:   []agentscenario.Message{},
		Mocks:      map[string]agentscenario.Mock{},
		Assertions: []agentscenario.Assertion{},
	}
	if message := strings.TrimSpace(messageRedactor.String(detail.Run.UserMessage)); message != "" {
		scenario.Messages = append(scenario.Messages, agentscenario.Message{Content: message})
	}
	called := map[string]bool{}
	for _, span := range detail.Spans {
		if span.ToolInstanceID == "" {
			continue
		}
		if _, mocked := scenario.Mocks[span.ToolInstanceID]; !mocked {
			scenario.Mocks[span.ToolInstanceID] = mockOf(span)
		}
		if !called[span.ToolInstanceID] {
			called[span.ToolInstanceID] = true
			scenario.Assertions = append(scenario.Assertions, agentscenario.Assertion{Kind: agentscenario.KindToolCalled, ToolID: span.ToolInstanceID})
		}
	}
	return scenario, nil
}

func mockOf(span aa.ToolCall) agentscenario.Mock {
	mock := agentscenario.Mock{IsError: span.Outcome == aa.ToolCallError}
	if span.MappedOutput != nil {
		mock.Result = span.MappedOutput
		return mock
	}
	var payload struct {
		Result interface{} `json:"result"`
	}
	if json.Unmarshal([]byte(span.ModelResult), &payload) == nil && payload.Result != nil {
		mock.Result = payload.Result
		return mock
	}
	mock.Result = span.ModelResult
	return mock
}
