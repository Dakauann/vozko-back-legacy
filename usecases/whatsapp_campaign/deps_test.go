package whatsapp_campaign_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/campaign"
	"vozko/domain/campaign/launchcheck"
	"vozko/domain/campaign/scheduledstart"
	"vozko/domain/lead"
	"vozko/domain/shared/deps"
	businessphone "vozko/domain/whatsapp/business_phone"
	"vozko/domain/whatsapp/template"
	wc "vozko/domain/whatsapp_campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
	"vozko/domain/whatsappform"
	"vozko/domain/workspace_phone_access"
	"vozko/usecases/campaignautomation"
	whatsappform_usecase "vozko/usecases/whatsappform"
)

type unusedCampaigns struct{ wc.Repository }
type unusedEntries struct{ wce.Repository }
type unusedLeads struct{ lead.Repository }
type unusedTemplates struct{ template.Repository }
type unusedPhones struct{ businessphone.Repository }
type unusedPhoneAccess struct {
	workspace_phone_access.Repository
}
type unusedKeys struct{ IdempotentCampaigns }

type noDepartment struct{}

func (noDepartment) Resolve(context.Context, string) (string, error) { return "", nil }

type refusingRules struct{}

func (refusingRules) Rule(string, campaign.Automation) (campaignautomation.Rule, error) {
	return campaignautomation.Rule{}, campaign.ErrAutomationUnavailable
}

type silentSchedules struct{}

func (silentSchedules) RunOn(context.Context, *wc.Campaign) (launchcheck.Report, error) {
	return launchcheck.Report{}, nil
}

type quietReporter struct{}

func (quietReporter) Cleared(context.Context, string, scheduledstart.Failure) {}

type quietToggle struct{}

func (quietToggle) AIToggle(string, string, string, string, bool) {}

func createWith(d CreateCampaignDeps) wc.CreateCampaignUseCase {
	if d.Campaigns == nil {
		d.Campaigns = unusedCampaigns{}
	}
	if d.Entries == nil {
		d.Entries = unusedEntries{}
	}
	if d.Leads == nil {
		d.Leads = unusedLeads{}
	}
	if d.Templates == nil {
		d.Templates = unusedTemplates{}
	}
	if d.Phones == nil {
		d.Phones = unusedPhones{}
	}
	if d.PhoneAccess == nil {
		d.PhoneAccess = unusedPhoneAccess{}
	}
	if d.Eligibility == nil {
		d.Eligibility = &fakeScreener{}
	}
	if d.Departments == nil {
		d.Departments = noDepartment{}
	}
	if d.TemplateGrants == nil {
		d.TemplateGrants = allowGrants{}
	}
	if d.Automation == nil {
		d.Automation = automationThatAllows{}
	}
	if d.Keyed == nil {
		d.Keyed = unusedKeys{}
	}
	if d.ExitRisk == nil {
		d.ExitRisk = admittingGate{}
	}
	uc, err := NewCreateCampaignUseCase(d)
	if err != nil {
		panic(err)
	}
	return uc
}

func updateWith(d UpdateCampaignDeps) wc.UpdateCampaignUseCase {
	if d.Campaigns == nil {
		d.Campaigns = unusedCampaigns{}
	}
	if d.Entries == nil {
		d.Entries = unusedEntries{}
	}
	if d.Templates == nil {
		d.Templates = unusedTemplates{}
	}
	if d.Phones == nil {
		d.Phones = unusedPhones{}
	}
	if d.PhoneAccess == nil {
		d.PhoneAccess = unusedPhoneAccess{}
	}
	if d.Automation == nil {
		d.Automation = refusingRules{}
	}
	if d.ExitRisk == nil {
		d.ExitRisk = admittingGate{}
	}
	if d.TemplateGrants == nil {
		d.TemplateGrants = allowGrants{}
	}
	if d.ScheduleCheck == nil {
		d.ScheduleCheck = silentSchedules{}
	}
	uc, err := NewUpdateCampaignUseCase(d)
	if err != nil {
		panic(err)
	}
	return uc
}

