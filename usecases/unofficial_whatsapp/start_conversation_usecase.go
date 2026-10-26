package unofficial_whatsapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	ia "vozko/domain/inbox_assignment"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	uw "vozko/domain/unofficial_whatsapp"
)

var (
	ErrNotOnWhatsApp = errors.New("unofficial whatsapp: this number is not on WhatsApp")
	ErrInvalidPhone  = errors.New("unofficial whatsapp: not a valid phone number")
)

const minPhoneDigits = 8

type StarterClaimer interface {
	ClaimForStarter(in ia.ConversationStart) error
}

type StartConversationUseCase struct {
	instances     uw.InstanceRepository
	servers       uw.ServerRepository
	contacts      uw.ContactRepository
	conversations uw.ConversationRepository
	messaging     uw.MessagingAPI
	leads         LeadLinker
	starts        StarterClaimer
}

type StartConversationDeps struct {
	Instances     uw.InstanceRepository
	Servers       uw.ServerRepository
	Contacts      uw.ContactRepository
	Conversations uw.ConversationRepository
	Messaging     uw.MessagingAPI
	Leads         LeadLinker
	Starts        StarterClaimer
}

func NewStartConversationUseCase(d StartConversationDeps) (*StartConversationUseCase, error) {
	if err := deps.Require("unofficial whatsapp start conversation", map[string]any{
		"instance repository":     d.Instances,
		"server repository":       d.Servers,
		"contact repository":      d.Contacts,
		"conversation repository": d.Conversations,
		"messaging api":           d.Messaging,
		"starter rule":            d.Starts,
	}); err != nil {
		return nil, err
	}
	return &StartConversationUseCase{
		instances:     d.Instances,
		servers:       d.Servers,
		contacts:      d.Contacts,
		conversations: d.Conversations,
		messaging:     d.Messaging,
		leads:         d.Leads,
		starts:        d.Starts,
	}, nil
}

type StartConversationInput struct {
	WorkspaceID string
	UserID      string
	InstanceID  string
	PhoneNumber string
	Name        string
	Scope       uw.DepartmentScope
}

type StartedConversation struct {
	ConversationID string
	ContactID      string
	PhoneNumber    string
	DisplayName    string
	AlreadyExisted bool
}

func (uc *StartConversationUseCase) Execute(
	ctx context.Context,
	in StartConversationInput,
) (*StartedConversation, error) {
	phone := uw.NormalizePhone(in.PhoneNumber)
	if len(phone) < minPhoneDigits {
		return nil, ErrInvalidPhone
	}

	instance, err := uc.instances.FindByID(ctx, in.InstanceID)
	if err != nil {
		return nil, err
	}
	if err := EnsureVisible(instance, in.WorkspaceID, in.Scope); err != nil {
		return nil, err
	}
	if ok, err := instance.CanSend(time.Now().UTC()); !ok {
		return nil, err
	}

	server, err := uc.servers.FindByID(ctx, instance.ServerID)
	if err != nil {
		return nil, err
	}
	ref := uw.RefFor(server, instance)

	checks, err := uc.messaging.CheckNumbers(ctx, ref, []string{phone})
	if err != nil {
		return nil, fmt.Errorf("unofficial whatsapp: could not verify the number: %w", err)
	}
	if len(checks) == 0 || !checks[0].Reachable() {
		return nil, ErrNotOnWhatsApp
	}
	check := checks[0]

	resolved, err := uc.resolver().Resolve(ctx, instance, ResolveInput{
		JID:         check.JID,
		LID:         check.LID,
		PhoneNumber: phone,
		Name:        firstNonEmpty(check.VerifiedName, in.Name),
	})
	if err != nil {
		return nil, err
	}
	contact, conv, alreadyExisted := resolved.Contact, resolved.Conversation, resolved.AlreadyExisted

	if err := uc.starts.ClaimForStarter(ia.ConversationStart{
		WorkspaceID: in.WorkspaceID,
		EntryID:     conv.ID,
		EntryType:   string(shared.EntryTypeUnofficialWhatsApp),
		AccountID:   instance.ID,
		StarterID:   in.UserID,
	}); err != nil {
		return nil, err
	}

	return &StartedConversation{
		ConversationID: conv.ID,
		ContactID:      contact.ID,
		PhoneNumber:    phone,
		DisplayName:    contact.DisplayName(),
		AlreadyExisted: alreadyExisted,
	}, nil
}

func (uc *StartConversationUseCase) resolver() *ConversationResolver {
	return NewConversationResolver(uc.contacts, uc.conversations, uc.leads)
}
