package node_executors

import (
	"testing"

	"github.com/stretchr/testify/require"

	"vozko/domain/actor"
	"vozko/domain/stage"
	"vozko/domain/workflow"
)

type stageStoreStub struct {
	stages  map[string]*stage.Stage
	current map[string]*stage.EntryStage
}

func (s *stageStoreStub) FindByID(id string) (*stage.Stage, error) {
	if st, ok := s.stages[id]; ok {
		return st, nil
	}
	return nil, stage.ErrTagNotFound
}

func (s *stageStoreStub) GetEntryStage(entryID, _, _ string) (*stage.EntryStage, error) {
	if es, ok := s.current[entryID]; ok {
		return es, nil
	}
	return nil, stage.ErrEntryTagNotFound
}

type stageMoverStub struct {
	destinations map[string]stage.FunnelStage
	from         stage.FunnelStage
	calls        []stage.AssignEntryStageInput
	destErr      error
	err          error
}

func (m *stageMoverStub) Execute(ws string, in stage.AssignEntryStageInput) (*stage.EntryStage, error) {
	move, err := m.Move(ws, in)
	return move.Entry, err
}

func (m *stageMoverStub) Destination(_ string, stageID string) (stage.FunnelStage, error) {
	if m.destErr != nil {
		return stage.FunnelStage{}, m.destErr
	}
	to, ok := m.destinations[stageID]
	if !ok {
		return stage.FunnelStage{}, stage.ErrTagNotFound
	}
	return to, nil
}

func (m *stageMoverStub) Move(_ string, in stage.AssignEntryStageInput) (stage.Move, error) {
	m.calls = append(m.calls, in)
	if m.err != nil {
		return stage.Move{}, m.err
	}
	return stage.Move{Entry: &stage.EntryStage{StageID: in.StageID}, From: m.from, To: m.destinations[in.StageID]}, nil
}

func salesMover() *stageMoverStub {
	return &stageMoverStub{
		destinations: map[string]stage.FunnelStage{
			"st-new":  {StageID: "st-new", StageName: "Novo", PipelineID: "pl-sales", PipelineName: "Vendas"},
			"st-won":  {StageID: "st-won", StageName: "Ganho", PipelineID: "pl-sales", PipelineName: "Vendas"},
			"st-paid": {StageID: "st-paid", StageName: "pagamento efetuado", PipelineID: "pl-altiva", PipelineName: "Altiva"},
		},
		from: stage.FunnelStage{StageID: "st-new", StageName: "Novo", PipelineID: "pl-sales", PipelineName: "Vendas"},
	}
}

type stageBroadcastStub struct{ entries []string }

func (b *stageBroadcastStub) BroadcastStageUpdate(_, entryID, _ string) {
	b.entries = append(b.entries, entryID)
}

func stagePipelines() *stageStoreStub {
	return &stageStoreStub{
		stages: map[string]*stage.Stage{
			"st-new":     {ID: "st-new", WorkspaceID: "ws1", PipelineID: "pl-sales", Name: "Novo"},
			"st-won":     {ID: "st-won", WorkspaceID: "ws1", PipelineID: "pl-sales", Name: "Ganho"},
			"st-foreign": {ID: "st-foreign", WorkspaceID: "ws-other", PipelineID: "pl-x", Name: "Outro"},
		},
		current: map[string]*stage.EntryStage{"e-1": {StageID: "st-new", StageName: "Novo"}},
	}
}

func stageNodeCtx(nodeType workflow.NodeType, config map[string]interface{}, edges []workflow.Edge) *workflow.NodeContext {
	state := workflow.NewRunState()
	return &workflow.NodeContext{
		Node:  &workflow.Node{ID: "n1", Type: nodeType, Config: config},
		Graph: &workflow.Graph{Edges: edges},
		Run:   &workflow.WorkflowRun{ID: "run1", WorkflowID: "wf-1", WorkspaceID: "ws1", EntryID: "e-1", EntryType: "unofficial_whatsapp"},
		State: &state,
	}
}

func moveEdges() []workflow.Edge {
	return []workflow.Edge{{Source: "n1", Target: "ok", Label: "sucesso"}, {Source: "n1", Target: "fail", Label: "erro"}}
}

func checkEdges() []workflow.Edge {
	return []workflow.Edge{{Source: "n1", Target: "yes", Label: "true"}, {Source: "n1", Target: "no", Label: "false"}}
}

