package automationtransfer_usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"vozko/domain/actor"
	"vozko/domain/agent"
	"vozko/domain/automationneeds"
	"vozko/domain/automationtransfer"
	"vozko/domain/conversation"
	ce "vozko/domain/conversation_event"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	"vozko/domain/stage"
	"vozko/domain/workflow"
	ia_usecase "vozko/usecases/inbox_assignment"
)

var ErrStageNotRestored = errors.New("automation transfer: the conversation could not be put back in its stage")

type Agents interface {
	FindByID(agentID string) (*agent.Agent, error)
}

type Workflows interface {
	FindByID(workflowID string) (*workflow.Workflow, error)
}

type Governance interface {
	Governing(workspaceID, entryID string, entryType shared.EntryType) (conversation.Automation, bool, error)
}

type Delegations interface {
	CheckDelegation(ctx context.Context, in ia_usecase.SystemDelegation) error
	DelegateAsSystem(ctx context.Context, in ia_usecase.SystemDelegation) error
}

type Stages interface {
	stage.DestinationReader
	Move(workspaceID string, input stage.AssignEntryStageInput) (stage.Move, error)
}

type Placement interface {
	GetEntryStage(entryID, entryType, workspaceID string) (*stage.EntryStage, error)
}

type Broadcaster interface {
	BroadcastStageUpdate(workspaceID, entryID, entryType string)
}

type Timeline interface {
	Create(event *ce.ConversationEvent) error
	ListByEntryFiltered(workspaceID, entryID, entryType string, filter ce.ListFilter) ([]*ce.ConversationEvent, int64, error)
}

type Alerts interface {
	TransferUnmet(ctx context.Context, workspaceID string, refusal *automationneeds.Refusal, fromName string) error
}

type Deps struct {
	Needs       Needs
	Values      Values
	Variables   VariableStore
	Agents      Agents
	Workflows   Workflows
	Governance  Governance
	Delegations Delegations
	Stages      Stages
	Placement   Placement
	Remover     stage.RemoveEntryStageUseCase
	Broadcaster Broadcaster
	Timeline    Timeline
	Alerts      Alerts
	Now         func() time.Time
}

type Service struct {
	d Deps
}

func New(d Deps) (*Service, error) {
	if err := deps.Require("automation transfer", map[string]any{
		"agents": d.Agents, "workflows": d.Workflows, "governance": d.Governance, "delegations": d.Delegations,
		"stages": d.Stages, "placement": d.Placement, "stage remover": d.Remover, "broadcaster": d.Broadcaster,
		"timeline": d.Timeline, "alerts": d.Alerts, "needs": d.Needs, "values": d.Values, "variables": d.Variables,
	}); err != nil {
		return nil, err
	}
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{d: d}, nil
}

type Offer struct {
	Destination automationtransfer.Destination
	Label       string
	Description string
	Informs     []automationtransfer.VariableSource
}

type Request struct {
	WorkspaceID   string
	EntryID       string
	EntryType     shared.EntryType
	AgentID       string
	AgentName     string
	Config        map[string]interface{}
	DestinationID string
	Note          string
	Provided      map[string]string
}

type Done struct {
	Record automationtransfer.Record
	Label  string
}

type target struct {
	name        string
	description string
	stage       stage.FunnelStage
}

type problem struct{ err error }

func (p *problem) Error() string { return p.err.Error() }
func (p *problem) Unwrap() error { return p.err }

func Problem(err error) bool {
	var p *problem
	return errors.As(err, &p)
}

func Refused(err error) error { return &problem{err: err} }

func (s *Service) Offer(workspaceID, selfAgentID string, config map[string]interface{}) ([]Offer, error) {
	list, err := automationtransfer.Parse(config)
	if err != nil {
		return nil, err
	}
	offers := make([]Offer, 0, len(list))
	for _, d := range list {
		if automationtransfer.Check([]automationtransfer.Destination{d}, selfAgentID) != nil {
			continue
		}
		found, err := s.resolve(workspaceID, d)
		if err != nil {
			continue
		}
		informs, err := s.informs(workspaceID, d)
		if err != nil {
			continue
		}
		description := d.Description
		if description == "" {
			description = found.description
		}
		offers = append(offers, Offer{Destination: d, Label: automationtransfer.Label(found.name, found.stage), Description: description, Informs: informs})
	}
	return offers, nil
}

func (s *Service) CheckBinding(workspaceID, selfAgentID string, config map[string]interface{}) error {
	list, err := automationtransfer.Parse(config)
	if err != nil {
		return Refused(err)
	}
	if err := automationtransfer.Check(list, selfAgentID); err != nil {
		return Refused(err)
	}
	for i, d := range list {
		if _, err := s.resolve(workspaceID, d); err != nil {
			return automationtransfer.At(i+1, err)
		}
		if err := s.checkMapping(workspaceID, d); err != nil {
			return automationtransfer.At(i+1, err)
		}
	}
	return nil
}

