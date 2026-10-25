package container

import (
	"log"

	deliveryHttp "vozko/delivery/http"
	"vozko/delivery/http/emailapihttp"
	emailhygienehttp "vozko/delivery/http/emailhygiene"
	emailoutreachhttp "vozko/delivery/http/emailoutreach"
	"vozko/delivery/http/emailpricing"
	cronPackage "vozko/infra/cron"
	balance_usecase "vozko/usecases/balance"
	workspace_pricing_usecase "vozko/usecases/workspace_pricing"
)

type emailBundle struct {
	Sending   *emailSendingBundle
	Hygiene   *emailHygieneBundle
	Campaigns *emailCampaignBundle
	Billing   *balance_usecase.ConsumeEmailUseCase
	Prices    *balance_usecase.EmailPriceList
	Pricing   *emailpricing.Handler
	Outreach  *emailoutreachhttp.Handler
}

func (c *Container) initEmail() {
	if c.repositories == nil || c.repositories.balance == nil || c.repositories.workspacePricing == nil {
		log.Fatalf("[email] the balance and pricing repositories must exist first")
	}
	if c.services == nil || c.services.workspacePricer == nil || c.useCases == nil || c.useCases.ensureActiveWorkspaceSubscription == nil {
		log.Fatalf("[email] the pricer and the subscription checker must exist first")
	}
	billing := balance_usecase.NewConsumeEmailUseCase(c.repositories.balance, c.services.workspacePricer, c.useCases.ensureActiveWorkspaceSubscription)
	sending := c.buildEmailSending()
	hygiene := c.emailHygiene()
	campaigns := c.buildEmailCampaigns(sending, hygiene, billing)
	rates := workspace_pricing_usecase.NewGetExchangeRateUseCase(c.repositories.workspacePricing)
	prices := balance_usecase.NewEmailPriceList(c.services.workspacePricer, rates)
	c.email = &emailBundle{
		Sending:   sending,
		Hygiene:   hygiene,
		Campaigns: campaigns,
		Billing:   billing,
		Prices:    prices,
		Pricing:   emailpricing.NewHandler(prices),
	}
	c.startEmailHygiene(hygiene)
	c.startEmailCampaignConsumer(campaigns)
}

func (c *Container) emailJobs() cronJobs {
	if c.email == nil || c.email.Sending == nil || c.email.Campaigns == nil || c.email.Hygiene == nil {
		log.Fatalf("[email] jobs requested before email was built")
	}
	m := c.email.Sending.Module
	campaigns := c.email.Campaigns.Campaigns
	hygiene := c.email.Hygiene
	return mergeJobs(cronJobs{
		"email_domain_verification":      cronPackage.Executes(m.Verification),
		"email_promotion_sweep":          cronPackage.Executes(m.Promotion),
		"email_deliverability_sweep":     cronPackage.Executes(m.Deliverability),
		"email_tenant_retirement_sweep":  cronPackage.Executes(m.Dormancy),
		"email_unused_account_release":   cronPackage.Executes(m.UnusedRelease),
		"email_reply_sweep":              cronPackage.Executes(m.ReplySweep),
		"email_ses_send_quota_refresh":   cronPackage.Executes(m.SendQuota),
		"email_ses_capacity_check":       cronPackage.Executes(m.Capacity),
		"email_tenant_health_sweep":      cronPackage.Executes(m.HealthSweep),
		"email_campaign_scheduled_start": cronPackage.Method(campaigns, campaigns.StartScheduled),
		"email_campaign_tick":            cronPackage.Method(campaigns, campaigns.Tick),
		"email_domain_checks":            cronPackage.Executes(hygiene.Domains),
		"email_disposable_refresh":       cronPackage.Executes(hygiene.Disposable),
		"email_hygiene_prune":            cronPackage.Executes(hygiene.Pruner),
	}, c.emailAPIJobs())
}

func emailRoutes(c *Container) deliveryHttp.EmailRoutes {
	if c.email == nil || c.email.Sending == nil || c.email.Campaigns == nil || c.email.Pricing == nil || c.email.Outreach == nil || c.email.Hygiene == nil {
		log.Fatalf("[email] routes requested before email was built")
	}
	return deliveryHttp.EmailRoutes{
		Sending:      c.email.Sending.Handler,
		SendingAdmin: c.email.Sending.Admin,
		Webhooks:     c.email.Sending.Webhooks,
		Campaigns:    c.email.Campaigns.Handler,
		Consent:      c.email.Campaigns.ConsentHandler,
		Public:       c.email.Campaigns.Public,
		Pricing:      c.email.Pricing,
		Inboxes:      emailInboxHandler(c.emailInbox),
		Outreach:     c.email.Outreach,
		API:          emailAPIHandler(c),
		Hygiene:      emailhygienehttp.NewHandler(c.email.Hygiene.Service),
	}
}

func emailAPIHandler(c *Container) *emailapihttp.Handler {
	if c.emailAPI == nil {
		log.Fatalf("[email-api] routes requested before the email API was built")
	}
	return c.emailAPI.Handler
}
