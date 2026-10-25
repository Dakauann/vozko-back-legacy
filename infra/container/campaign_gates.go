package container

import (
	"context"
	"log"

	"vozko/domain/buttonaction"
	"vozko/domain/campaign/launchcheck"
	"vozko/domain/shared/deps"
	"vozko/domain/whatsapp/template"
	wc "vozko/domain/whatsapp_campaign"
	wc_usecase "vozko/usecases/whatsapp_campaign"
)

type officialCampaignGates struct {
	exitRisk *deps.Handle[wc_usecase.ExitRiskGate]
	launch   *deps.Handle[*wc_usecase.LaunchCheck]
}

func (c *Container) campaignGates() *officialCampaignGates {
	if c.officialGates == nil {
		c.officialGates = &officialCampaignGates{
			exitRisk: deps.Tracked[wc_usecase.ExitRiskGate](&c.handles, "official campaign exit risk gate"),
			launch:   deps.Tracked[*wc_usecase.LaunchCheck](&c.handles, "official campaign launch check"),
		}
	}
	return c.officialGates
}

func (c *Container) bindCampaignGates() {
	gates := c.campaignGates()
	if err := gates.exitRisk.Bind(c.buttonActions()); err != nil {
		log.Fatalf("[campaigns] %v", err)
	}
	if err := gates.launch.Bind(c.launchCheck()); err != nil {
		log.Fatalf("[campaigns] %v", err)
	}
}

type lateExitRisk struct {
	gate *deps.Handle[wc_usecase.ExitRiskGate]
}

func (l lateExitRisk) AdmitCampaign(ctx context.Context, workspaceID string, tmpl *template.Template, acknowledgedBy string) (buttonaction.CampaignAdmission, error) {
	gate, err := l.gate.Get()
	if err != nil {
		return buttonaction.CampaignAdmission{}, err
	}
	return gate.AdmitCampaign(ctx, workspaceID, tmpl, acknowledgedBy)
}

func (l lateExitRisk) RecordCampaignAdmission(ctx context.Context, workspaceID, campaignID string, admission buttonaction.CampaignAdmission) error {
	gate, err := l.gate.Get()
	if err != nil {
		return err
	}
	return gate.RecordCampaignAdmission(ctx, workspaceID, campaignID, admission)
}

type lateLaunchCheck struct {
	check *deps.Handle[*wc_usecase.LaunchCheck]
}

func (l lateLaunchCheck) Run(ctx context.Context, campaignID string) (launchcheck.Report, error) {
	check, err := l.check.Get()
	if err != nil {
		return launchcheck.Report{}, err
	}
	return check.Run(ctx, campaignID)
}

func (l lateLaunchCheck) RunOn(ctx context.Context, camp *wc.Campaign) (launchcheck.Report, error) {
	check, err := l.check.Get()
	if err != nil {
		return launchcheck.Report{}, err
	}
	return check.RunOn(ctx, camp)
}
