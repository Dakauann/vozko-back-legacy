package container

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	webhook_repository "vozko/infra/repositories/webhook"
	adsuc "vozko/usecases/advertising"
	webhook_usecase "vozko/usecases/webhook"
	businessphone_usecase "vozko/usecases/whatsapp/business_phone"
	messagestatus_usecase "vozko/usecases/whatsapp/messagestatus"

	agent_domain "vozko/domain/agent"
	"vozko/domain/shared"
	businessphone "vozko/domain/whatsapp/business_phone"
	cronPackage "vozko/infra/cron"
	ia_repo "vozko/infra/repositories/inbox_assignment"
	workspace_config_repository "vozko/infra/repositories/workspace_config"
	conversation_usecase "vozko/usecases/conversation"
	ia_usecase "vozko/usecases/inbox_assignment"
	invoice_usecase "vozko/usecases/invoice"
	uwcuc "vozko/usecases/unofficial_whatsapp_campaign"
	whatsapp_campaign_usecase "vozko/usecases/whatsapp_campaign"
)

const reportRetentionSweepLimit = 500

func (c *Container) initJobRunner() {
	startScheduledWhatsappCampaignsJob := whatsapp_campaign_usecase.NewScheduleStartJob(c.useCases.dispatchWCCampaign, c.repositories.wcCampaign, c.repositories.wcSchedules, c.officialScheduleReporter())
	analysisDebounceJob := conversation_usecase.NewAnalysisDebounceJob(
		c.redisProvider.SharedState(),
		c.repositories.conversation,
		c.repositories.wcEntry,
		c.repositories.wcCampaign,
		c.repositories.lead,
		c.services.ai,
		c.services.toolRegistry,
		c.repositories.stage,
		c.useCases.listLeadMemories,
		c.services.conversationHub,
		c.services.cachedBalanceChecker,
		conversation_usecase.AnalysisJobDeps{
			Queue:         c.audience.ConversationAdapter,
			Policy:        workspace_config_repository.NewAudienceSettingsStore(c.db),
			DealSettings:  c.useCases.dealAutomation,
			DealDesk:      c.useCases.opportunity,
			Cascade:       c.services.liveGate,
			ProfileAgents: c.publishedAgents(),
		},
	)
	channels := []analysisChannel{
		{shared.EntryTypeWhatsApp, conversation_usecase.NewWhatsAppAnalysisResolver(
			c.repositories.wcEntry, c.repositories.wcCampaign, c.repositories.lead)},
	}
	if c.instagram != nil {
		channels = append(channels, analysisChannel{shared.EntryTypeInstagram, instagramAnalysisResolver(c.instagram)})
	}
	if c.telegram != nil {
		channels = append(channels, analysisChannel{shared.EntryTypeTelegram, telegramAnalysisResolver(c.telegram)})
	}
	if c.facebook != nil {
		channels = append(channels, analysisChannel{shared.EntryTypeFacebook, facebookAnalysisResolver(c.facebook)})
	}
	if c.webchat != nil {
		channels = append(channels, analysisChannel{shared.EntryTypeWebchat, webchatAnalysisResolver(c.webchat)})
	}
	channels = append(channels, c.tiktokAnalysisChannel())
	channels = append(channels, emailInboxAnalysisChannel(c.emailInbox))
	channels = append(channels, c.carrierAnalysisChannels()...)
	if c.unofficialWhatsApp != nil {
		resolver := campaignAwareResolver(
			unofficialWhatsAppAnalysisResolver(c.unofficialWhatsApp),
			c.unofficialWhatsApp.Conversations,
			c.unofficialWhatsAppCampaigns,
		)
		channels = append(channels, analysisChannel{shared.EntryTypeUnofficialWhatsApp, resolver})
	}

	debounceSink, ok := analysisDebounceJob.(analysisSubjectSink)
	if !ok {
		log.Fatalf("[analysis] the debounce job cannot take per-channel subject resolvers")
	}
	sinks := []analysisSubjectSink{c.services.liveSubjects, debounceSink}

	c.audience.ConversationAdapter.SubjectContext = c.analysisSubjectContext
	sinks = append(sinks, adapterSink{c.audience.ConversationAdapter})

	registerAnalysisChannels(channels, sinks...)

	autoCloseJob := conversation_usecase.NewAutoCloseJob(
		c.repositories.wcEntry,
		c.services.conversationStatusUpdater,
	)
	rescue, err := ia_usecase.NewRescueJob(ia_usecase.RescueDeps{
		Config:      c.repositories.workspaceConfig,
		History:     c.repositories.assignmentHistory,
		Attention:   ia_repo.NewAttentionRepository(c.db),
		Status:      c.services.conversationStatusUpdater,
		Assign:      c.services.assignmentService,
		Departments: c.repositories.workspaceDepartment,
	})
	if err != nil {
		log.Fatalf("[assignment-rescue] %v", err)
	}

	messageStatuses := messagestatus_usecase.NewPipeline(c.repositories.messageStatusLedger)
	reportService := c.services.reportService
	reconcileTemplateSends := c.useCases.reconcileTemplateSends
	sync := c.ads.Sync
	forms := c.whatsappForms()
	audienceJobs := c.audience
	ml := c.mercadoLivre
	uwBundle := c.unofficialWhatsApp
	uwCampaigns := c.unofficialWhatsAppCampaigns
	leadImports := c.leadImports()
	geocoding := c.geocodingSweeper()
	refine := c.geocodingDistrictRefine()
	workflowManager := c.useCases.workflowManager
	expire := c.useCases.expireSubscriptions
	remind := c.useCases.remindExpiringSubscriptions
	lowBalance := c.useCases.monitorLowBalance
	calendarRenewals := c.useCases.renewCalendarChannels
	templates := c.useCases.reconcileWhatsAppTemplates
	entitlements := c.useCases.reconcileWhatsAppEntitlements
	invoices := c.useCases.emitMonthlyInvoices
	cancelSweep := c.useCases.cancelBillingSweep
	vendor := c.useCases.vendorChannelReconciler
	channelStatus := c.useCases.channelStatusReconciler

	bindings := mergeJobs(cronJobs{
		"reconcile_whatsapp_templates":    cronPackage.Method(templates, func(context.Context) error { return templates.Execute() }),
		"reconcile_channel_status":        cronPackage.Method(channelStatus, func(context.Context) error { return reconcileChannelStatus(channelStatus) }),
		"reconcile_whatsapp_entitlements": cronPackage.Method(entitlements, cronPackage.Counted("reconcile_whatsapp_entitlements: reconciled %d workspace(s)", func() (int, error) { return entitlements.Execute() })),
		"emit_monthly_invoices":           cronPackage.Method(invoices, cronPackage.Counted("emit_monthly_invoices: emitted %d invoice(s)", func() (int, error) { return invoices.Execute() })),
		"cancel_billing_sweep":            cronPackage.Method(cancelSweep, cronPackage.Counted("cancel_billing_sweep: swept %d workspace(s)", func() (int, error) { return cancelSweep.Execute() })),
		"vendor_channel_reconcile":        cronPackage.Method(vendor, func(context.Context) error { return reconcileVendorChannels(vendor) }),
		"start_scheduled_wc_campaigns": cronPackage.Method(startScheduledWhatsappCampaignsJob, func(context.Context) error {
			return startScheduledWhatsappCampaignsJob.StartScheduledWhatsappCampaigns()
		}),
		"analysis_debounce":       cronPackage.Method(analysisDebounceJob, func(context.Context) error { return analysisDebounceJob.ProcessPendingAnalyses() }),
		"conversation_auto_close": cronPackage.Method(autoCloseJob, func(context.Context) error { return autoCloseJob.ProcessIdleCloses() }),
		"workflow_manager":        cronPackage.Method(workflowManager, func(context.Context) error { workflowManager.Tick(); return nil }),
		"expire_subscriptions":    cronPackage.Method(expire, cronPackage.Counted("expire_subscriptions: expired %d subscription(s)", func() (int64, error) { return expire.Execute() })),
		"plan_expiry_reminders":   cronPackage.Method(remind, cronPackage.Counted("plan_expiry_reminders: sent %d reminder(s)", func() (int, error) { return remind.RemindExpiring() })),
		"monitor_low_balance":     cronPackage.Method(lowBalance, cronPackage.Counted("monitor_low_balance: %d low wallet(s) found", func() (int, error) { return lowBalance.Run() })),
		"renew_calendar_channels": cronPackage.Method(calendarRenewals, cronPackage.Counted("renew_calendar_channels: renewed %d channel(s)", func() (int, error) { return calendarRenewals.Execute() })),
		"shortlink_retention":     cronPackage.Executes(c.useCases.purgeShortLinkClicks),

		"scheduled_message_sweep":   cronPackage.Executes(c.useCases.sweepScheduledMessages),
		"scheduled_message_purge":   cronPackage.Executes(c.useCases.purgeScheduledMessages),
		"team_chat_scheduled_sweep": c.teamChatSweep(),
		"assignment_rescue":         cronPackage.Executes(rescue),
		"whatsapp_template_send_reconcile": cronPackage.Method(reconcileTemplateSends, func(ctx context.Context) error {
			_, err := reconcileTemplateSends.Execute(ctx)
			return err
		}),
		"smart_campaign_waves": cronPackage.Method(c.smartCampaigns.engine, c.smartCampaignTick),
		"whatsapp_phone_quality_poll": cronPackage.Executes(businessphone_usecase.NewPollPhoneQualityUseCase(
			c.repositories.businessPhone, c.services.businessPhoneMetaAPI, c.repositories.messageStatusSending,
			c.repositories.whatsappQualityHistory, c.repositories.whatsappQuality,
		)),
		"whatsapp_message_status_sweep": cronPackage.Method(messageStatuses, messageStatuses.Sweep),
		"whatsapp_message_status_purge": cronPackage.Method(messageStatuses, messageStatuses.Purge),
		"report_retention": cronPackage.Method(reportService, cronPackage.Counted("report_retention: expired %d report(s)", func() (int64, error) {
			return reportService.ExpireOldFiles(reportRetentionSweepLimit)
		})),
		"lead_import_sweep":            cronPackage.Method(leadImports, leadImports.Sweep),
		"campaign_audience_list_sweep": cronPackage.Run(c.campaignAudience().SweepExpired),
		"geocoding_sweep":              cronPackage.Method(geocoding, geocoding.Sweep),
		"geocoding_district_refine":    cronPackage.Method(refine, refine.Run),
		"lead_action_sweep":            cronPackage.Run(c.leadActions().Sweep),
		"call_list_build_sweep":        cronPackage.Run(c.callLists().Sweep),
		"invoice_reservation_sweep":    cronPackage.Executes(invoice_usecase.NewCancelStaleReservations(c.repositories.invoice)),
		"webhook_event_purge": cronPackage.Executes(webhook_usecase.NewPurgeProcessedEventsUseCase(
			webhook_repository.NewProcessedEventRepository(c.db), 30*24*time.Hour)),
		"comment_rule_execution_purge":      cronPackage.Executes(c.commentAutomation().Purge),
		"workflow_trace_retention":          cronPackage.Executes(c.workflowTraceRetention()),
		"workflow_runtime_alerts":           cronPackage.Executes(c.useCases.workflowRuntimeAlerts),
		"workflow_agent_continuation_sweep": cronPackage.Executes(c.useCases.agentContinuationSweep),
		"instagram_token_refresh":           cronPackage.Executes(c.instagram.RefreshTokens),
		"facebook_health_check":             cronPackage.Executes(c.facebook.Health),
		"facebook_publish_reaper":           cronPackage.Method(c.facebook.Publisher, c.facebook.Publisher.Reap),
		"ads_recent_sync":                   cronPackage.Method(sync, func(ctx context.Context) error { return sync.SyncAll(ctx, adsuc.RecentInsightDays) }),
		"ads_settled_sync":                  cronPackage.Method(sync, func(ctx context.Context) error { return sync.SyncAll(ctx, adsuc.SettledInsightDays) }),
		"ads_review_refresh":                cronPackage.Method(sync, sync.RefreshReviews),
		"ads_funds_watch":                   cronPackage.Method(sync, sync.WatchFunds),
		"ads_publish_reaper":                cronPackage.Method(c.ads.Publish, c.ads.Publish.Resume),
		"ads_form_lead_poll":                cronPackage.Method(c.ads.Forms, c.ads.Forms.PollAll),
		"ads_conversion_dispatch":           cronPackage.Method(c.ads.Conversions, c.ads.Conversions.DispatchAll),
		"ads_grant_health":                  cronPackage.Executes(c.ads.GrantHealth),
		"whatsapp_form_recover":             cronPackage.Method(forms, forms.RecoverStuck),
		"media_generation_reaper":           cronPackage.Method(c.mediaGeneration().Service, c.mediaGeneration().Service.Reap),
		"audience_flush":                    cronPackage.Executes(audienceJobs.Flush),
		"audience_backstop":                 cronPackage.Executes(audienceJobs.Backstop),
		"audience_rollup":                   cronPackage.Executes(audienceJobs.Rollup),
		"audience_purge":                    cronPackage.Executes(audienceJobs.Purge),
		"audience_backfill":                 cronPackage.Executes(audienceJobs.Backfill),
		"telegram_webhook_health":           cronPackage.Executes(c.telegram.CheckHealth),
		"mercadolivre_token_refresh":        cronPackage.Executes(ml.RefreshTokens),
		"mercadolivre_health_check":         cronPackage.Executes(ml.Health),
		"mercadolivre_reconcile":            cronPackage.Executes(ml.Reconcile),
		"mercadolivre_missed_feeds":         cronPackage.Executes(ml.MissedFeeds),
		"mercadolivre_purge":                cronPackage.Executes(ml.Purge),
		"workflow_birthday_sweep":           cronPackage.Executes(c.workflowLeadTriggers.Birthday),
		"unofficial_whatsapp_campaign_scheduled_start": cronPackage.Method(uwCampaigns.ScheduleJob, func(context.Context) error {
			return uwCampaigns.ScheduleJob.StartScheduledCampaigns()
		}),
		"unofficial_whatsapp_session_health":     cronPackage.Executes(uwBundle.CheckHealth),
		"unofficial_whatsapp_integrity":          cronPackage.Method(uwBundle.CheckHealth, uwBundle.CheckHealth.VerifyIntegrity),
		"unofficial_whatsapp_capacity_reconcile": cronPackage.Executes(uwBundle.ReconcileCapacity),
		"unofficial_whatsapp_history_sweep":      cronPackage.Executes(uwBundle.HistorySync),
		"unofficial_whatsapp_groups_sweep":       cronPackage.Executes(uwBundle.WorkspaceGroups.Sweep()),
	}, c.notificationJobs(), c.tiktokJobs(), c.googleJobs(), c.googleSheetsJobs(), c.emailJobs(), c.carrierCronJobs())

	registry, err := cronPackage.NewRegistry(c.redisProvider.SharedState(), cronPackage.Catalog, bindings)
	if err != nil {
		log.Fatalf("[cron] %v", err)
	}
	c.jobRunner = registry
}

