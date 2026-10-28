package container

import (
	"context"
	"log"
	"time"

	deliveryHttp "vozko/delivery/http"
	"vozko/delivery/http/siteformhttp"
	"vozko/domain/cache"
	"vozko/domain/sitesubmission"
	cronPackage "vozko/infra/cron"
	conversation_repository "vozko/infra/repositories/conversation"
	opportunity_repository "vozko/infra/repositories/opportunity"
	siteform_repository "vozko/infra/repositories/siteform"
	"vozko/usecases/conversationad"
	lead_usecase "vozko/usecases/lead"
	"vozko/usecases/siteforms"
	sitepublish_usecase "vozko/usecases/sitepublish"
	wc_usecase "vozko/usecases/whatsapp_campaign"
	whatsapp_outreach_usecase "vozko/usecases/whatsapp_outreach"
)

const (
	siteFormsChallengesPerIP = 60
	siteFormsSubmitsPerIP    = 20
	siteFormsPerFormPerHour  = 1200
	siteFormsPreviewPerIP    = 300
	siteFormsIPWindow        = 10 * time.Minute
)

func (c *Container) siteEmbedOrigins() siteforms.EmbedOrigins {
	return siteforms.EmbedOrigins{Forms: siteform_repository.NewFormStore(c.db), Hostnames: c.siteHostnameReader()}
}

func (c *Container) sitePageOriginRecorder() *conversationad.PageRecorder {
	return conversationad.NewPageRecorder(
		conversation_repository.NewPageOriginRepository(c.db),
		siteform_repository.NewReferenceStore(c.db),
		c.services.campaignWorkspaceResolver,
		c.repositories.conversation,
		time.Now,
	)
}

func (c *Container) siteFormLimiter(factory cache.RateLimiterFactory, prefix string, max int, window time.Duration) cache.RateLimiter {
	if factory == nil {
		log.Fatalf("[site-forms] the rate limiter factory must exist before the page forms")
	}
	return factory(prefix, max, window)
}

func (c *Container) siteForms() *siteforms.Service {
	if c.siteFormsWiring != nil {
		return c.siteFormsWiring
	}
	if c.email == nil || c.email.Campaigns == nil || c.email.Campaigns.Consent == nil || c.emailInbox == nil || c.emailInbox.Transcript == nil ||
		c.unofficialWhatsApp == nil || c.unofficialWhatsApp.StartConv == nil || c.useCases.opportunity == nil || c.services.assignmentService == nil {
		log.Fatalf("[site-forms] email consent, the email inbox, unofficial WhatsApp, deals and the roulette must be wired before the page forms")
	}
	entries, err := whatsapp_outreach_usecase.NewEntryOpener(whatsapp_outreach_usecase.Deps{
		Phones: c.repositories.businessPhone, PhoneGrants: c.repositories.workspacePhoneAccess, Leads: c.repositories.lead,
		Entries: c.repositories.wcEntry, EnsureReceptive: wc_usecase.NewEnsureReceptiveContainerUseCase(c.repositories.wcCampaign),
	})
	if err != nil {
		log.Fatalf("[site-forms] %v", err)
	}
	owners, err := lead_usecase.NewCommands(c.leadCommandDeps())
	if err != nil {
		log.Fatalf("[site-forms] %v", err)
	}
	factory, sites := c.services.rateLimiterFactory, c.cfg.Sites
	service, err := siteforms.New(siteforms.Deps{
		Forms:       siteform_repository.NewFormStore(c.db),
		Submissions: siteform_repository.NewSubmissionStore(c.db),
		Effects:     siteform_repository.NewEffectStore(c.db),
		Hostnames:   c.siteHostnameReader(),
		Projects:    siteform_repository.NewProjectDirectory(c.db),
		OneShot:     c.redisProvider.SharedState(),
		PerIP:       c.siteFormLimiter(factory, "site_forms_ip", siteFormsChallengesPerIP+siteFormsSubmitsPerIP, siteFormsIPWindow),
		PerForm:     c.siteFormLimiter(factory, "site_forms_form", siteFormsPerFormPerHour, time.Hour),
		PerPreview:  c.siteFormLimiter(factory, "site_forms_preview_ip", siteFormsPreviewPerIP, siteFormsIPWindow),
		Leads:       c.formLeads(),
		EmailOptIns: c.email.Campaigns.Consent,
		Roulette:    c.services.assignmentService,
		LeadOwners:  owners,
		Deals:       c.useCases.opportunity,
		LeadDeals:   opportunity_repository.NewLeadDealReader(c.db),
		Official:    entries,
		Unofficial:  c.unofficialWhatsApp.StartConv,
		Email:       c.emailInbox.Transcript,
		Notices:     c.notifications().Publisher,
		Conversions: c.adsManager().Conversions,
		Links:       c.useCases.createShortLink,
		Numbers:     siteforms.PageNumbers{Phones: c.repositories.businessPhone, Grants: c.repositories.workspacePhoneAccess, Instances: c.unofficialWhatsApp.Instances},
		References:  siteform_repository.NewReferenceStore(c.db),
		Config: siteforms.Config{
			ChallengeKey: []byte(sites.FormsChallengeKey), IPHashKey: sitesubmission.IPHashKey(sites.FormsChallengeKey),
			Addresses: siteAddresses(sites), ShortLinkBase: c.cfg.ShortLinkBaseURL,
		},
	})
	if err != nil {
		log.Fatalf("[site-forms] %v", err)
	}
	c.siteFormsWiring = service
	return service
}

type siteFormsForPublish struct {
	c *Container
}

func (p siteFormsForPublish) KeysFor(ctx context.Context, workspaceID, projectID string, formIDs []string) (map[string]string, error) {
	return p.c.siteForms().KeysFor(ctx, workspaceID, projectID, formIDs)
}

func (p siteFormsForPublish) Activate(ctx context.Context, in sitepublish_usecase.ActivateForms) error {
	return p.c.siteForms().Activate(ctx, siteforms.ActivateInput{
		WorkspaceID: in.WorkspaceID, ProjectID: in.ProjectID, DeploymentID: in.DeploymentID,
		Manifests: in.Manifests, Settings: in.Settings, Widgets: in.Widgets,
	})
}

func (p siteFormsForPublish) Deactivate(ctx context.Context, projectID string) error {
	return p.c.siteForms().Deactivate(ctx, projectID)
}

func (p siteFormsForPublish) WhatsAppLink(ctx context.Context, projectID, numberRef, text, route string) (string, error) {
	link, err := p.c.siteForms().WhatsAppLink(ctx, projectID, numberRef, text, route)
	if err != nil {
		return "", err
	}
	return link.URL, nil
}

func (p siteFormsForPublish) DirectLink(ctx context.Context, workspaceID, numberRef, text string) (string, error) {
	return p.c.siteForms().DirectLink(ctx, workspaceID, numberRef, text)
}

func (c *Container) siteFormsForPublish() siteFormsForPublish {
	return siteFormsForPublish{c: c}
}

func (c *Container) siteFormJobs() cronJobs {
	forms := c.siteForms()
	return cronJobs{"site_form_effects_sweep": cronPackage.Method(forms, forms.RunPendingEffects)}
}

func (c *Container) siteFormHandlers() (*siteformhttp.PublicHandler, *siteformhttp.Handler) {
	forms := c.siteForms()
	return siteformhttp.NewPublicHandler(forms), siteformhttp.NewHandler(forms)
}

func siteFormRoutes(c *Container) deliveryHttp.SiteFormRoutes {
	public, submissions := c.siteFormHandlers()
	return deliveryHttp.SiteFormRoutes{Public: public, Submissions: submissions}
}