func addEntriesWith(campaigns wc.Repository, entries wce.Repository, leads lead.Repository) wc.AddEntriesUseCase {
	uc, err := NewAddEntriesUseCase(campaigns, entries, leads, automationThatAllows{}, &fakeScreener{})
	if err != nil {
		panic(err)
	}
	return uc
}

func dispatchWith(campaigns wc.Repository, entries wce.Repository) *dispatchCampaignUseCase {
	uc, err := NewDispatchCampaignUseCase(newQSQueuePub(), campaigns, entries, newQSConsumer(), newQSShared(), passingGate(), &fakeSmartStarter{managed: map[string]bool{}})
	if err != nil {
		panic(err)
	}
	return uc.(*dispatchCampaignUseCase)
}

func entryUpdateWith(campaigns wc.Repository, entries wce.Repository, leads lead.Repository, merger LeadMerger) *updateEntryUseCase {
	uc, err := NewUpdateEntryUseCase(campaigns, entries, leads, merger, quietToggle{})
	if err != nil {
		panic(err)
	}
	return uc
}

func startWith(d StartCampaignDeps) wc.StartCampaignUseCase {
	if d.Schedules == nil {
		d.Schedules = unclaimedSchedules{}
	}
	if d.Reporter == nil {
		d.Reporter = quietReporter{}
	}
	uc, err := NewStartCampaignUseCase(d)
	if err != nil {
		panic(err)
	}
	return uc
}

func TestOfficialCampaignUseCasesRefuseMissingDependencies(t *testing.T) {
	checks := map[string]func() error{
		"create": func() error { _, err := NewCreateCampaignUseCase(CreateCampaignDeps{}); return err },
		"update": func() error { _, err := NewUpdateCampaignUseCase(UpdateCampaignDeps{}); return err },
		"add entries": func() error {
			_, err := NewAddEntriesUseCase(newMockCampaignRepo(), newMockEntryRepo(), &qsLeadRepo{}, nil, nil)
			return err
		},
		"quick send": func() error {
			_, err := NewQuickSendUseCase(newMockCampaignRepo(), newMockEntryRepo(), &qsLeadRepo{}, newQSQueuePub(), newQSConsumer(), newQSShared(), nil)
			return err
		},
		"dispatch": func() error {
			_, err := NewDispatchCampaignUseCase(newQSQueuePub(), newMockCampaignRepo(), newMockEntryRepo(), newQSConsumer(), newQSShared(), nil, nil)
			return err
		},
		"entry update": func() error {
			_, err := NewUpdateEntryUseCase(newMockCampaignRepo(), newMockEntryRepo(), &qsLeadRepo{}, &entryEditMerger{}, nil)
			return err
		},
		"start": func() error { _, err := NewStartCampaignUseCase(StartCampaignDeps{}); return err },
	}
	for name, build := range checks {
		t.Run(name, func(t *testing.T) {
			if err := build(); !errors.Is(err, deps.ErrMissing) {
				t.Fatalf("err = %v, want a missing dependency refused", err)
			}
		})
	}
}

func (unusedTemplates) FindByID(string) (*template.Template, error) {
	return nil, errors.New("no template in this test")
}

type unclaimedSchedules struct{ wc.ScheduleClaimer }

func (unclaimedSchedules) ClaimScheduledStart(string, time.Time) (bool, error) { return false, nil }

type unsendableForms struct{}

func (unsendableForms) PrepareTemplateSend(context.Context, whatsappform_usecase.TemplateSend) (whatsappform.Send, error) {
	return whatsappform.Send{}, template.ErrFormNotSendable
}

func (unsendableForms) SettleTemplateSend(context.Context, string, string, error) error { return nil }

func TestTheCampaignConsumerRefusesEveryMissingDependency(t *testing.T) {
	consumer, err := NewMessageConsumerUseCase(ConsumerDeps{})
	var missing *deps.MissingError
	if consumer != nil || !errors.As(err, &missing) || len(missing.Missing) != 20 {
		t.Fatalf("consumer = %v err = %v, want every dependency refused", consumer, err)
	}
}