type cronJobs map[string]cronPackage.Run

func mergeJobs(groups ...cronJobs) cronJobs {
	merged := cronJobs{}
	for _, group := range groups {
		for name, run := range group {
			if _, taken := merged[name]; taken {
				log.Fatalf("[cron] %s is bound twice", name)
			}
			merged[name] = run
		}
	}
	return merged
}

func reconcileChannelStatus(reconciler businessphone.ChannelStatusReconciler) error {
	rep, err := reconciler.Execute()
	if err != nil {
		return err
	}
	if rep.Suspended > 0 || rep.Updated > 0 || rep.WABAsNamed > 0 {
		log.Printf("[cron] reconcile_channel_status: suspended=%d updated=%d wabas_named=%d", rep.Suspended, rep.Updated, rep.WABAsNamed)
	}
	return nil
}

func reconcileVendorChannels(reconciler businessphone.VendorChannelReconciler) error {
	report, err := reconciler.Execute()
	if err != nil {
		return err
	}
	if report.Orphans > 0 || report.Leaks > 0 {
		log.Printf("[cron] vendor_channel_reconcile: billing=%d orphans=%d leaks=%d recancelled=%d",
			report.VendorBilling, report.Orphans, report.Leaks, report.Recancelled)
	}
	return nil
}

func derefID(id *string) string {
	if id == nil {
		return ""
	}
	return *id
}

