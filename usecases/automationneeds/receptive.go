package automationneeds

import (
	"context"
	"errors"
	"log/slog"
	"time"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
)

var errReceptiveUnwired = errors.New("automation needs: the receptive guard needs the service, the switch-off records and the alerts")

type Receptive struct {
	service *Service
	records domain.SwitchOffRecords
	alerts  *Alerts
	now     func() time.Time
}

func NewReceptive(service *Service, records domain.SwitchOffRecords, alerts *Alerts) (*Receptive, error) {
	if service == nil || records == nil || alerts == nil {
		return nil, errReceptiveUnwired
	}
	return &Receptive{service: service, records: records, alerts: alerts, now: func() time.Time { return time.Now().UTC() }}, nil
}

var _ domain.ReceptiveGuard = (*Receptive)(nil)

func (r *Receptive) Admit(workspaceID string, kind domain.SlotKind, config conversation.ChannelAutomation) error {
	return r.service.AdmitReceptive(workspaceID, kind, config)
}

func (r *Receptive) Saved(ctx context.Context, workspaceID string, kind domain.SlotKind, slotID string, config conversation.ChannelAutomation) {
	attrs := []any{slog.String("workspace_id", workspaceID), slog.String("slot_kind", string(kind)), slog.String("slot_id", slotID)}
	open, err := r.records.Open(ctx, workspaceID, kind, slotID)
	if err != nil {
		slog.Error("automation.switch_off.unreadable", append(attrs, slog.String("error", err.Error()))...)
		return
	}
	if open == nil || !open.SettledBy(config) {
		return
	}
	if _, err := r.records.Resolve(ctx, workspaceID, kind, slotID, r.now()); err != nil {
		slog.Error("automation.switch_off.resolve_failed", append(attrs, slog.String("error", err.Error()))...)
		return
	}
	if err := r.alerts.SwitchOffResolved(ctx, workspaceID, kind, slotID); err != nil {
		slog.Error("automation.switch_off.episode_resolve_failed", append(attrs, slog.String("error", err.Error()))...)
	}
}
