package whatsapp_outreach

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"vozko/domain/conversation"
	ia "vozko/domain/inbox_assignment"
	"vozko/domain/lead"
	lcs "vozko/domain/lead_campaign_send"
	"vozko/domain/shared"
	businessphone "vozko/domain/whatsapp/business_phone"
	"vozko/domain/whatsapp/template"
	wc "vozko/domain/whatsapp_campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
	wo "vozko/domain/whatsapp_outreach"
	"vozko/domain/whatsappform"
	"vozko/domain/workspace_phone_access"
	"vozko/domain/workspace_template_access"
	"vozko/usecases/campaignguard"
)

type SpamPolicyReader = campaignguard.SpamPolicy

type WindowReader interface {
	IsWindowOpen(leadID, businessPhoneID string) (bool, error)
}

type ConversationClaimer interface {
	ClaimIfUnassigned(entryID, entryType, businessPhoneID, workspaceID, userID, trigger string) (bool, error)
}

type StarterHolder interface {
	HoldForStarter(in ia.ConversationStart) (ia.StartHold, error)
	ConfirmStart(hold ia.StartHold)
	ReleaseStart(hold ia.StartHold) error
}

type Deps struct {
	Phones        businessphone.Repository
	PhoneGrants   workspace_phone_access.Repository
	Templates     template.Repository
	TemplateGrant workspace_template_access.CheckAccessUseCase

	Leads           lead.Repository
	Entries         wce.Repository
	Campaigns       wc.Repository
	EnsureReceptive wc.EnsureReceptiveContainerUseCase
	Windows         WindowReader
	CampaignSends   lcs.Repository
	SpamPolicy      SpamPolicyReader
	SendClaims      campaignguard.SendClaims
	History         conversation.MessageHistoryManager
	Assignments     ConversationClaimer
	Starts          StarterHolder
	Sender          template.BilledTemplateSendUseCase
	Now             func() time.Time
}

type startConversationUseCase struct {
	sendRules
}

func NewStartConversationUseCase(deps Deps) (wo.StartOfficialConversationUseCase, error) {
	rules, err := newSendRules(deps, map[string]bool{
		"organic campaign use case": deps.EnsureReceptive != nil,
		"conversation starts":       deps.Starts != nil,
	})
	if err != nil {
		return nil, err
	}
	return &startConversationUseCase{sendRules: rules}, nil
}

