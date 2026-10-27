package scheduled_message_usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	sm "vozko/domain/scheduled_message"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
)

type WorkspaceZones interface {
	Location(ctx context.Context, workspaceID, departmentID string) (*time.Location, error)
}

type FollowUpsDeps struct {
	Repo     sm.Repository
	Schedule sm.ScheduleUseCase
	Gate     AutomationStanding
	Zones    WorkspaceZones
	Windows  sm.WindowReader
	Clock    sm.Clock
}

type followUps struct {
	repo     sm.Repository
	schedule sm.ScheduleUseCase
	gate     AutomationStanding
	zones    WorkspaceZones
	windows  sm.WindowReader
	clock    sm.Clock
}

func NewFollowUps(d FollowUpsDeps) (sm.FollowUpUseCase, error) {
	if err := deps.Require("follow-ups", map[string]any{
		"repository":      d.Repo,
		"schedule":        d.Schedule,
		"automation gate": d.Gate,
		"workspace zones": d.Zones,
		"window reader":   d.Windows,
		"clock":           d.Clock,
	}); err != nil {
		return nil, err
	}
	return &followUps{repo: d.Repo, schedule: d.Schedule, gate: d.Gate, zones: d.Zones, windows: d.Windows, clock: d.Clock}, nil
}

func (f *followUps) Schedule(ctx context.Context, in sm.FollowUpInput) (*sm.FollowUpScheduled, error) {
	if !sm.ValidAutomationAuthor(in.Author) {
		return nil, sm.ErrAutomationAuthorInvalid
	}
	loc, err := f.zones.Location(ctx, in.WorkspaceID, "")
	if err != nil {
		return nil, fmt.Errorf("follow-up: the workspace time zone could not be read: %w", err)
	}
	if replayed, err := f.replay(in, loc); replayed != nil || err != nil {
		return replayed, err
	}
	if err := f.admit(in); err != nil {
		return nil, err
	}

	now := f.clock.Now()
	at, err := in.When.Resolve(now, loc)
	if err != nil {
		return nil, err
	}
	live := f.windows.GetWindowStatusForEntry(in.EntryID, in.EntryType)
	kind, err := sm.FollowUpKind(at, live.Open, live.ExpiresAt, now, shared.EntryType(in.EntryType), hasTemplate(in.Fallback))
	if err != nil {
		return nil, err
	}

	result, err := f.schedule.Execute(ctx, scheduleOf(in, kind, at))
	if err != nil {
		return nil, err
	}
	return &sm.FollowUpScheduled{Message: result.Message, Local: result.Message.ScheduledAt.In(loc), Replayed: result.AlreadyExisted}, nil
}

