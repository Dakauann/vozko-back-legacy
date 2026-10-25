package container

import (
	"context"
	"log"
	"time"

	smartcampaignhttp "vozko/delivery/http/smartcampaignhttp"
	balance_domain "vozko/domain/balance"
	"vozko/domain/shared/deps"
	wc_domain "vozko/domain/whatsapp_campaign"
	wcw_repository "vozko/infra/repositories/whatsapp_campaign_wave"
	"vozko/usecases/campaignqueue"
	smartcampaign_usecase "vozko/usecases/smartcampaign"
	businessphone_usecase "vozko/usecases/whatsapp/business_phone"
	wc_usecase "vozko/usecases/whatsapp_campaign"
)

type smartCampaignWiring struct {
	entries      *smartcampaign_usecase.ObservedEntries
	engine       *smartcampaign_usecase.Engine
	configurator *smartcampaign_usecase.Configurator
	signals      *smartcampaign_usecase.Signals
	handle       *deps.Handle[wc_usecase.SmartCampaigns]
}

func (c *Container) observedCampaignEntries() *smartcampaign_usecase.ObservedEntries {
	if c.smartCampaigns.entries == nil {
		c.smartCampaigns.entries = smartcampaign_usecase.ObserveFailures(c.repositories.wcEntry)
	}
	return c.smartCampaigns.entries
}

func (c *Container) wireSmartCampaigns(consumer wc_domain.MessageConsumerUseCase, dashboardURL string) {
	caps, ok := c.repositories.monthlySendCaps.(balance_domain.MonthlySendCapUsageReader)
	if !ok {
		log.Fatalf("[smart-campaign] the monthly send cap repository cannot read what is left of the cap")
	}

	waves := wcw_repository.NewRepository(c.db)
	probe := businessphone_usecase.NewPollPhoneQualityUseCase(c.repositories.businessPhone, c.services.businessPhoneMetaAPI,
		c.repositories.messageStatusSending, c.repositories.whatsappQualityHistory, c.repositories.whatsappQuality)
	engine := smartcampaign_usecase.NewEngine(smartcampaign_usecase.Deps{
		Campaigns: c.repositories.wcCampaign,
		Entries:   c.repositories.wcEntry,
		Policies:  waves,
		States:    waves,
		Waves:     waves,
		Health:    smartcampaign_usecase.NewHealthReader(c.repositories.whatsappTemplate, c.repositories.businessPhone, c.repositories.whatsappQualityTimeline, probe),
		Consumer:  consumer,
		Queue:     campaignqueue.NewDispatcher(c.services.wcQueuePub, c.redisProvider.SharedState(), wc_domain.QueueNamespace),
		Caps:      caps,
		Notifier:  smartcampaign_usecase.NewHoldNotice(c.notifications().Publisher, dashboardURL),
		Locks:     c.redisProvider.SharedState(),
		Now:       time.Now,
	})

	c.observedCampaignEntries().Attach(engine)
	if err := c.smartCampaignsHandle().Bind(engine); err != nil {
		log.Fatalf("[smart-campaign] %v", err)
	}
	c.smartCampaigns.engine = engine
	c.smartCampaigns.configurator = smartcampaign_usecase.NewConfigurator(waves, c.repositories.workspaceConfig, c.repositories.workspaceDepartment)
	c.smartCampaigns.signals = smartcampaign_usecase.NewSignals(c.repositories.whatsappTemplate, engine)
}

func (c *Container) smartCampaignTick(ctx context.Context) error {
	return c.smartCampaigns.engine.Tick(ctx)
}

func (c *Container) signallingPhones() *smartcampaign_usecase.SignallingPhones {
	return smartcampaign_usecase.SignalPhoneChanges(c.repositories.businessPhone, c.smartCampaigns.signals)
}

func (c *Container) smartCampaignHandler() *smartcampaignhttp.Handler {
	return smartcampaignhttp.NewHandler(c.useCases.wcCampaignAccess, c.smartCampaigns.engine, c.smartCampaigns.configurator)
}

func (c *Container) createWithWavePolicy(create wc_domain.CreateCampaignUseCase, remove wc_domain.DeleteCampaignUseCase) wc_domain.CreateCampaignUseCase {
	return smartcampaign_usecase.CreateWithPolicy(create, c.smartCampaigns.configurator, remove)
}

func (c *Container) smartCampaignsHandle() *deps.Handle[wc_usecase.SmartCampaigns] {
	if c.smartCampaigns.handle == nil {
		c.smartCampaigns.handle = deps.Tracked[wc_usecase.SmartCampaigns](&c.handles, "smart campaign engine")
	}
	return c.smartCampaigns.handle
}

type lateSmartCampaigns struct {
	engine *deps.Handle[wc_usecase.SmartCampaigns]
}

func (l lateSmartCampaigns) Manages(campaignID string) (bool, error) {
	engine, err := l.engine.Get()
	if err != nil {
		return false, err
	}
	return engine.Manages(campaignID)
}