func TestStageNodesOfferThePipelineStages(t *testing.T) {
	for _, def := range []workflow.NodeDefinition{
		NewMoveStageExecutor(stagePipelines(), salesMover(), nil).Definition(),
		NewCheckStageExecutor(stagePipelines()).Definition(),
	} {
		var source string
		for _, f := range def.ConfigSchema {
			if f.Key == "stage_id" {
				source = f.OptionsSource
			}
		}
		require.Equal(t, "stages", source, "%s has no stage picker", def.Type)
		require.True(t, def.Type.Valid(), "%s is not a registered node type", def.Type)
	}
}

func TestMoveStageDocumentsTheFunnelItLeftAndTheChange(t *testing.T) {
	def := NewMoveStageExecutor(stagePipelines(), salesMover(), nil).Definition()
	keys := map[string]workflow.VariableType{}
	for _, k := range def.OutputKeys {
		keys[k.Key] = k.Type
	}
	for _, key := range []string{"success", "changed", "stage_id", "stage_name", "pipeline_id", "pipeline_name", "from_stage_id", "from_stage_name", "from_pipeline_id", "from_pipeline_name", "pipeline_changed", "error", "error_code"} {
		_, ok := keys[key]
		require.True(t, ok, "output %s is not documented", key)
	}
	require.Equal(t, workflow.VariableTypeBoolean, keys["pipeline_changed"])
	require.Contains(t, def.Guidance.Behavior, "outro funil")
	require.NotEmpty(t, def.Guidance.Examples)
}

func TestMoveStageMovesTheConversationAsTheWorkflow(t *testing.T) {
	mover := salesMover()
	screens := &stageBroadcastStub{}
	exec := NewMoveStageExecutor(stagePipelines(), mover, screens)

	res, err := exec.Execute(stageNodeCtx(workflow.NodeTypeActionMoveStage, map[string]interface{}{"stage_id": "st-won"}, moveEdges()))

	require.NoError(t, err)
	require.Equal(t, "ok", res.NextNodeID)
	require.Len(t, mover.calls, 1)
	require.Equal(t, stage.AssignEntryStageInput{StageID: "st-won", EntryID: "e-1", EntryType: "unofficial_whatsapp", ActorID: actor.FormatWorkflow("wf-1"), Cause: stage.MoveCause{WorkflowID: "wf-1", RunID: "run1"}, AllowCrossPipeline: true}, mover.calls[0])
	require.Equal(t, []string{"e-1"}, screens.entries)
	require.Equal(t, true, res.Output["changed"])
	require.Equal(t, "Ganho", res.Output["stage_name"])
	require.Equal(t, "Novo", res.Output["from_stage_name"])
	require.Equal(t, false, res.Output["pipeline_changed"])
	require.Equal(t, "pl-sales", res.Output["from_pipeline_id"])
}

func TestMoveStageCarriesAConversationIntoTheChosenStagesFunnel(t *testing.T) {
	mover := salesMover()
	res, err := NewMoveStageExecutor(stagePipelines(), mover, nil).
		Execute(stageNodeCtx(workflow.NodeTypeActionMoveStage, map[string]interface{}{"stage_id": "st-paid"}, moveEdges()))

	require.NoError(t, err)
	require.Equal(t, "ok", res.NextNodeID)
	require.Len(t, mover.calls, 1)
	require.True(t, mover.calls[0].AllowCrossPipeline, "a stage the automation chose explicitly crosses funnels")
	require.Equal(t, map[string]interface{}{
		"success":            true,
		"changed":            true,
		"stage_id":           "st-paid",
		"stage_name":         "pagamento efetuado",
		"pipeline_id":        "pl-altiva",
		"pipeline_name":      "Altiva",
		"from_stage_id":      "st-new",
		"from_stage_name":    "Novo",
		"from_pipeline_id":   "pl-sales",
		"from_pipeline_name": "Vendas",
		"pipeline_changed":   true,
	}, res.Output)
}

func TestMoveStageToTheCurrentStageChangesNothing(t *testing.T) {
	mover := salesMover()
	res, err := NewMoveStageExecutor(stagePipelines(), mover, nil).
		Execute(stageNodeCtx(workflow.NodeTypeActionMoveStage, map[string]interface{}{"stage_id": "st-new"}, moveEdges()))

	require.NoError(t, err)
	require.Equal(t, "ok", res.NextNodeID)
	require.Empty(t, mover.calls)
	require.Equal(t, false, res.Output["changed"])
	require.Equal(t, false, res.Output["pipeline_changed"])
	require.Equal(t, "pl-sales", res.Output["from_pipeline_id"])
}

