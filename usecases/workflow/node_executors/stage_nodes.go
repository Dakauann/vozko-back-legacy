package node_executors

import (
	"errors"
	"fmt"
	"strings"

	"vozko/domain/stage"
	"vozko/domain/workflow"
)

type StageReader interface {
	FindByID(id string) (*stage.Stage, error)
	GetEntryStage(entryID, entryType, workspaceID string) (*stage.EntryStage, error)
}

type StageBroadcaster interface {
	BroadcastStageUpdate(workspaceID, entryID, entryType string)
}

func stagePicker(description string) workflow.ConfigField {
	return workflow.ConfigField{
		Key:           "stage_id",
		Label:         "Etapa",
		Type:          "select",
		OptionsSource: "stages",
		Required:      true,
		Description:   description,
	}
}

func currentStage(stages StageReader, run *workflow.WorkflowRun) *stage.EntryStage {
	current, err := stages.GetEntryStage(run.EntryID, run.EntryType, run.WorkspaceID)
	if err != nil || current == nil || current.StageID == "" {
		return nil
	}
	return current
}

type moveStageExecutor struct {
	stages    StageReader
	mover     stage.EntryMover
	broadcast StageBroadcaster
}

func NewMoveStageExecutor(stages StageReader, mover stage.EntryMover, broadcast StageBroadcaster) workflow.NodeExecutor {
	return &moveStageExecutor{stages: stages, mover: mover, broadcast: broadcast}
}

func (e *moveStageExecutor) Definition() workflow.NodeDefinition {
	return workflow.NodeDefinition{
		Type:         workflow.NodeTypeActionMoveStage,
		TestBehavior: workflow.NodeTestWritesData,
		Category:     workflow.NodeCategoryAction,
		Scopes:       []workflow.NodeScope{workflow.NodeScopeShared},
		Label:        "Mover Etapa",
		Description:  "Move a conversa para a etapa escolhida. Uma conversa de outro funil vai para o funil dessa etapa.",
		Icon:         "Kanban",
		Guidance: workflow.NodeGuidance{
			When: "Para avançar a conversa no funil conforme o fluxo (ex.: qualificado, proposta enviada, ganho) ou para levá-la a uma etapa de outro funil de conversas.",
			Behavior: "A conversa vai exatamente para a etapa escolhida. Se ela está em outro funil, muda para o funil dessa etapa, " +
				"como quem usa \"mover para funil\" no CRM; uma conversa ainda sem etapa entra no funil da etapa escolhida. " +
				"Já estando na etapa, não muda nada. Só etapas de funis de conversa servem; etapas de funis de negociações são recusadas. " +
				"pipeline_changed diz se a conversa trocou de funil, from_pipeline_id e from_pipeline_name dizem de onde ela saiu.",
			Examples: []string{
				"Cobrança paga: mover para \"pagamento efetuado\" do funil Altiva, mesmo que a conversa esteja no funil Atendimento.",
				"Depois da qualificação: mover para \"proposta enviada\" no mesmo funil.",
				"Ramificar com uma Condição em pipeline_changed para avisar a equipe do novo funil.",
			},
		},
		Outputs: []workflow.HandleDefinition{
			{ID: "sucesso", Label: "Sucesso"},
			{ID: "erro", Label: "Erro", Optional: true},
		},
		OutputKeys: []workflow.OutputKeyDefinition{
			{Key: "success", Description: "true quando a conversa está na etapa escolhida", Label: "Deu certo", Type: workflow.VariableTypeBoolean},
			{Key: "changed", Description: "true quando a conversa mudou de etapa (false se já estava nela)", Label: "Mudou agora", Type: workflow.VariableTypeBoolean},
			{Key: "stage_id", Description: "ID da etapa escolhida", Label: "ID da etapa", Type: workflow.VariableTypeText},
			{Key: "stage_name", Description: "Nome da etapa escolhida", Label: "Nome da etapa", Type: workflow.VariableTypeText},
			{Key: "pipeline_id", Description: "ID do funil da etapa escolhida", Label: "ID do funil", Type: workflow.VariableTypeText},
			{Key: "pipeline_name", Description: "Nome do funil da etapa escolhida", Label: "Funil", Type: workflow.VariableTypeText},
			{Key: "from_stage_id", Description: "ID da etapa anterior (vazio se não havia)", Label: "ID da etapa anterior", Type: workflow.VariableTypeText},
			{Key: "from_stage_name", Description: "Nome da etapa anterior", Label: "Etapa anterior", Type: workflow.VariableTypeText},
			{Key: "from_pipeline_id", Description: "ID do funil onde a conversa estava (vazio se não havia etapa)", Label: "ID do funil anterior", Type: workflow.VariableTypeText},
			{Key: "from_pipeline_name", Description: "Nome do funil onde a conversa estava", Label: "Funil anterior", Type: workflow.VariableTypeText},
			{Key: "pipeline_changed", Description: "true quando a conversa saiu de outro funil para o funil da etapa escolhida", Label: "Trocou de funil", Type: workflow.VariableTypeBoolean},
			{Key: "error", Description: "Descrição do erro quando a conversa não pôde ser movida", Label: "Erro", Type: workflow.VariableTypeText},
			{Key: "error_code", Description: "Código da recusa quando a conversa não pôde ser movida", Label: "Código da recusa", Type: workflow.VariableTypeText},
		},
		DefaultConfig: map[string]interface{}{"stage_id": ""},
		ConfigSchema:  []workflow.ConfigField{stagePicker("A etapa para onde a conversa vai. Uma conversa de outro funil muda para o funil desta etapa.")},
	}
}

