package stage

import (
	"errors"
	"strings"
)

var ErrStageNotConversationFunnel = errors.New("stage: the stage belongs to an opportunity funnel, a conversation cannot move there")

type FunnelStage struct {
	StageID      string
	StageName    string
	PipelineID   string
	PipelineName string
}

type Move struct {
	Entry *EntryStage
	From  FunnelStage
	To    FunnelStage
}

func (m Move) FunnelChanged() bool {
	return CrossesFunnel(m.From.PipelineID, m.To.PipelineID)
}

func CrossesFunnel(fromPipelineID, toPipelineID string) bool {
	from, to := strings.TrimSpace(fromPipelineID), strings.TrimSpace(toPipelineID)
	return from != "" && to != "" && from != to
}

type DestinationReader interface {
	Destination(workspaceID, stageID string) (FunnelStage, error)
}

type EntryMover interface {
	AssignEntryStageUseCase
	DestinationReader
	Move(workspaceID string, input AssignEntryStageInput) (Move, error)
}

const moveFailedCode = "stage_move_failed"

var moveRefusalCodes = []struct {
	code string
	errs []error
}{
	{"stage_not_conversation_funnel", []error{ErrStageNotConversationFunnel}},
	{"stage_not_found", []error{ErrUnauthorized, ErrTagNotFound}},
	{"stage_other_funnel", []error{ErrStagePipelineMismatch}},
	{"stage_entry_access", []error{ErrEntryAccess}},
	{"stage_invalid_entry", []error{ErrInvalidEntryType}},
}

func MoveRefusalCode(err error) string {
	for _, entry := range moveRefusalCodes {
		for _, known := range entry.errs {
			if errors.Is(err, known) {
				return entry.code
			}
		}
	}
	return moveFailedCode
}

func MoveRefusalCodes() []string {
	codes := make([]string, 0, len(moveRefusalCodes)+1)
	for _, entry := range moveRefusalCodes {
		codes = append(codes, entry.code)
	}
	return append(codes, moveFailedCode)
}
