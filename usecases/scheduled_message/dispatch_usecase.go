package scheduled_message_usecase

import (
	"context"
	"errors"
	"log"

	"vozko/domain/balance"
	"vozko/domain/conversation"
	sm "vozko/domain/scheduled_message"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	"vozko/domain/whatsapp/template"
	wo "vozko/domain/whatsapp_outreach"
	"vozko/domain/workspace"
	"vozko/domain/workspace/workspace_plan"
)

type AutomationStanding interface {
	MayAct(workspaceID, entryID string, entryType shared.EntryType) (bool, error)
}

type DispatchDeps struct {
	Repo           sm.Repository
	Windows        sm.WindowReader
	Send           conversation.OperatorSendUseCase
	AutomationSend conversation.AutomationSendUseCase
	Templates      wo.ConversationTemplateUseCase
	Permissions    workspace.CheckAccessUseCase
	Gate           AutomationStanding
	Broadcaster    conversation.EventBroadcaster
	Clock          sm.Clock
}

type dispatchUseCase struct {
	repo           sm.Repository
	windows        *windowService
	send           conversation.OperatorSendUseCase
	automationSend conversation.AutomationSendUseCase
	templates      wo.ConversationTemplateUseCase
	permission     templatePermission
	gate           AutomationStanding
	broadcaster    conversation.EventBroadcaster
	clock          sm.Clock
}

func NewDispatchUseCase(d DispatchDeps) (sm.DispatchUseCase, error) {
	if err := deps.Require("scheduled message dispatch use case", map[string]any{
		"repository":      d.Repo,
		"window reader":   d.Windows,
		"operator send":   d.Send,
		"automation send": d.AutomationSend,
		"template sender": d.Templates,
		"permissions":     d.Permissions,
		"automation gate": d.Gate,
		"broadcaster":     d.Broadcaster,
		"clock":           d.Clock,
	}); err != nil {
		return nil, err
	}
	windowSvc, err := newWindowService(d.Windows, d.Clock)
	if err != nil {
		return nil, err
	}
	return &dispatchUseCase{
		repo:           d.Repo,
		windows:        windowSvc,
		send:           d.Send,
		automationSend: d.AutomationSend,
		templates:      d.Templates,
		permission:     templatePermission{access: d.Permissions},
		gate:           d.Gate,
		broadcaster:    d.Broadcaster,
		clock:          d.Clock,
	}, nil
}

func (uc *dispatchUseCase) Execute(ctx context.Context, id string) error {
	message, err := uc.repo.ClaimForDispatch(id, uc.clock.Now())
	if err != nil {
		return err
	}
	if message == nil {
		return nil
	}

	return uc.deliver(ctx, message)
}

func (uc *dispatchUseCase) DispatchClaimed(ctx context.Context, message *sm.ScheduledMessage) error {
	if message == nil {
		return nil
	}
	return uc.deliver(ctx, message)
}

func (uc *dispatchUseCase) deliver(ctx context.Context, message *sm.ScheduledMessage) error {
	if message.ByAutomation() {
		if proceed, err := uc.automationStillActs(message); !proceed {
			return err
		}
	}
	if message.Kind == sm.KindTemplate {
		return uc.deliverTemplate(ctx, message)
	}
	return uc.deliverText(ctx, message)
}

func (uc *dispatchUseCase) deliverText(ctx context.Context, message *sm.ScheduledMessage) error {
	entryType := string(message.EntryType)

	if !uc.windows.IsOpen(message.EntryID, entryType) {
		return uc.fail(message, sm.ReasonWindowClosed, "the messaging window was closed at delivery time")
	}

	sent, err := uc.sendText(ctx, message)
	if err != nil {
		return uc.fail(message, classify(err), err.Error())
	}

	if err := uc.markSent(message, sent.ID); err != nil {
		return err
	}

	uc.broadcaster.BroadcastNewMessage(message.EntryID, entryType, sent)
	return nil
}

func (uc *dispatchUseCase) sendText(ctx context.Context, message *sm.ScheduledMessage) (*conversation.Message, error) {
	entryType := string(message.EntryType)
	if message.ByAutomation() {
		return uc.automationSend.Execute(ctx, conversation.AutomationSendInput{
			EntryID:     message.EntryID,
			EntryType:   entryType,
			WorkspaceID: message.WorkspaceID,
			Author:      message.CreatedByAutomation,
			Text:        message.Text,
		})
	}
	return uc.send.Execute(ctx, conversation.OperatorSendInput{
		EntryID:          message.EntryID,
		EntryType:        entryType,
		WorkspaceID:      message.WorkspaceID,
		SenderUserID:     message.CreatedByUserID,
		Text:             message.Text,
		MediaID:          deref(message.MediaID),
		MediaType:        deref(message.MediaType),
		ReplyToMessageID: deref(message.ReplyToMessageID),
		Signed:           message.Signed,
	})
}