func (e *moveStageExecutor) Execute(ctx *workflow.NodeContext) (*workflow.NodeResult, error) {
	stageID, _ := ctx.Node.Config["stage_id"].(string)
	stageID = strings.TrimSpace(workflow.Interpolate(stageID, ctx.State, nil))
	if stageID == "" {
		return nil, workflow.ErrNodeConfigMissing
	}
	edges := ctx.Graph.OutgoingEdges(ctx.Node.ID)
	if e.stages == nil || e.mover == nil {
		return failMove(edges, stageID, errMoverUnavailable), nil
	}

	to, err := e.mover.Destination(ctx.Run.WorkspaceID, stageID)
	if err != nil {
		return failMove(edges, stageID, err), nil
	}
	if from := currentStage(e.stages, ctx.Run); from != nil && from.StageID == to.StageID {
		return routeOutput(&workflow.NodeResult{Output: moveOutput(stage.Move{From: to, To: to}, false)}, edges, "sucesso"), nil
	}

	move, err := e.mover.Move(ctx.Run.WorkspaceID, stage.AssignEntryStageInput{
		StageID:            to.StageID,
		EntryID:            ctx.Run.EntryID,
		EntryType:          ctx.Run.EntryType,
		ActorID:            workflowActor(ctx.Run.WorkflowID),
		Cause:              moveCauseOf(ctx),
		AllowCrossPipeline: true,
	})
	if err != nil {
		return failMove(edges, stageID, err), nil
	}
	if e.broadcast != nil {
		e.broadcast.BroadcastStageUpdate(ctx.Run.WorkspaceID, ctx.Run.EntryID, ctx.Run.EntryType)
	}
	return routeOutput(&workflow.NodeResult{Output: moveOutput(move, true)}, edges, "sucesso"), nil
}

func moveOutput(move stage.Move, changed bool) map[string]interface{} {
	return map[string]interface{}{
		"success":            true,
		"changed":            changed,
		"stage_id":           move.To.StageID,
		"stage_name":         move.To.StageName,
		"pipeline_id":        move.To.PipelineID,
		"pipeline_name":      move.To.PipelineName,
		"from_stage_id":      move.From.StageID,
		"from_stage_name":    move.From.StageName,
		"from_pipeline_id":   move.From.PipelineID,
		"from_pipeline_name": move.From.PipelineName,
		"pipeline_changed":   move.FunnelChanged(),
	}
}

