package whatsapp_campaign_usecase

import (
	"context"
	"testing"

	businessphone "vozko/domain/whatsapp/business_phone"
	wc "vozko/domain/whatsapp_campaign"
)

type denyGrants struct{ asked string }

func (d *denyGrants) HasAccess(workspaceID, templateID string) (bool, error) {
	d.asked = workspaceID + "|" + templateID
	return false, nil
}

func TestCreateCampaignRefusesATemplateNotGrantedToTheWorkspace(t *testing.T) {
	businessPhones := newOwnershipBusinessPhoneRepo()
	businessPhones.phones["phone-1"] = &businessphone.WhatsAppBusinessPhoneNumber{ID: "phone-1", OwnerWorkspaceID: "ws-1"}
	grants := &denyGrants{}
	uc := createWith(CreateCampaignDeps{Phones: businessPhones, PhoneAccess: &ownershipWorkspacePhoneAccessRepo{hasAccess: true}})
	uc.(*createCampaignUseCase).templateGrants = grants

	_, err := uc.Execute(context.Background(), &wc.Campaign{
		WorkspaceID: "ws-1", Name: "Campaign", TemplateID: "tmpl-other", BusinessPhoneID: "phone-1",
		Status: wc.CampaignStatusStopped, PhoneInputs: []wc.PhoneInput{{Number: "5511999999999", Variables: []string{"Alice"}}},
	})
	if err != wc.ErrCampaignTemplateNotFound || grants.asked != "ws-1|tmpl-other" {
		t.Fatalf("err %v asked %q", err, grants.asked)
	}
}

func TestChangingACampaignToATemplateNotGrantedIsRefused(t *testing.T) {
	stored, fixture := updateFixture(t, "")
	counted := &countedCampaigns{mockCampaignRepo: stored}
	built := fixture.(*updateCampaignUseCase)
	uc := updateWith(UpdateCampaignDeps{Campaigns: counted, Entries: built.entryRepo, Templates: built.templateRepo, Phones: built.businessPhoneRepo})
	grants := &denyGrants{}
	uc.(*updateCampaignUseCase).templateGrants = grants
	uc.(*updateCampaignUseCase).exitRisk = &recordingGate{}

	_, err := uc.Execute(context.Background(), "camp-1", &wc.Campaign{Name: "Matrículas", TemplateID: "tmpl-2"})

	if err != wc.ErrCampaignTemplateNotFound || grants.asked != "ws-1|tmpl-2" {
		t.Fatalf("err %v asked %q", err, grants.asked)
	}
	if counted.updates != 0 {
		t.Fatal("a template the workspace was never granted was saved")
	}
}
