package whatsapp_campaign_usecase

import (
	"context"
	"fmt"
	"log"
	"time"

	"vozko/domain/campaign/scheduledstart"
	"vozko/domain/shared/deps"
	wc "vozko/domain/whatsapp_campaign"
	wd "vozko/domain/workspace/workspace_department"
	"vozko/domain/workspace/workspace_plan"
)

type StartCampaignDeps struct {
	Access       wc.CampaignAccessUseCase
	Subscription workspace_plan.EnsureActiveWorkspaceSubscriptionUseCase
	Dispatch     wc.DispatchCampaignUseCase
	Schedules    wc.ScheduleClaimer
	Reporter     ScheduleReporter
}

type ScheduleReporter interface {
	Cleared(ctx context.Context, campaignID string, current scheduledstart.Failure)
}

type startCampaign struct {
	deps StartCampaignDeps
}

func NewStartCampaignUseCase(d StartCampaignDeps) (wc.StartCampaignUseCase, error) {
	if err := deps.Require("official campaign start", map[string]any{
		"campaign access":   d.Access,
		"subscription":      d.Subscription,
		"dispatch":          d.Dispatch,
		"schedules":         d.Schedules,
		"schedule reporter": d.Reporter,
	}); err != nil {
		return nil, err
	}
	return &startCampaign{deps: d}, nil
}

func (uc *startCampaign) Start(workspaceID string, departments *wd.DepartmentFilter, campaignID string) (*wc.Campaign, error) {
	return uc.start(workspaceID, departments, campaignID, false, "")
}

func (uc *startCampaign) StartConfirmed(workspaceID string, departments *wd.DepartmentFilter, campaignID, confirmedFingerprint string) (*wc.Campaign, error) {
	return uc.start(workspaceID, departments, campaignID, false, confirmedFingerprint)
}

func (uc *startCampaign) StartReviewed(workspaceID string, departments *wd.DepartmentFilter, campaignID string) (*wc.Campaign, error) {
	return uc.start(workspaceID, departments, campaignID, true, "")
}

func (uc *startCampaign) start(workspaceID string, departments *wd.DepartmentFilter, campaignID string, reviewed bool, confirmedFingerprint string) (*wc.Campaign, error) {
	campaign, err := uc.deps.Access.Owned(workspaceID, departments, campaignID)
	if err != nil {
		return nil, err
	}
	if _, err := uc.deps.Subscription.Execute(campaign.WorkspaceID); err != nil {
		return nil, fmt.Errorf("%w: %v", wc.ErrCampaignNoSubscription, err)
	}
	if err := campaign.Startable(); err != nil {
		return nil, err
	}
	if err := uc.deps.Dispatch.Dispatch(wc.DispatchCampaignInput{CampaignID: campaign.ID, Action: wc.CampaignActionStart, Reviewed: reviewed, ConfirmedFingerprint: confirmedFingerprint}); err != nil {
		return nil, err
	}
	if _, err := uc.deps.Schedules.ClaimScheduledStart(campaign.ID, time.Now().AddDate(10, 0, 0)); err != nil {
		log.Printf("[whatsapp-campaign] started %s but could not consume its schedule: %v", campaign.ID, err)
	}
	uc.deps.Reporter.Cleared(context.Background(), campaign.ID, campaign.ScheduleFailure)
	return campaign, nil
}
