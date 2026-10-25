package campaignlaunch

import (
	"context"
	"errors"

	"vozko/domain/campaign"
	"vozko/domain/campaign/launchcheck"
	"vozko/domain/campaign/scheduledstart"
)

var (
	ErrPlanIncomplete = errors.New("campaign launch: the plan is missing a step")
)

type Plan[C any] struct {
	Launch               campaign.Launch
	ConfirmedFingerprint string
	Check                func(ctx context.Context) (launchcheck.Report, error)
	Create               func(ctx context.Context, schedule scheduledstart.Schedule) (C, error)
	Settle               func(ctx context.Context, created C) []string
	Start                func(ctx context.Context, created C) error
}

type Outcome[C any] struct {
	Campaign     C
	Started      bool
	Warnings     []string
	StartRefusal error
}

func (p Plan[C]) complete() bool {
	if p.Create == nil || (p.Launch.Starts() && p.Check == nil) {
		return false
	}
	return !p.Launch.StartsNow() || p.Start != nil
}

func (p Plan[C]) admit(ctx context.Context) (scheduledstart.Schedule, error) {
	if !p.Launch.Starts() {
		return scheduledstart.Schedule{}, nil
	}
	report, err := p.Check(ctx)
	if err != nil {
		return scheduledstart.Schedule{}, err
	}
	if p.Launch.StartsNow() {
		return scheduledstart.Schedule{}, report.Admit(p.ConfirmedFingerprint)
	}
	return scheduledstart.Admit(p.Launch.At, report, p.ConfirmedFingerprint)
}

func Run[C any](ctx context.Context, p Plan[C]) (Outcome[C], error) {
	var none Outcome[C]
	if !p.complete() {
		return none, ErrPlanIncomplete
	}
	schedule, err := p.admit(ctx)
	if err != nil {
		return none, err
	}
	created, err := p.Create(ctx, schedule)
	if err != nil {
		return none, err
	}
	out := Outcome[C]{Campaign: created}
	if p.Settle != nil {
		out.Warnings = p.Settle(ctx, created)
	}
	if !p.Launch.StartsNow() {
		return out, nil
	}
	if len(out.Warnings) > 0 {
		out.StartRefusal = campaign.ErrSettingsNotApplied
		return out, nil
	}
	if err := p.Start(ctx, created); err != nil {
		out.StartRefusal = err
		return out, nil
	}
	out.Started = true
	return out, nil
}
