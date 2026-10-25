package container

import (
	"log"

	"vozko/domain/campaign/scheduledstart"
	uwc "vozko/domain/unofficial_whatsapp_campaign"
	"vozko/domain/workspace"
	"vozko/usecases/campaignschedule"
)

func (c *Container) scheduleReporter(channel campaignschedule.Channel, failures scheduledstart.Recorder, prefix string) *campaignschedule.Reporter {
	reporter, err := campaignschedule.New(campaignschedule.Deps{
		Channel:       channel,
		Failures:      failures,
		Notifications: c.notifications().Publisher,
		Logf: func(format string, args ...any) {
			log.Printf(prefix+" "+format, args...)
		},
	})
	if err != nil {
		log.Fatalf("%s failed schedules cannot be reported: %v", prefix, err)
	}
	return reporter
}

func (c *Container) officialScheduleReporter() *campaignschedule.Reporter {
	if c.officialSchedules != nil {
		return c.officialSchedules
	}
	failures, ok := c.repositories.wcCampaign.(scheduledstart.Recorder)
	if !ok {
		log.Fatalf("[whatsapp-campaign] the campaign repository cannot record failed schedules")
	}
	c.officialSchedules = c.scheduleReporter(campaignschedule.Channel{
		Key: "whatsapp", Label: "Campanha oficial", Path: "/dashboard/whatsapp-campaigns/", Resource: workspace.ResourceWhatsAppCampaigns,
	}, failures, "[whatsapp-campaign]")
	return c.officialSchedules
}

func (c *Container) unofficialScheduleReporter(campaigns uwc.Repository) *campaignschedule.Reporter {
	failures, ok := campaigns.(scheduledstart.Recorder)
	if !ok {
		log.Fatalf("[unofficial-whatsapp-campaign] the campaign repository cannot record failed schedules")
	}
	return c.scheduleReporter(campaignschedule.Channel{
		Key: "unofficial_whatsapp", Label: "Campanha não oficial", Path: "/dashboard/unofficial-whatsapp-campaigns/", Resource: workspace.ResourceUnofficialWhatsAppCampaigns,
	}, failures, "[unofficial-whatsapp-campaign]")
}
