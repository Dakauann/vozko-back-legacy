package stage_usecase

import (
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/google/uuid"

	ce "vozko/domain/conversation_event"
	"vozko/domain/pipeline"
	"vozko/domain/shared"
	"vozko/domain/stage"
)

var errFunnelsUnreadable = errors.New("stage: the funnel of the stage cannot be checked")

type AssignEntryStageUseCase struct {
	repo    stage.Repository
	funnels PipelineReader
	events  ce.Logger
}

func NewAssignEntryStageUseCase(repo stage.Repository, funnels PipelineReader, events ce.Logger) stage.EntryMover {
	return &AssignEntryStageUseCase{repo: repo, funnels: funnels, events: events}
}

func (uc *AssignEntryStageUseCase) Execute(workspaceID string, input stage.AssignEntryStageInput) (*stage.EntryStage, error) {
	move, err := uc.Move(workspaceID, input)
	if err != nil {
		return nil, err
	}
	return move.Entry, nil
}

func (uc *AssignEntryStageUseCase) Move(workspaceID string, input stage.AssignEntryStageInput) (stage.Move, error) {
	if err := stage.ValidateEntryType(input.EntryType); err != nil {
		return stage.Move{}, err
	}
	to, err := uc.Destination(workspaceID, input.StageID)
	if err != nil {
		return stage.Move{}, err
	}
	from, err := uc.placement(workspaceID, input, to)
	if err != nil {
		return stage.Move{}, err
	}
	if stage.CrossesFunnel(from.PipelineID, to.PipelineID) && !input.AllowCrossPipeline {
		return stage.Move{}, stage.ErrStagePipelineMismatch
	}

	et := &stage.EntryStage{
		ID:          uuid.New().String(),
		StageID:     input.StageID,
		EntryID:     input.EntryID,
		EntryType:   input.EntryType,
		WorkspaceID: workspaceID,
		Cause:       input.Cause,
	}
	if _, err := uc.repo.AssignStage(et); err != nil {
		return stage.Move{}, err
	}

	move := stage.Move{Entry: et, From: from, To: to}
	uc.logMove(workspaceID, input, move)

	fetched, err := uc.repo.GetEntryStage(input.EntryID, input.EntryType, workspaceID)
	if err != nil {
		return stage.Move{}, err
	}
	if fetched != nil {
		move.Entry = fetched
	}
	return move, nil
}

func (uc *AssignEntryStageUseCase) Destination(workspaceID, stageID string) (stage.FunnelStage, error) {
	target, err := uc.repo.FindByID(stageID)
	if err != nil {
		return stage.FunnelStage{}, err
	}
	if target == nil {
		return stage.FunnelStage{}, stage.ErrTagNotFound
	}
	if target.WorkspaceID != workspaceID {
		return stage.FunnelStage{}, stage.ErrUnauthorized
	}
	if target.PipelineID == "" {
		return stage.FunnelStage{StageID: target.ID, StageName: target.Name}, nil
	}
	funnel, err := uc.funnel(workspaceID, target.PipelineID)
	if err != nil {
		return stage.FunnelStage{}, err
	}
	if funnel.ObjectType != pipeline.ObjectConversation {
		return stage.FunnelStage{}, stage.ErrStageNotConversationFunnel
	}
	return stage.FunnelStage{StageID: target.ID, StageName: target.Name, PipelineID: funnel.ID, PipelineName: funnel.Name}, nil
}

func (uc *AssignEntryStageUseCase) funnel(workspaceID, pipelineID string) (*pipeline.Pipeline, error) {
	if uc.funnels == nil {
		return nil, errFunnelsUnreadable
	}
	funnel, err := uc.funnels.GetByID(workspaceID, pipelineID)
	if errors.Is(err, pipeline.ErrNotFound) {
		return nil, stage.ErrUnauthorized
	}
	if err != nil {
		return nil, fmt.Errorf("stage: reading the funnel %s: %w", pipelineID, err)
	}
	if funnel == nil || funnel.WorkspaceID != workspaceID {
		return nil, stage.ErrUnauthorized
	}
	return funnel, nil
}

func (uc *AssignEntryStageUseCase) placement(workspaceID string, input stage.AssignEntryStageInput, to stage.FunnelStage) (stage.FunnelStage, error) {
	current, err := uc.repo.GetEntryStage(input.EntryID, input.EntryType, workspaceID)
	if err != nil && !errors.Is(err, stage.ErrEntryTagNotFound) {
		return stage.FunnelStage{}, fmt.Errorf("stage: reading the stage of entry %s: %w", input.EntryID, err)
	}
	if current == nil || current.StageID == "" {
		return stage.FunnelStage{}, nil
	}
	if current.StageID == to.StageID {
		return to, nil
	}
	held, err := uc.repo.FindByID(current.StageID)
	if errors.Is(err, stage.ErrTagNotFound) || (err == nil && held == nil) {
		return stage.FunnelStage{StageID: current.StageID, StageName: current.StageName}, nil
	}
	if err != nil {
		return stage.FunnelStage{}, fmt.Errorf("stage: reading the current stage %s: %w", current.StageID, err)
	}
	if held.WorkspaceID != workspaceID {
		return stage.FunnelStage{}, ErrStageOutsideWorkspace
	}
	from := stage.FunnelStage{StageID: held.ID, StageName: held.Name, PipelineID: held.PipelineID}
	if !stage.CrossesFunnel(from.PipelineID, to.PipelineID) {
		return from, nil
	}
	funnel, err := uc.funnel(workspaceID, from.PipelineID)
	if errors.Is(err, stage.ErrUnauthorized) {
		return from, nil
	}
	if err != nil {
		return stage.FunnelStage{}, err
	}
	from.PipelineName = funnel.Name
	return from, nil
}

func (uc *AssignEntryStageUseCase) logMove(workspaceID string, input stage.AssignEntryStageInput, move stage.Move) {
	if uc.events == nil {
		return
	}

	details := map[string]string{
		"stage_id":    input.StageID,
		"to_stage_id": input.StageID,
		"stage_name":  move.To.StageName,
	}
	if input.Confidence > 0 {
		details["confidence"] = strconv.Itoa(int(math.Round(input.Confidence * 100)))
	}
	if move.From.StageID != "" && move.From.StageID != input.StageID {
		details["from_stage_id"] = move.From.StageID
		if move.From.StageName != "" {
			details["from_stage_name"] = move.From.StageName
		}
	}
	if move.FunnelChanged() {
		details["from_pipeline_id"] = move.From.PipelineID
		details["from_pipeline_name"] = move.From.PipelineName
		details["pipeline_id"] = move.To.PipelineID
		details["pipeline_name"] = move.To.PipelineName
	}

	channel := shared.EntryType(input.EntryType).EventChannel()
	uc.events.Log(ce.New(workspaceID, input.EntryID, input.EntryType, ce.EventStageChanged).
		WithActor(input.ActorID).
		WithChannel(channel).
		WithDetails(details).
		Build())
}