func moveCauseOf(ctx *workflow.NodeContext) stage.MoveCause {
	state := ctx.Run.State
	if ctx.State != nil {
		state = *ctx.State
	}
	return stage.MoveCause{WorkflowID: ctx.Run.WorkflowID, RunID: ctx.Run.ID, Chain: workflow.StageChainOf(state)}
}

var errMoverUnavailable = errors.New("stage mover unavailable")

func failMove(edges []workflow.Edge, stageID string, cause error) *workflow.NodeResult {
	return routeFailure(&workflow.NodeResult{
		Output: map[string]interface{}{
			"success":          false,
			"changed":          false,
			"pipeline_changed": false,
			"stage_id":         stageID,
			"error":            moveFailure(cause),
			"error_code":       stage.MoveRefusalCode(cause),
		},
	}, edges, "erro")
}

func moveFailure(err error) string {
	switch {
	case errors.Is(err, errMoverUnavailable):
		return "mover etapa indisponível neste contexto"
	case errors.Is(err, stage.ErrStageNotConversationFunnel):
		return "a etapa escolhida é de um funil de negociações: uma conversa só vai para etapas de funis de conversa"
	case errors.Is(err, stage.ErrTagNotFound), errors.Is(err, stage.ErrUnauthorized):
		return "a etapa escolhida não existe mais neste workspace: escolha outra no campo Etapa do nó"
	}
	return fmt.Sprintf("falha ao mover a conversa de etapa: %v", err)
}

type checkStageExecutor struct {
	stages StageReader
}

func NewCheckStageExecutor(stages StageReader) workflow.NodeExecutor {
	return &checkStageExecutor{stages: stages}
}

func (e *checkStageExecutor) Definition() workflow.NodeDefinition {
	return workflow.NodeDefinition{
		Type:         workflow.NodeTypeConditionCheckStage,
		TestBehavior: workflow.NodeTestReadsData,
		Category:     workflow.NodeCategoryCondition,
		Scopes:       []workflow.NodeScope{workflow.NodeScopeShared},
		Label:        "Verificar Etapa",
		Description:  "Verifica se a conversa está em uma etapa do funil.",
		Icon:         "Kanban",
		Outputs: []workflow.HandleDefinition{
			{ID: "true", Label: "Está na etapa"},
			{ID: "false", Label: "Não está"},
		},
		OutputKeys: []workflow.OutputKeyDefinition{
			{Key: "matched", Description: "true se a conversa está na etapa escolhida", Label: "Está na etapa", LabelKey: "output_matched_stage", Type: workflow.VariableTypeBoolean},
			{Key: "stage_id", Description: "ID da etapa verificada", RepeatsConfig: true},
			{Key: "current_stage_id", Description: "ID da etapa atual da conversa (vazio se não há)", Label: "ID da etapa atual", Type: workflow.VariableTypeText},
			{Key: "current_stage_name", Description: "Nome da etapa atual da conversa", Label: "Etapa atual", Type: workflow.VariableTypeText},
		},
		DefaultConfig: map[string]interface{}{"stage_id": ""},
		ConfigSchema:  []workflow.ConfigField{stagePicker("A etapa a comparar com a etapa atual da conversa.")},
	}
}

func (e *checkStageExecutor) Execute(ctx *workflow.NodeContext) (*workflow.NodeResult, error) {
	stageID, _ := ctx.Node.Config["stage_id"].(string)
	stageID = strings.TrimSpace(workflow.Interpolate(stageID, ctx.State, nil))
	if stageID == "" {
		return nil, workflow.ErrNodeConfigMissing
	}

	out := map[string]interface{}{"matched": false, "stage_id": stageID, "current_stage_id": "", "current_stage_name": ""}
	var current *stage.EntryStage
	if e.stages != nil {
		current = currentStage(e.stages, ctx.Run)
	}
	if current != nil {
		out["current_stage_id"], out["current_stage_name"] = current.StageID, current.StageName
		out["matched"] = current.StageID == stageID
	}

	branch := "false"
	if out["matched"] == true {
		branch = "true"
	}
	return routeOutput(&workflow.NodeResult{
		Output: out,
	}, ctx.Graph.OutgoingEdges(ctx.Node.ID), branch), nil
}