func (s *Service) resolve(workspaceID string, d automationtransfer.Destination) (target, error) {
	found, err := s.automation(workspaceID, d.Target)
	if err != nil {
		return target{}, err
	}
	if d.StageID == "" {
		return found, nil
	}
	at, err := s.d.Stages.Destination(workspaceID, d.StageID)
	if automationtransfer.StageRefused(err) {
		return target{}, Refused(fmt.Errorf("%w: %w", automationtransfer.ErrStageInvalid, err))
	}
	if err != nil {
		return target{}, fmt.Errorf("read the destination stage %s: %w", d.StageID, err)
	}
	found.stage = at
	return found, nil
}

func (s *Service) automation(workspaceID string, a conversation.Automation) (target, error) {
	if a.Kind == conversation.AutomationWorkflow {
		w, err := s.d.Workflows.FindByID(a.ID)
		if err != nil {
			return target{}, fmt.Errorf("read workflow %s: %w", a.ID, err)
		}
		if cause := w.StartsConversationsIn(workspaceID); cause != nil {
			return target{}, Refused(workflowCause(cause))
		}
		return target{name: w.Name, description: strings.TrimSpace(w.Description)}, nil
	}
	found, err := s.d.Agents.FindByID(a.ID)
	if errors.Is(err, agent.ErrAgentNotFound) {
		return target{}, Refused(automationtransfer.ErrTargetUnusable)
	}
	if err != nil {
		return target{}, fmt.Errorf("read agent %s: %w", a.ID, err)
	}
	if !found.TakesConversationsIn(workspaceID) {
		return target{}, Refused(automationtransfer.ErrTargetUnusable)
	}
	return target{name: found.Name, description: strings.TrimSpace(found.Description)}, nil
}

func workflowCause(cause error) error {
	if errors.Is(cause, workflow.ErrWorkflowNotPublished) || errors.Is(cause, workflow.ErrWorkflowNotActive) {
		return cause
	}
	return fmt.Errorf("%w: %w", automationtransfer.ErrTargetUnusable, cause)
}

func (s *Service) Transfer(ctx context.Context, in Request) (Done, error) {
	if strings.TrimSpace(in.WorkspaceID) == "" || strings.TrimSpace(in.EntryID) == "" || in.EntryType == "" || strings.TrimSpace(in.AgentID) == "" {
		return Done{}, automationtransfer.ErrNoConversation
	}
	chosen, note, err := s.choice(in)
	if err != nil {
		return Done{}, err
	}
	if err := s.governs(in); err != nil {
		return Done{}, err
	}
	if err := s.belowLoopCap(in); err != nil {
		return Done{}, err
	}
	found, err := s.resolve(in.WorkspaceID, chosen)
	if err != nil {
		return Done{}, err
	}
	pending, filled, err := s.pendingValues(ctx, in, chosen)
	if err != nil {
		return Done{}, err
	}
	delegation := ia_usecase.SystemDelegation{WorkspaceID: in.WorkspaceID, EntryID: in.EntryID, EntryType: in.EntryType, Automation: chosen.Target, Pending: pending}
	if err := s.d.Delegations.CheckDelegation(ctx, delegation); err != nil {
		s.warnUnmet(ctx, in, err)
		return Done{}, err
	}
	move, moved, err := s.moveStage(in, found.stage)
	if err != nil {
		return Done{}, err
	}
	stored, err := s.storeValues(ctx, in, pending)
	if err != nil {
		return Done{}, s.undo(ctx, in, move, moved, storedValues{}, err)
	}
	if err := s.d.Delegations.DelegateAsSystem(ctx, delegation); err != nil {
		s.warnUnmet(ctx, in, err)
		return Done{}, s.undo(ctx, in, move, moved, stored, err)
	}
	if moved {
		s.d.Broadcaster.BroadcastStageUpdate(in.WorkspaceID, in.EntryID, string(in.EntryType))
	}
	record := automationtransfer.Record{
		From: conversation.Automation{Kind: conversation.AutomationAgent, ID: in.AgentID}, FromName: strings.TrimSpace(in.AgentName),
		To: chosen.Target, ToName: found.name, DestinationID: chosen.ID, Note: note, Stage: found.stage, Filled: filled,
	}
	s.write(in, record)
	return Done{Record: record, Label: automationtransfer.Label(found.name, found.stage)}, nil
}

func (s *Service) choice(in Request) (automationtransfer.Destination, string, error) {
	list, err := automationtransfer.Parse(in.Config)
	if err != nil {
		return automationtransfer.Destination{}, "", err
	}
	chosen, ok := automationtransfer.Find(list, in.DestinationID)
	if !ok {
		return automationtransfer.Destination{}, "", automationtransfer.ErrUnknownDestination
	}
	if err := automationtransfer.Check([]automationtransfer.Destination{chosen}, in.AgentID); err != nil {
		return automationtransfer.Destination{}, "", err
	}
	note, err := automationtransfer.NoteOf(in.Note)
	return chosen, note, err
}