func (uc *startConversationUseCase) Execute(ctx context.Context, in wo.StartConversationInput) (*wo.StartedConversation, error) {
	if strings.TrimSpace(in.WorkspaceID) == "" {
		return nil, template.ErrWorkspaceRequired
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return nil, template.ErrIdempotencyKeyRequired
	}

	number := lead.NormalizeNumber(in.PhoneNumber)
	if number == "" {
		return nil, wo.ErrInvalidPhone
	}

	phone, err := uc.sendablePhone(in.WorkspaceID, in.BusinessPhoneID)
	if err != nil {
		return nil, err
	}

	tmpl, err := uc.grantedTemplate(in.WorkspaceID, in.TemplateID)
	if err != nil {
		return nil, err
	}

	campaign, _, err := uc.deps.EnsureReceptive.Execute(in.WorkspaceID, phone.ID, phone.DisplayPhoneNumber)
	if err != nil || campaign == nil {
		return nil, fmt.Errorf("whatsapp outreach: could not resolve the conversation container: %w", err)
	}
	if !uc.departmentAllows(in, campaign) {
		return nil, wo.ErrDepartmentForbidden
	}

	leadRecord, _, err := uc.deps.Leads.FindOrCreate(in.WorkspaceID, number, lead.LeadUpdate{Source: lead.SourceImport, Name: strings.TrimSpace(in.Name)})
	if err != nil {
		return nil, err
	}
	if err := uc.refuseUnreachable(leadRecord); err != nil {
		return nil, err
	}

	entry, entryExisted := uc.findExistingEntry(in.WorkspaceID, number, phone.ID)

	if uc.deps.Windows != nil && !tmpl.IsAuthentication() {
		open, windowErr := uc.deps.Windows.IsWindowOpen(leadRecord.ID, phone.ID)
		if windowErr != nil {
			return nil, fmt.Errorf("whatsapp outreach: could not read the messaging window: %w", windowErr)
		}
		if open {
			result := &wo.StartedConversation{
				LeadID:              leadRecord.ID,
				EntryType:           string(shared.EntryTypeWhatsApp),
				ConversationExisted: entryExisted,
			}
			if entry != nil {
				result.EntryID = entry.ID
			}
			return result, wo.ErrWindowAlreadyOpen
		}
	}

	if err := uc.refuseIfSpam(ctx, in.WorkspaceID, leadRecord.ID, phone.ID); err != nil {
		return nil, err
	}
	release, err := uc.claimContact(ctx, in.WorkspaceID, leadRecord.ID, phone.ID)
	if err != nil {
		return nil, err
	}
	sent := false
	defer func() {
		if !sent {
			release()
		}
	}()

	if entry == nil {
		entry = &wce.WhatsAppCampaignEntry{
			ID:         uuid.New().String(),
			CampaignID: campaign.ID,
			LeadID:     leadRecord.ID,
			Status:     wce.SendStatusPending,
			Variables:  in.BodyParams,
		}
		if createErr := uc.deps.Entries.Create(entry); createErr != nil {
			if errors.Is(createErr, wce.ErrEntryDuplicate) {
				if existing, findErr := uc.deps.Entries.FindByCampaignAndLead(campaign.ID, leadRecord.ID); findErr == nil && existing != nil {
					entry, entryExisted = existing, true
				} else {
					return nil, createErr
				}
			} else {
				return nil, createErr
			}
		}
	}

	hold, err := uc.deps.Starts.HoldForStarter(ia.ConversationStart{
		WorkspaceID: in.WorkspaceID,
		EntryID:     entry.ID,
		EntryType:   string(shared.EntryTypeWhatsApp),
		AccountID:   phone.ID,
		StarterID:   in.UserID,
	})
	if err != nil {
		uc.markEntryFailed(entry.ID, nil, err)
		return nil, err
	}

	d := delivery{
		entry:        entry,
		tmpl:         tmpl,
		bodyParams:   in.BodyParams,
		headerParams: in.HeaderParams,
		workspaceID:  in.WorkspaceID,
		userID:       in.UserID,
		to:           leadRecord.Number,
		leadID:       leadRecord.ID,
		phoneID:      phone.ID,
		campaignID:   campaign.ID,
	}
	sendResult, err := uc.charge(ctx, d, template.BilledSendInput{
		WorkspaceID:     in.WorkspaceID,
		UserID:          in.UserID,
		IdempotencyKey:  in.IdempotencyKey,
		BusinessPhoneID: phone.ID,
		TemplateID:      tmpl.ID,
		ToNumber:        lead.NormalizeWhatsAppNumber(number),
		BodyParams:      in.BodyParams,
		HeaderParams:    in.HeaderParams,
		CampaignID:      campaign.ID,
		EntryID:         entry.ID,
		Origin:          whatsappform.OriginTemplateStart,
		LeadID:          leadRecord.ID,
	})
	sent = keepsClaim(err)
	if err != nil {
		uc.settleStart(hold, sent)
		return nil, err
	}

	result := &wo.StartedConversation{
		EntryID:             entry.ID,
		EntryType:           string(shared.EntryTypeWhatsApp),
		LeadID:              leadRecord.ID,
		AttemptID:           sendResult.AttemptID,
		MessageID:           sendResult.MessageID,
		ConversationExisted: entryExisted,
		Replayed:            sendResult.Replayed,
		ChargedMicros:       sendResult.ChargedMicros,
	}
	if sendResult.Replayed {
		result.Recorded = true
	} else {
		result.Recorded = uc.settle(ctx, d, sendResult)
	}
	uc.settleStart(hold, sent)
	return result, nil
}

func (uc *startConversationUseCase) settleStart(hold ia.StartHold, sent bool) {
	if sent {
		uc.deps.Starts.ConfirmStart(hold)
		return
	}
	if err := uc.deps.Starts.ReleaseStart(hold); err != nil {
		log.Printf("[whatsapp-outreach] the template to entry %s was not sent and the conversation could not be put back as it was: %v", hold.Start.EntryID, err)
	}
}

func (uc *startConversationUseCase) findExistingEntry(workspaceID, number, phoneID string) (*wce.WhatsAppCampaignEntry, bool) {
	existing, err := uc.deps.Entries.FindByNumberBusinessPhoneAndWorkspace(number, phoneID, workspaceID)
	if err != nil || existing == nil {
		return nil, false
	}
	return existing, true
}

func (uc *startConversationUseCase) departmentAllows(in wo.StartConversationInput, campaign *wc.Campaign) bool {
	if in.IsAdmin || len(in.DepartmentIDs) == 0 {
		return true
	}
	if strings.TrimSpace(campaign.DepartmentID) == "" {
		return true
	}
	for _, id := range in.DepartmentIDs {
		if id == campaign.DepartmentID {
			return true
		}
	}
	return false
}
