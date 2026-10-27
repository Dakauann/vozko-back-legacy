package conversation_usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"vozko/domain/campaign"
	"vozko/domain/conversation"
	"vozko/domain/lead"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
)

type AutomationLeads interface {
	FindByID(workspaceID, id string) (*lead.Lead, error)
}

type AutomationSendDeps struct {
	Sender     conversation.MessageSender
	Billing    conversation.ServiceMessageBilling
	EntryLeads lead.EntryLeads
	Leads      AutomationLeads
}

type automationSend struct {
	sender     conversation.MessageSender
	billing    conversation.ServiceMessageBilling
	entryLeads lead.EntryLeads
	leads      AutomationLeads
}

func NewAutomationSend(d AutomationSendDeps) (conversation.AutomationSendUseCase, error) {
	if err := deps.Require("automation send", map[string]any{
		"message sender":          d.Sender,
		"service message billing": d.Billing,
		"entry leads":             d.EntryLeads,
		"leads":                   d.Leads,
	}); err != nil {
		return nil, err
	}
	return &automationSend{sender: d.Sender, billing: d.Billing, entryLeads: d.EntryLeads, leads: d.Leads}, nil
}

func (uc *automationSend) Execute(ctx context.Context, in conversation.AutomationSendInput) (*conversation.Message, error) {
	author, ok := conversation.SentByAutomation(in.Author)
	if !ok {
		return nil, conversation.ErrAutomationAuthorRequired
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return nil, conversation.ErrMessageContentRequired
	}
	if err := uc.billing.AllowSend(in.WorkspaceID); err != nil {
		return nil, err
	}
	if err := uc.refuseUnreachable(ctx, in); err != nil {
		return nil, err
	}
	return uc.sender.SendTextMessage(in.EntryID, in.EntryType, text, conversation.SendOrigin{Automation: author})
}

func (uc *automationSend) refuseUnreachable(ctx context.Context, in conversation.AutomationSendInput) error {
	ref := shared.EntryRef{EntryID: in.EntryID, EntryType: shared.EntryType(in.EntryType)}
	leadID, err := uc.entryLeads.LeadOfEntry(ctx, in.WorkspaceID, ref)
	if errors.Is(err, lead.ErrLeadNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: the lead of %s %s: %w", conversation.ErrRecipientUnreadable, in.EntryType, in.EntryID, err)
	}
	contact, err := uc.leads.FindByID(in.WorkspaceID, leadID)
	if errors.Is(err, lead.ErrLeadNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: lead %s: %w", conversation.ErrRecipientUnreadable, leadID, err)
	}
	return campaign.ContactReplyRefusal(contact, nil)
}

var _ conversation.AutomationSendUseCase = (*automationSend)(nil)
