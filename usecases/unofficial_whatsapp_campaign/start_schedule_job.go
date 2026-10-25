package unofficial_whatsapp_campaign

import (
	"context"
	"errors"
	"fmt"
	"time"

	"vozko/domain/campaign"
	uwc "vozko/domain/unofficial_whatsapp_campaign"
	"vozko/usecases/campaignschedule"
)

const scheduleStartBatchSize = 200

var ErrScheduleStartUnavailable = errors.New("unofficial whatsapp campaign scheduled start: the failure reporter is not configured")

type scheduleStartJob struct {
	campaigns uwc.Repository
	dispatch  uwc.DispatchCampaignUseCase
	schedules campaign.ScheduleClaimer
	reporter  *campaignschedule.Reporter
}

func NewScheduleStartJob(
	campaigns uwc.Repository,
	dispatch uwc.DispatchCampaignUseCase,
	schedules campaign.ScheduleClaimer,
	reporter *campaignschedule.Reporter,
) uwc.StartScheduleJob {
	return &scheduleStartJob{campaigns: campaigns, dispatch: dispatch, schedules: schedules, reporter: reporter}
}

func (j *scheduleStartJob) StartScheduledCampaigns() error {
	if j.reporter == nil {
		return ErrScheduleStartUnavailable
	}
	now := time.Now().UTC()
	due, err := j.campaigns.ListScheduledToStart(now, scheduleStartBatchSize)
	if err != nil {
		return err
	}

	ctx := context.Background()
	var errs []error
	for _, camp := range due {
		if camp == nil || camp.ID == "" {
			continue
		}
		claimed, err := j.schedules.ClaimScheduledStart(camp.ID, now)
		if err != nil {
			errs = append(errs, fmt.Errorf("claim the schedule of %s: %w", camp.ID, err))
			continue
		}
		if !claimed {
			continue
		}
		err = j.dispatch.Dispatch(ctx, uwc.DispatchCampaignInput{
			CampaignID: camp.ID,
			Action:     campaign.ActionStart,
		})
		if err == nil || errors.Is(err, campaign.ErrAlreadyRunning) {
			continue
		}
		if err := j.reporter.Failed(ctx, attemptOf(camp), err); err != nil {
			errs = append(errs, fmt.Errorf("record the failed schedule of %s: %w", camp.ID, err))
		}
	}
	return errors.Join(errs...)
}

func attemptOf(c *uwc.Campaign) campaignschedule.Attempt {
	return campaignschedule.Attempt{WorkspaceID: c.WorkspaceID, CampaignID: c.ID, CampaignName: c.Name, ScheduledBy: c.CreatedByID, ScheduledAt: c.ScheduledStart}
}
