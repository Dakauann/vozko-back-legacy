package container

import (
	"log"

	"vozko/delivery/http/handlers"
	wc_domain "vozko/domain/whatsapp_campaign"
	"vozko/infra/repositories/campaignstamp"
	wc_usecase "vozko/usecases/whatsapp_campaign"
)

func (c *Container) officialCampaignLaunch() handlers.CampaignLaunchDeps {
	start, ok := c.useCases.startWCCampaign.(wc_domain.ConfirmedStartUseCase)
	if !ok {
		log.Fatalf("[launch-check] the official campaign start cannot carry the confirmed launch check")
	}
	live := c.campaignLive("whatsapp_campaign", c.repositories.wcEntry, campaignstamp.OfficialEntries)
	return handlers.CampaignLaunchDeps{
		Access:    c.useCases.wcCampaignAccess,
		Start:     start,
		Progress:  live.progress,
		Timeline:  live.timeline,
		Scheduled: c.officialDraftLaunch(),
	}
}

func (c *Container) numberReceptive() *wc_usecase.NumberReceptive {
	return wc_usecase.NewNumberReceptive(c.repositories.businessPhone, c.repositories.wcCampaign, c.useCases.ensureReceptiveContainer)
}
