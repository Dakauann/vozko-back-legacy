package unofficial_whatsapp_campaign

import (
	"errors"
	"testing"

	"vozko/domain/campaign"
	"vozko/domain/lead"
	"vozko/domain/shared/deps"
	uwc "vozko/domain/unofficial_whatsapp_campaign"
	"vozko/usecases/campaignautomation"
)

type unusedKeys struct{ IdempotentCampaigns }

type noLeadsByID struct{}

func (noLeadsByID) FindByIDs(string, []string) ([]*lead.Lead, error) { return nil, nil }

type refusingRules struct{}

func (refusingRules) Rule(string, campaign.Automation) (campaignautomation.Rule, error) {
	return campaignautomation.Rule{}, campaign.ErrAutomationUnavailable
}

func createWith(d CreateCampaignDeps) *createCampaignUseCase {
	if d.Campaigns == nil {
		d.Campaigns = newFakeCampaignRepo()
	}
	if d.Entries == nil {
		d.Entries = newFakeEntryRepo()
	}
	if d.Leads == nil {
		d.Leads = newFakeLeadRepo()
	}
	if d.Instances == nil {
		d.Instances = &fakeGateway{}
	}
	if d.Eligibility == nil {
		d.Eligibility = &fakeSpam{}
	}
	if d.Departments == nil {
		d.Departments = fakeDepartments{id: "dept-1"}
	}
	if d.LeadsByID == nil {
		d.LeadsByID = noLeadsByID{}
	}
	if d.Automation == nil {
		d.Automation = automationThatAllows{}
	}
	if d.Keyed == nil {
		d.Keyed = unusedKeys{}
	}
	if d.GroupSenders == nil {
		d.GroupSenders = &fakeSenderDirectory{}
	}
	uc, err := NewCreateCampaignUseCase(d)
	if err != nil {
		panic(err)
	}
	return uc.(*createCampaignUseCase)
}

func updateWith(campaigns uwc.Repository, entries uwc.EntryRepository, instances InstanceGateway) uwc.UpdateCampaignUseCase {
	uc, err := NewUpdateCampaignUseCase(campaigns, entries, instances, refusingRules{})
	if err != nil {
		panic(err)
	}
	return uc
}

func addEntriesWith(campaigns uwc.Repository, entries uwc.EntryRepository, leads LeadResolver) uwc.AddEntriesUseCase {
	uc, err := NewAddEntriesUseCase(campaigns, entries, leads, automationThatAllows{})
	if err != nil {
		panic(err)
	}
	return uc
}

func TestUnofficialCampaignUseCasesRefuseMissingDependencies(t *testing.T) {
	checks := map[string]func() error{
		"create": func() error { _, err := NewCreateCampaignUseCase(CreateCampaignDeps{}); return err },
		"update": func() error {
			_, err := NewUpdateCampaignUseCase(newFakeCampaignRepo(), newFakeEntryRepo(), &fakeGateway{}, nil)
			return err
		},
		"add entries": func() error {
			_, err := NewAddEntriesUseCase(newFakeCampaignRepo(), newFakeEntryRepo(), newFakeLeadRepo(), nil)
			return err
		},
		"quick send without shared state": func() error {
			_, err := NewQuickSendUseCase(newFakeCampaignRepo(), newFakeEntryRepo(), newFakeLeadRepo(), nil, nil, automationThatAllows{})
			return err
		},
	}
	for name, build := range checks {
		t.Run(name, func(t *testing.T) {
			if err := build(); !errors.Is(err, deps.ErrMissing) {
				t.Fatalf("err = %v, want a missing dependency refused", err)
			}
		})
	}
}

func mustQuickSend(campaigns uwc.Repository, entries uwc.EntryRepository, leads LeadResolver, dispatch uwc.DispatchCampaignUseCase) uwc.QuickSendUseCase {
	uc, err := NewQuickSendUseCase(campaigns, entries, leads, dispatch, newFakeShared(), automationThatAllows{})
	if err != nil {
		panic(err)
	}
	return uc
}

type quietEntryBroadcaster struct{}

func (quietEntryBroadcaster) BroadcastEntryUpdate(string, string, interface{}) {}

func TestTheUnofficialConsumerRefusesMissingDependencies(t *testing.T) {
	consumer, err := NewMessageConsumerUseCase(ConsumerDeps{})
	var missing *deps.MissingError
	if consumer != nil || !errors.As(err, &missing) || len(missing.Missing) != 14 {
		t.Fatalf("consumer = %v err = %v, want every required dependency refused", consumer, err)
	}
}
