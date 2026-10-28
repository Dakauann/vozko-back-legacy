package whatsapp_outreach

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/lead"
	businessphone "vozko/domain/whatsapp/business_phone"
	wc "vozko/domain/whatsapp_campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
	wo "vozko/domain/whatsapp_outreach"
)

func openerDeps(entries *fakeEntries) Deps {
	return Deps{
		Phones: &fakePhones{phone: &businessphone.WhatsAppBusinessPhoneNumber{
			ID: "bp-1", OwnerWorkspaceID: "ws-1", DisplayPhoneNumber: "5511888888888", Status: businessphone.StatusConnected,
		}},
		Leads:           &fakeLeads{rec: &lead.Lead{ID: "lead-1", Number: "5511999999999"}},
		Entries:         entries,
		EnsureReceptive: &fakeOrganic{campaign: &wc.Campaign{ID: "camp-1", WorkspaceID: "ws-1", Type: wc.CampaignTypeOrganic}},
	}
}

func openInput() wo.OpenEntryInput {
	return wo.OpenEntryInput{WorkspaceID: "ws-1", BusinessPhoneID: "bp-1", PhoneNumber: "5511999999999", Name: "Ana"}
}

func TestOpeningAnEntryCreatesItOnTheNumbersReceptiveContainerWithoutSending(t *testing.T) {
	entries := &fakeEntries{}
	opener, err := NewEntryOpener(openerDeps(entries))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := opener.Open(context.Background(), openInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries.created) != 1 || entries.created[0].CampaignID != "camp-1" || entries.created[0].LeadID != "lead-1" || entries.created[0].Status != wce.SendStatusPending {
		t.Fatalf("created %+v", entries.created)
	}
	if opened.EntryID != entries.created[0].ID || opened.LeadID != "lead-1" || opened.Existed || opened.EntryType != "whatsapp" {
		t.Fatalf("opened %+v", opened)
	}
}

func TestOpeningAnEntryReusesTheConversationOnThatNumber(t *testing.T) {
	entries := &fakeEntries{existing: &wce.WhatsAppCampaignEntry{ID: "entry-9"}}
	opener, _ := NewEntryOpener(openerDeps(entries))
	opened, err := opener.Open(context.Background(), openInput())
	if err != nil || opened.EntryID != "entry-9" || !opened.Existed || len(entries.created) != 0 {
		t.Fatalf("opened %+v err %v created %d", opened, err, len(entries.created))
	}
}

func TestOpeningAnEntryRefusesANumberTheWorkspaceCannotUse(t *testing.T) {
	deps := openerDeps(&fakeEntries{})
	deps.Phones = &fakePhones{}
	opener, _ := NewEntryOpener(deps)
	if _, err := opener.Open(context.Background(), openInput()); !errors.Is(err, wo.ErrBusinessPhoneNotFound) {
		t.Fatalf("err = %v", err)
	}
	bad := openInput()
	bad.PhoneNumber = "abc"
	opener, _ = NewEntryOpener(openerDeps(&fakeEntries{}))
	if _, err := opener.Open(context.Background(), bad); !errors.Is(err, wo.ErrInvalidPhone) {
		t.Fatalf("err = %v", err)
	}
	if _, err := NewEntryOpener(Deps{}); err == nil {
		t.Fatal("an opener without its dependencies must not be built")
	}
}
