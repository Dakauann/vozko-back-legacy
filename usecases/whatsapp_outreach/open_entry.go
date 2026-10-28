package whatsapp_outreach

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"vozko/domain/lead"
	"vozko/domain/shared"
	wce "vozko/domain/whatsapp_campaign_entry"
	wo "vozko/domain/whatsapp_outreach"
)

var ErrOpenerIncomplete = errors.New("whatsapp outreach: the entry opener needs phones, leads, entries and the receptive container")

type EntryOpener struct {
	sendRules
}

func NewEntryOpener(deps Deps) (*EntryOpener, error) {
	if deps.Phones == nil || deps.Leads == nil || deps.Entries == nil || deps.EnsureReceptive == nil {
		return nil, ErrOpenerIncomplete
	}
	return &EntryOpener{sendRules: sendRules{deps: deps}}, nil
}

func (o *EntryOpener) Open(_ context.Context, in wo.OpenEntryInput) (*wo.OpenedEntry, error) {
	number := lead.NormalizeNumber(in.PhoneNumber)
	if number == "" {
		return nil, wo.ErrInvalidPhone
	}
	phone, err := o.sendablePhone(in.WorkspaceID, in.BusinessPhoneID)
	if err != nil {
		return nil, err
	}
	campaign, _, err := o.deps.EnsureReceptive.Execute(in.WorkspaceID, phone.ID, phone.DisplayPhoneNumber)
	if err != nil || campaign == nil {
		return nil, fmt.Errorf("whatsapp outreach: could not resolve the conversation container: %w", err)
	}
	leadRecord, _, err := o.deps.Leads.FindOrCreate(in.WorkspaceID, number, lead.LeadUpdate{Source: lead.SourceForm, Name: strings.TrimSpace(in.Name)})
	if err != nil {
		return nil, err
	}
	opened := &wo.OpenedEntry{EntryType: string(shared.EntryTypeWhatsApp), LeadID: leadRecord.ID}
	if existing, found := findExistingEntry(o.deps.Entries, in.WorkspaceID, number, phone.ID); found {
		opened.EntryID, opened.Existed = existing.ID, true
		return opened, nil
	}
	entry, existed, err := createEntry(o.deps.Entries, campaign.ID, leadRecord.ID, nil)
	if err != nil {
		return nil, err
	}
	opened.EntryID, opened.Existed = entry.ID, existed
	return opened, nil
}

func findExistingEntry(entries wce.Repository, workspaceID, number, phoneID string) (*wce.WhatsAppCampaignEntry, bool) {
	existing, err := entries.FindByNumberBusinessPhoneAndWorkspace(number, phoneID, workspaceID)
	if err != nil || existing == nil {
		return nil, false
	}
	return existing, true
}

func createEntry(entries wce.Repository, campaignID, leadID string, variables []string) (*wce.WhatsAppCampaignEntry, bool, error) {
	entry := &wce.WhatsAppCampaignEntry{
		ID:         uuid.New().String(),
		CampaignID: campaignID,
		LeadID:     leadID,
		Status:     wce.SendStatusPending,
		Variables:  variables,
	}
	err := entries.Create(entry)
	if err == nil {
		return entry, false, nil
	}
	if !errors.Is(err, wce.ErrEntryDuplicate) {
		return nil, false, err
	}
	existing, findErr := entries.FindByCampaignAndLead(campaignID, leadID)
	if findErr != nil || existing == nil {
		return nil, false, err
	}
	return existing, true, nil
}
