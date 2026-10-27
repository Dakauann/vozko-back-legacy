package automationneeds

import (
	"context"
	"errors"
	"log/slog"

	"vozko/domain/agent"
	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	ia "vozko/domain/inbox_assignment"
	"vozko/domain/workflow"
)

var errPublishGuardUnwired = errors.New("automation needs: the publish guard needs the service, the slots, the alerts, the held conversations and the hand-off")

type confirmedKey struct{}

func WithSwitchOffConfirmed(ctx context.Context) context.Context {
	return context.WithValue(ctx, confirmedKey{}, true)
}

func SwitchOffConfirmed(ctx context.Context) bool {
	confirmed, _ := ctx.Value(confirmedKey{}).(bool)
	return confirmed
}

type HeldConversation struct {
	EntryID   string
	EntryType string
}

type HeldConversations interface {
	HeldOn(ctx context.Context, workspaceID string, automation conversation.Automation, slotID string, limit int) ([]HeldConversation, error)
}

type RouletteHandOff interface {
	HandOffToRoulette(in ia.RouletteHandOff) (string, error)
}

type ContainerCache interface {
	Forget(campaignID string)
}

type PublishGuardDeps struct {
	Service    *Service
	Slots      domain.Slots
	Alerts     *Alerts
	Held       HeldConversations
	HandOff    RouletteHandOff
	Containers ContainerCache
}

type PublishGuard struct {
	d PublishGuardDeps
}

func NewPublishGuard(d PublishGuardDeps) (*PublishGuard, error) {
	if d.Service == nil || d.Slots == nil || d.Alerts == nil || d.Held == nil || d.HandOff == nil || d.Containers == nil {
		return nil, errPublishGuardUnwired
	}
	return &PublishGuard{d: d}, nil
}

type Plan struct {
	WorkspaceID string
	Automation  conversation.Automation
	Offs        []domain.SwitchOff
}

func (g *PublishGuard) PlanAgent(ctx context.Context, draft *agent.Agent) (Plan, error) {
	if draft == nil {
		return Plan{}, domain.ErrNotFound
	}
	automation := conversation.Automation{Kind: conversation.AutomationAgent, ID: draft.ID}
	return g.plan(ctx, draft.WorkspaceID, automation, domain.OverlayAgent(g.d.Service.Source(), draft))
}

func (g *PublishGuard) PlanWorkflow(ctx context.Context, draft *workflow.Workflow) (Plan, error) {
	if draft == nil {
		return Plan{}, domain.ErrNotFound
	}
	automation := conversation.Automation{Kind: conversation.AutomationWorkflow, ID: draft.ID}
	src := domain.OverlayWorkflow(g.d.Service.Source(), draft)
	if draft.Type == workflow.WorkflowTypeVoice {
		needs, err := domain.Of(src, draft.WorkspaceID, automation)
		if err != nil {
			return Plan{}, err
		}
		if len(needs) > 0 {
			return Plan{}, &domain.Refusal{Cause: domain.ErrVoiceVariables, Automation: rootOf(automation, needs), Needs: needs}
		}
	}
	return g.plan(ctx, draft.WorkspaceID, automation, src)
}

func (g *PublishGuard) Impact(ctx context.Context, workspaceID string, automation conversation.Automation, src domain.Source) ([]domain.SwitchOff, error) {
	slots, err := g.d.Slots.InWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, errors.Join(domain.ErrUnreadable, err)
	}
	var affected []domain.Slot
	for _, slot := range slots {
		var running []conversation.Automation
		for _, r := range slot.Running {
			if g.dependsOn(src, workspaceID, r, automation) {
				running = append(running, r)
			}
		}
		if len(running) > 0 {
			slot.Running = running
			affected = append(affected, slot)
		}
	}
	return domain.Impact(affected, func(a conversation.Automation) (domain.Needs, error) {
		return domain.Of(src, workspaceID, a)
	})
}

func (g *PublishGuard) dependsOn(src domain.Source, workspaceID string, running, automation conversation.Automation) bool {
	if running == automation {
		return true
	}
	depends, err := domain.DependsOn(src, workspaceID, running, automation)
	if err != nil {
		slog.Warn("automation.publish.slot_unreadable", slog.String("workspace_id", workspaceID),
			slog.String("automation_kind", string(running.Kind)), slog.String("automation_id", running.ID), slog.String("error", err.Error()))
		return false
	}
	return depends
}

func (g *PublishGuard) plan(ctx context.Context, workspaceID string, automation conversation.Automation, src domain.Source) (Plan, error) {
	offs, err := g.Impact(ctx, workspaceID, automation, src)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{WorkspaceID: workspaceID, Automation: automation, Offs: offs}
	if len(offs) == 0 || SwitchOffConfirmed(ctx) {
		return plan, nil
	}
	var needs domain.Needs
	for _, off := range offs {
		needs = needs.Union(off.Needs)
	}
	return Plan{}, &domain.Refusal{Cause: domain.ErrPublishSwitchesOff, Automation: domain.Link{Automation: automation}, Needs: needs, Impact: offs}
}

const heldBatch = 200

func (g *PublishGuard) Done(ctx context.Context, plan Plan, containers []string) {
	for _, id := range containers {
		g.d.Containers.Forget(id)
	}
	if err := g.d.Alerts.UnmetResolved(ctx, plan.WorkspaceID, plan.Automation); err != nil {
		slog.Error("automation.publish.unmet_resolve_failed", slog.String("workspace_id", plan.WorkspaceID), slog.String("error", err.Error()))
	}
	for _, off := range plan.Offs {
		if err := g.d.Alerts.SwitchedOff(ctx, plan.WorkspaceID, off); err != nil {
			slog.Error("automation.switch_off.alert_failed", slog.String("workspace_id", plan.WorkspaceID), slog.String("slot_id", off.Slot.ID), slog.String("error", err.Error()))
		}
		g.handOffHeld(ctx, plan.WorkspaceID, off)
	}
}

func (g *PublishGuard) handOffHeld(ctx context.Context, workspaceID string, off domain.SwitchOff) {
	attrs := []any{slog.String("workspace_id", workspaceID), slog.String("slot_kind", string(off.Slot.Kind)), slog.String("slot_id", off.Slot.ID), slog.String("automation_id", off.Automation.ID)}
	handed := map[string]bool{}
	for {
		held, err := g.d.Held.HeldOn(ctx, workspaceID, off.Automation.Automation, off.Slot.ID, heldBatch)
		if err != nil {
			slog.Error("automation.switch_off.held_unreadable", append(attrs, slog.String("error", err.Error()))...)
			return
		}
		progressed := false
		for _, h := range held {
			key := h.EntryType + ":" + h.EntryID
			if handed[key] {
				continue
			}
			handed[key] = true
			progressed = true
			if _, err := g.d.HandOff.HandOffToRoulette(ia.RouletteHandOff{WorkspaceID: workspaceID, EntryID: h.EntryID, EntryType: h.EntryType, ByActorID: off.Automation.ActorID()}); err != nil {
				slog.Error("automation.switch_off.hand_off_failed", append(attrs, slog.String("entry_id", h.EntryID), slog.String("error", err.Error()))...)
			}
		}
		if !progressed || len(held) < heldBatch {
			slog.Info("automation.switch_off.handed_off", append(attrs, slog.Int("conversations", len(handed)))...)
			return
		}
	}
}