func (s *Service) governs(in Request) error {
	governing, governed, err := s.d.Governance.Governing(in.WorkspaceID, in.EntryID, in.EntryType)
	if err != nil {
		return fmt.Errorf("read who governs %s %s: %w", in.EntryType, in.EntryID, err)
	}
	if !governed || governing != (conversation.Automation{Kind: conversation.AutomationAgent, ID: in.AgentID}) {
		return automationtransfer.ErrNotGoverning
	}
	return nil
}

func (s *Service) belowLoopCap(in Request) error {
	since := s.d.Now().Add(-automationtransfer.LoopWindow)
	_, recent, err := s.d.Timeline.ListByEntryFiltered(in.WorkspaceID, in.EntryID, string(in.EntryType), ce.ListFilter{
		EventType: ce.EventAutomationTransferred, Since: &since, Limit: 1,
	})
	if err != nil {
		return fmt.Errorf("count the transfers of %s %s: %w", in.EntryType, in.EntryID, err)
	}
	if automationtransfer.LoopReached(recent) {
		return automationtransfer.ErrLoop
	}
	return nil
}

func (s *Service) moveStage(in Request, to stage.FunnelStage) (stage.Move, bool, error) {
	if to.StageID == "" {
		return stage.Move{}, false, nil
	}
	current, err := s.d.Placement.GetEntryStage(in.EntryID, string(in.EntryType), in.WorkspaceID)
	if err == nil && current != nil && current.StageID == to.StageID {
		return stage.Move{}, false, nil
	}
	move, err := s.d.Stages.Move(in.WorkspaceID, s.stageInput(in, to.StageID))
	if err != nil {
		return stage.Move{}, false, fmt.Errorf("move %s %s to stage %s: %w", in.EntryType, in.EntryID, to.StageID, err)
	}
	return move, true, nil
}

func (s *Service) restoreStage(in Request, move stage.Move) error {
	if move.From.StageID == "" {
		return s.d.Remover.Execute(in.WorkspaceID, stage.RemoveEntryStageInput{
			StageID: move.To.StageID, EntryID: in.EntryID, EntryType: string(in.EntryType), ActorID: actor.FormatAI(in.AgentID),
		})
	}
	_, err := s.d.Stages.Move(in.WorkspaceID, s.stageInput(in, move.From.StageID))
	return err
}

func (s *Service) stageInput(in Request, stageID string) stage.AssignEntryStageInput {
	return stage.AssignEntryStageInput{
		StageID: stageID, EntryID: in.EntryID, EntryType: string(in.EntryType),
		ActorID: actor.FormatAI(in.AgentID), AllowCrossPipeline: true,
	}
}

func (s *Service) warnUnmet(ctx context.Context, in Request, err error) {
	refusal, unmet := automationneeds.RefusalOf(err)
	if !unmet {
		return
	}
	if alertErr := s.d.Alerts.TransferUnmet(ctx, in.WorkspaceID, refusal, in.AgentName); alertErr != nil {
		slog.Error("automation_transfer.alert_failed", slog.String("workspace_id", in.WorkspaceID), slog.String("entry_id", in.EntryID), slog.String("error", alertErr.Error()))
	}
}

func (s *Service) write(in Request, record automationtransfer.Record) {
	event := ce.New(in.WorkspaceID, in.EntryID, string(in.EntryType), ce.EventAutomationTransferred).
		WithActorAI(in.AgentID).
		WithChannel(in.EntryType.EventChannel()).
		WithDetails(record.Details()).
		Build()
	event.ID, event.CreatedAt = uuid.NewString(), s.d.Now()
	if err := s.d.Timeline.Create(event); err != nil {
		slog.Error("automation_transfer.timeline_unwritten",
			slog.String("workspace_id", in.WorkspaceID), slog.String("entry_id", in.EntryID),
			slog.String("to", record.To.ActorID()), slog.String("error", err.Error()))
	}
}

func (s *Service) HandOffLine(workspaceID, entryID, entryType, agentID string) string {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(entryID) == "" || strings.TrimSpace(agentID) == "" {
		return ""
	}
	now := s.d.Now()
	since := now.Add(-automationtransfer.NoteLifetime)
	events, _, err := s.d.Timeline.ListByEntryFiltered(workspaceID, entryID, entryType, ce.ListFilter{
		EventType: ce.EventAutomationTransferred, Since: &since, Limit: 1,
	})
	if err != nil {
		slog.Warn("automation_transfer.note_unreadable", slog.String("workspace_id", workspaceID), slog.String("entry_id", entryID), slog.String("error", err.Error()))
		return ""
	}
	if len(events) == 0 || events[0] == nil {
		return ""
	}
	record, ok := automationtransfer.RecordOf(events[0].DetailsMap())
	if !ok || record.To != (conversation.Automation{Kind: conversation.AutomationAgent, ID: agentID}) {
		return ""
	}
	return record.ContextLine(now.Sub(events[0].CreatedAt))
}