const agentPromptContextRunes = 6000

func (c *Container) analysisSubjectContext(_ context.Context, subject *conversation_usecase.AnalysisSubject) (string, error) {
	text := fmt.Sprintf(
		"Campaign/account: %s. A name alone does not establish a conversion objective.",
		subject.ContainerName,
	)
	if subject.AgentID == "" || c.repositories.agent == nil {
		return text, nil
	}

	agent, err := c.publishedAgents().FindByID(subject.AgentID)
	if errors.Is(err, agent_domain.ErrAgentNotPublished) {
		return text, nil
	}
	if err != nil {
		return "", err
	}
	if agent == nil {
		return text, nil
	}
	prompt := []rune(strings.TrimSpace(agent.MessagingPrompt))
	if len(prompt) > agentPromptContextRunes {
		prompt = prompt[:agentPromptContextRunes]
	}
	if len(prompt) == 0 {
		return text, nil
	}
	return text + "\nConfigured agent purpose and guidance:\n" + string(prompt), nil
}

type analysisChannel struct {
	entry    shared.EntryType
	resolver conversation_usecase.AnalysisSubjectResolver
}

type analysisSubjectSink interface {
	SetAnalysisSubjectResolver(shared.EntryType, conversation_usecase.AnalysisSubjectResolver)
}

type adapterSink struct {
	adapter *conversation_usecase.AnalysisAdapter
}

func (a adapterSink) SetAnalysisSubjectResolver(entry shared.EntryType, resolver conversation_usecase.AnalysisSubjectResolver) {
	a.adapter.RegisterResolver(entry, resolver)
}

func registerAnalysisChannels(channels []analysisChannel, sinks ...analysisSubjectSink) {
	for _, sink := range sinks {
		for _, ch := range channels {
			sink.SetAnalysisSubjectResolver(ch.entry, ch.resolver)
		}
	}
}

func campaignAwareResolver(
	base conversation_usecase.AnalysisSubjectResolver,
	conversations uwcuc.ConversationCampaigns,
	campaigns *unofficialWhatsAppCampaignBundle,
) conversation_usecase.AnalysisSubjectResolver {
	if conversations == nil || campaigns == nil || campaigns.Campaigns == nil {
		return base
	}
	return uwcuc.NewAutomationSource(conversations, campaigns.Campaigns).AnalysisResolver(base)
}
