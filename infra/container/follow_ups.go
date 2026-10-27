package container

import (
	"context"

	scheduled_message_domain "vozko/domain/scheduled_message"
	"vozko/domain/shared/deps"
)

func (c *Container) followUpsHandle() *deps.Handle[scheduled_message_domain.FollowUpUseCase] {
	if c.followUps == nil {
		c.followUps = deps.Tracked[scheduled_message_domain.FollowUpUseCase](&c.handles, "automation follow-ups")
	}
	return c.followUps
}

type lateFollowUps struct {
	handle *deps.Handle[scheduled_message_domain.FollowUpUseCase]
}

func (l lateFollowUps) Schedule(ctx context.Context, in scheduled_message_domain.FollowUpInput) (*scheduled_message_domain.FollowUpScheduled, error) {
	followUps, err := l.handle.Get()
	if err != nil {
		return nil, err
	}
	return followUps.Schedule(ctx, in)
}

func (l lateFollowUps) Cancel(ctx context.Context, in scheduled_message_domain.FollowUpCancel) ([]*scheduled_message_domain.ScheduledMessage, error) {
	followUps, err := l.handle.Get()
	if err != nil {
		return nil, err
	}
	return followUps.Cancel(ctx, in)
}

var _ scheduled_message_domain.FollowUpUseCase = lateFollowUps{}
