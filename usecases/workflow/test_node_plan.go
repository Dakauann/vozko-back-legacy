package workflow_usecase

import (
	"context"
	"errors"
	"strings"

	"vozko/domain/agent"
	"vozko/domain/automationneeds"
	"vozko/domain/shared"
	"vozko/domain/workflow"
)

type nodeTestPlan struct {
	deps         []workflow.VariableDependency
	mocks        []workflow.RequiredMock
	mode         TestMode
	hasAIDeps    bool
	canRunDirect bool
}

func (uc *testNodeUseCase) planFor(wf *workflow.Workflow, node *workflow.Node) (nodeTestPlan, error) {
	deps := workflow.ExtractDependencies(node.Config)
	mode, _, canRunDirect := resolveTestMode(deps)
	mocks := filterMocksForAIDeps(workflow.GetRequiredMocks(deps), &wf.Graph)
	hasAIDeps := workflow.HasAIDependencies(deps) || len(mocks) > 0
	if len(mocks) > 0 && canRunDirect {
		canRunDirect = false
		mode = TestModeExecuteUntil
		hasAIDeps = true
	}
	needs, err := automationneeds.OfNode(uc.needsSource(), wf.WorkspaceID, wf, node.ID)
	if err != nil {
		return nodeTestPlan{}, err
	}
	for _, need := range needs {
		mocks = withMock(mocks, automationMock(need))
	}
	if node.Type == workflow.NodeTypeActionAIAgent && len(needs) > 0 {
		hasAIDeps = true
	}
	if node.Type == workflow.NodeTypeWaitDTMF {
		mocks = append(mocks, workflow.RequiredMock{
			StateKey:    testKeyMock,
			DisplayName: "Tecla pressionada (vazio simula silêncio)",
			Source:      workflow.DependencySourceCaller,
			Optional:    true,
		})
	}
	if node.Type == workflow.NodeTypeTransferToQueue {
		mocks = append(mocks, workflow.RequiredMock{
			StateKey:    testTransferMock,
			DisplayName: "Resultado da fila ('timeout' simula ninguém atendendo; vazio, atendido)",
			Source:      workflow.DependencySourceCaller,
			Optional:    true,
		})
	}
	return nodeTestPlan{deps: deps, mocks: mocks, mode: mode, hasAIDeps: hasAIDeps, canRunDirect: canRunDirect}, nil
}

func campaignVarKey(name string) string {
	return workflow.DataKeyCampaignVars + "." + name
}

func automationMock(need automationneeds.Need) workflow.RequiredMock {
	return workflow.RequiredMock{
		StateKey:    campaignVarKey(need.Name),
		DisplayName: campaignVarKey(need.Name),
		Source:      workflow.DependencySourceAutomationVar,
		Hint:        need.Description,
		NeededBy:    need.Owner().Name,
	}
}

func withMock(mocks []workflow.RequiredMock, mock workflow.RequiredMock) []workflow.RequiredMock {
	for _, existing := range mocks {
		if existing.StateKey == mock.StateKey {
			return mocks
		}
	}
	return append(mocks, mock)
}

var errNodeTestNeedsUnwired = errors.New("workflow: the node test has no reader for the agents and workflows a node runs")

type unwiredNeedsSource struct{}

func (unwiredNeedsSource) Agent(string) (*agent.Agent, error) { return nil, errNodeTestNeedsUnwired }

func (unwiredNeedsSource) Workflow(string) (*workflow.Workflow, error) {
	return nil, errNodeTestNeedsUnwired
}

func (uc *testNodeUseCase) needsSource() automationneeds.Source {
	if uc.deps.Needs == nil {
		return unwiredNeedsSource{}
	}
	return uc.deps.Needs
}

func (uc *testNodeUseCase) viewer(ctx context.Context) (shared.Person, bool) {
	if uc.deps.ViewerOf == nil {
		return shared.Person{}, false
	}
	viewer, ok := uc.deps.ViewerOf(ctx)
	return viewer, ok && strings.TrimSpace(viewer.UserID) != ""
}

func (uc *testNodeUseCase) recordedState(ctx context.Context, wf *workflow.Workflow, nodeID string, from *TestFromRun) (*workflow.RunState, error) {
	if from == nil {
		return nil, nil
	}
	viewer, ok := uc.viewer(ctx)
	runID := strings.TrimSpace(from.RunID)
	if !ok || uc.deps.Runs == nil || runID == "" {
		return nil, workflow.ErrRunNotFound
	}
	detail, err := uc.deps.Runs.PickSteps(viewer, wf.WorkspaceID, runID, workflow.TestSeedPick(from.StepID))
	if err != nil {
		return nil, err
	}
	if detail == nil || detail.Run == nil || detail.Run.WorkflowID != wf.ID {
		return nil, workflow.ErrRunNotFound
	}
	state, err := workflow.RecordedTestState(detail.Run, detail.Logs, nodeID, strings.TrimSpace(from.StepID))
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (uc *testNodeUseCase) prefill(ctx context.Context, wf *workflow.Workflow, node *workflow.Node, mocks []workflow.RequiredMock, from *TestFromRun) (map[string]interface{}, error) {
	if from != nil {
		recorded, err := uc.recordedState(ctx, wf, node.ID, from)
		if err != nil {
			return nil, err
		}
		return recordedMockValues(recorded, mocks), nil
	}
	return uc.savedMockValues(ctx, wf, mocks)
}

func (uc *testNodeUseCase) savedMockValues(ctx context.Context, wf *workflow.Workflow, mocks []workflow.RequiredMock) (map[string]interface{}, error) {
	viewer, ok := uc.viewer(ctx)
	if !ok || uc.deps.TestValues == nil {
		return nil, nil
	}
	saved, err := uc.deps.TestValues.Saved(wf, viewer.UserID)
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{}
	for _, mock := range mocks {
		if mock.Source != workflow.DependencySourceAutomationVar {
			continue
		}
		name := strings.TrimPrefix(mock.StateKey, workflow.DataKeyCampaignVars+".")
		if value := strings.TrimSpace(saved[name]); value != "" {
			out[mock.StateKey] = value
		}
	}
	return out, nil
}

func recordedMockValues(recorded *workflow.RunState, mocks []workflow.RequiredMock) map[string]interface{} {
	out := map[string]interface{}{}
	if recorded == nil {
		return out
	}
	for _, mock := range mocks {
		if value, ok := workflow.ResolveVariable("var."+mock.StateKey, recorded); ok && !isBlankMock(value) {
			out[mock.StateKey] = value
		}
	}
	return out
}

func isBlankMock(value interface{}) bool {
	text, isText := value.(string)
	return value == nil || (isText && strings.TrimSpace(text) == "")
}

func (t *nodeTest) prepare(plan nodeTestPlan, recorded *workflow.RunState) {
	t.plan = plan
	t.mocked = t.input.MockedState
	if recorded == nil {
		return
	}
	t.recorded = true
	for key, value := range recorded.Vars {
		t.run.State.Set(key, value)
	}
	mocked := recordedMockValues(recorded, plan.mocks)
	for key, value := range t.input.MockedState {
		mocked[key] = value
	}
	t.mocked = mocked
}
