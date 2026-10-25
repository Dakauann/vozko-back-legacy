package whatsapp_campaign_usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	wc "vozko/domain/whatsapp_campaign"
	"vozko/usecases/campaignschedule"
)

const scheduleStartBatchSize = 200

var ErrScheduleStartUnavailable = errors.New("whatsapp campaign scheduled start: the failure reporter is not configured")

type ScheduleStartJob struct {
	dispatchCampaignUseCase wc.DispatchCampaignUseCase
	whatsappCampaingsRepo   wc.Repository
	schedules               wc.ScheduleClaimer
	reporter                *campaignschedule.Reporter
}

func NewScheduleStartJob(dispatchCampaignUseCase wc.DispatchCampaignUseCase, whatsappCampaingsRepo wc.Repository, schedules wc.ScheduleClaimer, reporter *campaignschedule.Reporter) wc.StartScheduleJob {
	return &ScheduleStartJob{
		dispatchCampaignUseCase: dispatchCampaignUseCase,
		whatsappCampaingsRepo:   whatsappCampaingsRepo,
		schedules:               schedules,
		reporter:                reporter,
	}
}

func (j *ScheduleStartJob) StartScheduledWhatsappCampaigns() error {
	if j.reporter == nil {
		return ErrScheduleStartUnavailable
	}
	now := time.Now().UTC()
	campaigns, err := j.whatsappCampaingsRepo.ListScheduledToStart(now, scheduleStartBatchSize)
	if err != nil {
		return err
	}

	var errs []error
	for _, campaign := range campaigns {
		if campaign == nil || campaign.ID == "" {
			continue
		}
		claimed, err := j.schedules.ClaimScheduledStart(campaign.ID, now)
		if err != nil {
			errs = append(errs, fmt.Errorf("claim the schedule of %s: %w", campaign.ID, err))
			continue
		}
		if !claimed {
			continue
		}
		err = j.dispatchCampaignUseCase.Dispatch(wc.DispatchCampaignInput{
			CampaignID:           campaign.ID,
			Action:               wc.CampaignActionStart,
			ConfirmedFingerprint: campaign.ScheduleConfirmedFingerprint,
		})
		if err == nil || errors.Is(err, wc.ErrDispatchCampaignAlreadyRunning) {
			continue
		}
		if err := j.reporter.Failed(context.Background(), attemptOf(campaign), err); err != nil {
			errs = append(errs, fmt.Errorf("record the failed schedule of %s: %w", campaign.ID, err))
		}
	}

	return errors.Join(errs...)
}

func attemptOf(c *wc.Campaign) campaignschedule.Attempt {
	return campaignschedule.Attempt{WorkspaceID: c.WorkspaceID, CampaignID: c.ID, CampaignName: c.Name, ScheduledBy: c.ScheduledBy, ScheduledAt: c.ScheduledStart}
}

var _ wc.StartScheduleJob = (*ScheduleStartJob)(nil)