func TestMoveStageRefusalsLeaveThroughTheErrorHandle(t *testing.T) {
	for name, tc := range map[string]struct {
		stageID string
		destErr error
		err     error
		want    string
		code    string
	}{
		"opportunity funnel": {"st-won", stage.ErrStageNotConversationFunnel, nil, "oportunidades", "stage_not_conversation_funnel"},
		"deleted stage":      {"st-gone", nil, nil, "não existe", "stage_not_found"},
		"other workspace":    {"st-won", stage.ErrUnauthorized, nil, "não existe", "stage_not_found"},
		"refused on write":   {"st-won", nil, stage.ErrStageNotConversationFunnel, "oportunidades", "stage_not_conversation_funnel"},
		"generic failure":    {"st-won", nil, stage.ErrEntryNotFound, "falha", "stage_move_failed"},
	} {
		t.Run(name, func(t *testing.T) {
			mover := salesMover()
			mover.destErr, mover.err = tc.destErr, tc.err
			res, err := NewMoveStageExecutor(stagePipelines(), mover, nil).
				Execute(stageNodeCtx(workflow.NodeTypeActionMoveStage, map[string]interface{}{"stage_id": tc.stageID}, moveEdges()))

			require.NoError(t, err)
			require.Equal(t, "fail", res.NextNodeID)
			require.Contains(t, res.Output["error"], tc.want)
			require.Equal(t, tc.code, res.Output["error_code"])
			require.Equal(t, false, res.Output["success"])
			require.Equal(t, false, res.Output["pipeline_changed"])
		})
	}
}

func TestMoveStageWithoutAStageIsAConfigError(t *testing.T) {
	_, err := NewMoveStageExecutor(stagePipelines(), salesMover(), nil).
		Execute(stageNodeCtx(workflow.NodeTypeActionMoveStage, map[string]interface{}{}, moveEdges()))
	require.ErrorIs(t, err, workflow.ErrNodeConfigMissing)
}

func TestMoveStageWithoutAMoverFailsClosed(t *testing.T) {
	res, err := NewMoveStageExecutor(stagePipelines(), nil, nil).
		Execute(stageNodeCtx(workflow.NodeTypeActionMoveStage, map[string]interface{}{"stage_id": "st-won"}, moveEdges()))
	require.NoError(t, err)
	require.Equal(t, "fail", res.NextNodeID)
	require.Equal(t, "stage_move_failed", res.Output["error_code"])
}

func TestCheckStageBranchesOnTheCurrentStage(t *testing.T) {
	store := stagePipelines()
	exec := NewCheckStageExecutor(store)

	res, err := exec.Execute(stageNodeCtx(workflow.NodeTypeConditionCheckStage, map[string]interface{}{"stage_id": "st-new"}, checkEdges()))
	require.NoError(t, err)
	require.Equal(t, "yes", res.NextNodeID)
	require.Equal(t, true, res.Output["matched"])
	require.Equal(t, "Novo", res.Output["current_stage_name"])

	res, err = exec.Execute(stageNodeCtx(workflow.NodeTypeConditionCheckStage, map[string]interface{}{"stage_id": "st-won"}, checkEdges()))
	require.NoError(t, err)
	require.Equal(t, "no", res.NextNodeID)

	delete(store.current, "e-1")
	res, err = exec.Execute(stageNodeCtx(workflow.NodeTypeConditionCheckStage, map[string]interface{}{"stage_id": "st-new"}, checkEdges()))
	require.NoError(t, err)
	require.Equal(t, "no", res.NextNodeID, "a conversation in no stage is not in the chosen one")
}

func TestMoveStageTellsTheMoveWhichRunMadeItAndHowFarTheChainWent(t *testing.T) {
	mover := salesMover()
	ctx := stageNodeCtx(workflow.NodeTypeActionMoveStage, map[string]interface{}{"stage_id": "st-won"}, moveEdges())
	ctx.State.Set(workflow.StageChainKey, float64(2))

	_, err := NewMoveStageExecutor(stagePipelines(), mover, nil).Execute(ctx)

	require.NoError(t, err)
	require.Len(t, mover.calls, 1)
	require.Equal(t, stage.MoveCause{WorkflowID: "wf-1", RunID: "run1", Chain: 2}, mover.calls[0].Cause)
}