func (uc *dispatchUseCase) automationStillActs(message *sm.ScheduledMessage) (bool, error) {
	mayAct, err := uc.gate.MayAct(message.WorkspaceID, message.EntryID, message.EntryType)
	switch {
	case errors.Is(err, conversation.ErrConversationNotFound):
		return false, uc.fail(message, sm.ReasonEntryUnavailable, err.Error())
	case err != nil:
		return false, uc.fail(message, sm.ReasonCheckUnavailable, "who may answer this conversation could not be read: "+err.Error())
	case !mayAct:
		return false, uc.standDown(message)
	}
	return true, nil
}

func (uc *dispatchUseCase) standDown(message *sm.ScheduledMessage) error {
	log.Printf("[scheduled_message] %s by %s not sent on %s (%s): automation may no longer act on the conversation",
		message.ID, message.CreatedByAutomation, message.EntryID, message.EntryType)
	return uc.repo.CancelClaimed(message.ID, sm.CancelAutomationStoodDown)
}

func (uc *dispatchUseCase) deliverTemplate(ctx context.Context, message *sm.ScheduledMessage) error {
	if !message.ByAutomation() {
		creator := shared.Person{UserID: message.CreatedByUserID}
		if err := uc.permission.require(creator, message.WorkspaceID); err != nil {
			reason := sm.ReasonPermissionRevoked
			if !errors.Is(err, sm.ErrTemplatePermission) {
				reason = sm.ReasonDispatchInterrupted
			}
			return uc.fail(message, reason, err.Error())
		}
	}

	if _, err := uc.templates.Send(ctx, templateSend(message)); err != nil {
		return uc.fail(message, classify(err), err.Error())
	}

	return uc.markSent(message, "")
}

func (uc *dispatchUseCase) markSent(message *sm.ScheduledMessage, sentMessageID string) error {
	if err := uc.repo.MarkSent(message.ID, sentMessageID, uc.clock.Now()); err != nil {
		log.Printf("[scheduled_message] %s was DELIVERED but could not be marked sent: %v", message.ID, err)
		return err
	}
	return nil
}

func (uc *dispatchUseCase) fail(message *sm.ScheduledMessage, reason sm.FailureReason, detail string) error {
	log.Printf("[scheduled_message] %s failed on %s (%s): %s=%s",
		message.ID, message.EntryID, message.EntryType, reason, detail)

	if err := uc.repo.MarkFailed(message.ID, reason, detail); err != nil {
		return err
	}
	return nil
}

func classify(err error) sm.FailureReason {
	switch {
	case errors.Is(err, conversation.ErrWindowClosed),
		errors.Is(err, conversation.ErrOutboundWindowClosed):
		return sm.ReasonWindowClosed
	case errors.Is(err, conversation.ErrConversationNotFound),
		errors.Is(err, conversation.ErrEntryTypeInvalid),
		errors.Is(err, conversation.ErrNoAdapterForEntryType),
		errors.Is(err, wo.ErrConversationNotFound),
		errors.Is(err, wo.ErrBusinessPhoneNotFound),
		errors.Is(err, wo.ErrPhoneNotConnected):
		return sm.ReasonEntryUnavailable
	case errors.Is(err, wo.ErrLeadBlocked),
		errors.Is(err, wo.ErrLeadOptedOut),
		errors.Is(err, wo.ErrWithinSpamWindow):
		return sm.ReasonContactIneligible
	case errors.Is(err, wo.ErrTemplateNotFound),
		errors.Is(err, wo.ErrTemplateForbidden),
		errors.Is(err, template.ErrTemplateNotSendable),
		errors.Is(err, template.ErrTemplatePhoneMismatch),
		errors.Is(err, template.ErrTemplateParamsMismatch):
		return sm.ReasonTemplateUnavailable
	case errors.Is(err, balance.ErrInsufficientBalance),
		errors.Is(err, balance.ErrBalanceNotFound):
		return sm.ReasonInsufficientBalance
	case errors.Is(err, balance.ErrMonthlySendCapReached):
		return sm.ReasonMonthlySendCapReached
	case errors.Is(err, balance.ErrSendWindowClosed):
		return sm.ReasonSendWindowClosed
	case errors.Is(err, template.ErrPricingUnavailable),
		errors.Is(err, balance.ErrPriceUnavailable),
		errors.Is(err, workspace_plan.ErrSubscriptionNotCurrent),
		errors.Is(err, workspace_plan.ErrSubscriptionNotActive):
		return sm.ReasonBillingUnavailable
	case errors.Is(err, wo.ErrSendOutcomeUnknown):
		return sm.ReasonOutcomeUnknown
	case errors.Is(err, conversation.ErrRecipientUnreadable):
		return sm.ReasonCheckUnavailable
	default:
		return sm.ReasonProviderError
	}
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

var _ sm.DispatchUseCase = (*dispatchUseCase)(nil)
