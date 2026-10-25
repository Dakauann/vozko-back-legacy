package cron

import (
	"testing"
	"time"

	workflow_domain "vozko/domain/workflow"
)

func catalogPeriod(t *testing.T, name string) time.Duration {
	t.Helper()
	for _, job := range Catalog {
		if job.Name == name {
			return job.Period
		}
	}
	t.Fatalf("%s is not in the catalog", name)
	return 0
}

func TestTheCatalogKeepsEveryJobOnItsSchedule(t *testing.T) {
	const day = 24 * time.Hour
	want := map[string]time.Duration{
		"reconcile_whatsapp_templates":                 15 * time.Minute,
		"reconcile_channel_status":                     10 * time.Minute,
		"reconcile_whatsapp_entitlements":              15 * time.Minute,
		"emit_monthly_invoices":                        time.Hour,
		"cancel_billing_sweep":                         time.Hour,
		"vendor_channel_reconcile":                     day,
		"start_scheduled_wc_campaigns":                 time.Minute,
		"analysis_debounce":                            time.Minute,
		"conversation_auto_close":                      5 * time.Minute,
		"workflow_manager":                             time.Minute,
		"expire_subscriptions":                         time.Hour,
		"plan_expiry_reminders":                        day,
		"monitor_low_balance":                          day,
		"renew_calendar_channels":                      30 * time.Minute,
		"shortlink_retention":                          day,
		"instagram_token_refresh":                      time.Hour,
		"facebook_health_check":                        day,
		"facebook_publish_reaper":                      5 * time.Minute,
		"ads_recent_sync":                              30 * time.Minute,
		"ads_review_refresh":                           2 * time.Minute,
		"ads_funds_watch":                              10 * time.Minute,
		"ads_settled_sync":                             day,
		"ads_publish_reaper":                           time.Minute,
		"ads_form_lead_poll":                           15 * time.Minute,
		"ads_conversion_dispatch":                      5 * time.Minute,
		"ads_grant_health":                             6 * time.Hour,
		"whatsapp_form_recover":                        15 * time.Minute,
		"media_generation_reaper":                      time.Minute,
		"webhook_event_purge":                          day,
		"workflow_trace_retention":                     6 * time.Hour,
		"workflow_runtime_alerts":                      workflow_domain.RuntimeAlertEvaluationPeriod,
		"workflow_agent_continuation_sweep":            workflow_domain.ContinuationSweepPeriod,
		"comment_rule_execution_purge":                 day,
		"audience_flush":                               30 * time.Second,
		"audience_backstop":                            5 * time.Minute,
		"audience_rollup":                              time.Hour,
		"audience_purge":                               day,
		"audience_backfill":                            time.Minute,
		"mercadolivre_token_refresh":                   30 * time.Minute,
		"mercadolivre_health_check":                    day,
		"mercadolivre_reconcile":                       10 * time.Minute,
		"mercadolivre_missed_feeds":                    time.Hour,
		"mercadolivre_purge":                           day,
		"tiktok_token_refresh":                         time.Hour,
		"tiktok_health_check":                          day,
		"tiktok_purge":                                 day,
		"tiktok_comment_work":                          10 * time.Second,
		"telegram_webhook_health":                      time.Hour,
		"unofficial_whatsapp_session_health":           15 * time.Minute,
		"unofficial_whatsapp_integrity":                time.Hour,
		"unofficial_whatsapp_capacity_reconcile":       day,
		"unofficial_whatsapp_history_sweep":            time.Minute,
		"unofficial_whatsapp_groups_sweep":             time.Minute,
		"assignment_rescue":                            time.Minute,
		"unofficial_whatsapp_campaign_scheduled_start": time.Minute,
		"whatsapp_template_send_reconcile":             time.Hour,
		"whatsapp_phone_quality_poll":                  30 * time.Minute,
		"smart_campaign_waves":                         time.Minute,
		"email_domain_verification":                    10 * time.Minute,
		"email_promotion_sweep":                        day,
		"email_deliverability_sweep":                   day,
		"email_tenant_retirement_sweep":                day,
		"email_unused_account_release":                 day,
		"email_reply_sweep":                            time.Minute,
		"email_webhook_sweep":                          15 * time.Second,
		"email_webhook_purge":                          day,
		"email_ses_send_quota_refresh":                 time.Minute,
		"email_ses_capacity_check":                     day,
		"email_tenant_health_sweep":                    day,
		"email_domain_checks":                          time.Minute,
		"email_disposable_refresh":                     day,
		"email_hygiene_prune":                          day,
		"email_campaign_scheduled_start":               time.Minute,
		"email_campaign_tick":                          5 * time.Minute,
		"workflow_birthday_sweep":                      workflow_domain.BirthdaySweepPeriod,
		"carrier_message_expiry":                       time.Minute,
		"carrier_sender_status_sync":                   15 * time.Minute,
		"carrier_campaign_schedules":                   time.Minute,
		"whatsapp_message_status_sweep":                10 * time.Minute,
		"whatsapp_message_status_purge":                day,
		"lead_import_sweep":                            time.Minute,
		"campaign_audience_list_sweep":                 15 * time.Minute,
		"geocoding_sweep":                              time.Minute,
		"geocoding_district_refine":                    time.Hour,
		"lead_action_sweep":                            time.Minute,
		"call_list_build_sweep":                        time.Minute,
		"report_retention":                             time.Hour,
		"notification_sweep":                           time.Minute,
		"notification_purge":                           day,
		"notification_digest":                          time.Hour,
		"scheduled_message_sweep":                      time.Minute,
		"scheduled_message_purge":                      day,
		"invoice_reservation_sweep":                    5 * time.Minute,
		"team_chat_scheduled_sweep":                    time.Minute,
		"google_token_refresh":                         30 * time.Minute,
		"google_sheets_dispatch":                       time.Minute,
		"google_sheets_retention":                      day,
	}
	got := map[string]time.Duration{}
	for _, job := range Catalog {
		got[job.Name] = job.Period
	}
	for name, period := range want {
		if got[name] != period {
			t.Errorf("%s every %v, want %v", name, got[name], period)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s is in the catalog but not pinned here", name)
		}
	}
}

func TestOnlyTheAnalysisDebounceRunsOnEveryReplica(t *testing.T) {
	for _, job := range Catalog {
		if job.EveryReplica != (job.Name == "analysis_debounce") {
			t.Errorf("%s EveryReplica = %v", job.Name, job.EveryReplica)
		}
	}
}
