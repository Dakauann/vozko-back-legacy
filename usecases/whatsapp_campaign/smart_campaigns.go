package whatsapp_campaign_usecase

import "context"

type SmartCampaigns interface {
	Manages(campaignID string) (bool, error)
}

type SmartStarter interface {
	SmartCampaigns
	Start(ctx context.Context, campaignID string) error
}

func (s campaignStatusStore) ownedByWaves(campaignID string) (bool, error) {
	return s.owner.smart.Manages(campaignID)
}