func (f *followUps) replay(in sm.FollowUpInput, loc *time.Location) (*sm.FollowUpScheduled, error) {
	key := strings.TrimSpace(in.IdempotencyKey)
	if key == "" {
		return nil, nil
	}
	existing, err := f.repo.FindByIdempotencyKey(in.WorkspaceID, key)
	if errors.Is(err, sm.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sm.FollowUpScheduled{Message: existing, Local: existing.ScheduledAt.In(loc), Replayed: true}, nil
}

func (f *followUps) admit(in sm.FollowUpInput) error {
	mayAct, err := f.gate.MayAct(in.WorkspaceID, in.EntryID, shared.EntryType(in.EntryType))
	if err != nil {
		return fmt.Errorf("follow-up: who may answer this conversation could not be read: %w", err)
	}
	if !mayAct {
		return sm.ErrFollowUpNotAllowed
	}
	return nil
}

func hasTemplate(t *sm.TemplateContent) bool {
	return t != nil && strings.TrimSpace(t.ID) != ""
}

func scheduleOf(in sm.FollowUpInput, kind sm.Kind, at time.Time) sm.ScheduleInput {
	out := sm.ScheduleInput{
		WorkspaceID:         in.WorkspaceID,
		EntryID:             in.EntryID,
		EntryType:           in.EntryType,
		CreatedByAutomation: in.Author,
		CancelOnReply:       in.CancelOnReply,
		ScheduledAt:         at,
		IdempotencyKey:      in.IdempotencyKey,
	}
	if kind == sm.KindTemplate {
		out.Template = &sm.TemplateContent{ID: in.Fallback.ID, BodyParams: in.Fallback.BodyParams, HeaderParams: in.Fallback.HeaderParams}
		return out
	}
	out.Text = in.Text
	return out
}

func (f *followUps) Cancel(_ context.Context, in sm.FollowUpCancel) ([]*sm.ScheduledMessage, error) {
	if !sm.ValidAutomationAuthor(in.Author) {
		return nil, sm.ErrAutomationAuthorInvalid
	}
	if id := strings.TrimSpace(in.ID); id != "" {
		cancelled, err := f.cancelOne(in, id)
		if err != nil {
			return nil, err
		}
		return []*sm.ScheduledMessage{cancelled}, nil
	}
	return f.cancelAll(in)
}

func (f *followUps) cancelOne(in sm.FollowUpCancel, id string) (*sm.ScheduledMessage, error) {
	message, err := f.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if !ownFollowUp(message, in) {
		return nil, sm.ErrNotFound
	}
	if message.Status != sm.StatusPending {
		return nil, sm.ErrNotPending
	}
	if err := f.repo.Cancel(message.ID, sm.CancelByAutomation); err != nil {
		return nil, err
	}
	return cancelledCopy(message), nil
}

func (f *followUps) cancelAll(in sm.FollowUpCancel) ([]*sm.ScheduledMessage, error) {
	pending, err := f.repo.ListByEntry(in.EntryID, in.EntryType, []sm.Status{sm.StatusPending})
	if err != nil {
		return nil, err
	}
	cancelled := []*sm.ScheduledMessage{}
	for _, message := range pending {
		if !ownFollowUp(message, in) {
			continue
		}
		err := f.repo.Cancel(message.ID, sm.CancelByAutomation)
		if errors.Is(err, sm.ErrNotPending) {
			log.Printf("[scheduled_message] follow-up %s left pending before %s could cancel it", message.ID, in.Author)
			continue
		}
		if err != nil {
			return cancelled, err
		}
		cancelled = append(cancelled, cancelledCopy(message))
	}
	return cancelled, nil
}

func ownFollowUp(m *sm.ScheduledMessage, in sm.FollowUpCancel) bool {
	return m != nil &&
		m.WorkspaceID == in.WorkspaceID &&
		m.EntryID == in.EntryID &&
		string(m.EntryType) == in.EntryType &&
		m.CreatedByAutomation == in.Author
}

func cancelledCopy(m *sm.ScheduledMessage) *sm.ScheduledMessage {
	copied := *m
	reason := sm.CancelByAutomation
	copied.Status = sm.StatusCanceled
	copied.CancelReason = &reason
	return &copied
}

type replyCancellation struct {
	repo sm.Repository
}

func NewReplyCancellation(repo sm.Repository) (sm.ReplyCancellation, error) {
	if err := deps.Require("follow-up reply cancellation", map[string]any{"repository": repo}); err != nil {
		return nil, err
	}
	return replyCancellation{repo: repo}, nil
}

func (r replyCancellation) ContactReplied(entryID, entryType string, at time.Time) error {
	cancelled, err := r.repo.CancelOnReply(entryID, entryType, at)
	if err != nil {
		return fmt.Errorf("follow-ups of %s %s were not cancelled after the contact replied: %w", entryType, entryID, err)
	}
	if cancelled > 0 {
		log.Printf("[scheduled_message] the contact replied on %s (%s): %d follow-up(s) cancelled", entryID, entryType, cancelled)
	}
	return nil
}

var (
	_ sm.FollowUpUseCase   = (*followUps)(nil)
	_ sm.ReplyCancellation = replyCancellation{}
)
