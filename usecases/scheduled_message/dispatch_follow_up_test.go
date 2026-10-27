package scheduled_message_usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"vozko/domain/balance"
	"vozko/domain/conversation"
	sm "vozko/domain/scheduled_message"
	"vozko/domain/shared"
	wo "vozko/domain/whatsapp_outreach"
)

func (f *dispatchFixture) followUp(id string, kind sm.Kind) *sm.ScheduledMessage {
	m := &sm.ScheduledMessage{
		ID:                  id,
		WorkspaceID:         "ws-1",
		EntryID:             "entry-1",
		EntryType:           shared.EntryTypeWhatsApp,
		CreatedByAutomation: "ai:agent-1",
		CancelOnReply:       true,
		Kind:                kind,
		ScheduledAt:         fixedNow,
		Status:              sm.StatusPending,
	}
	if kind == sm.KindTemplate {
		m.Template = &sm.TemplateContent{ID: "tpl-1", Name: "lembrete", BodyParams: []string{"Ana"}}
	} else {
		m.Text = "Oi, conseguiu enviar o comprovante?"
	}
	return f.repo.put(m)
}

func TestAFollowUpGoesOutAsTheAutomationThatScheduledIt(t *testing.T) {
	f := newDispatchFixture(t)
	f.followUp("f-1", sm.KindText)

	if err := f.uc.Execute(context.Background(), "f-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if f.send.count() != 0 {
		t.Fatal("a follow-up must not be sent as an operator")
	}
	if f.automation.count() != 1 {
		t.Fatalf("automation sends = %d, want 1", f.automation.count())
	}
	in := f.automation.calls[0]
	if in.Author != "ai:agent-1" || in.Text != "Oi, conseguiu enviar o comprovante?" || in.WorkspaceID != "ws-1" {
		t.Fatalf("send = %+v", in)
	}
	if got := f.repo.get("f-1"); got.Status != sm.StatusSent || got.SentMessageID == nil || *got.SentMessageID != "msg-auto" {
		t.Fatalf("stored = %+v", got)
	}
	if f.broadcaster.messages != 1 {
		t.Fatalf("broadcasts = %d", f.broadcaster.messages)
	}
}

func TestAFollowUpIsStoodDownWhenAPersonNowOwnsTheConversation(t *testing.T) {
	for _, kind := range []sm.Kind{sm.KindText, sm.KindTemplate} {
		t.Run(string(kind), func(t *testing.T) {
			f := newDispatchFixture(t)
			f.followUp("f-1", kind)
			f.gate.refuse = true

			if err := f.uc.Execute(context.Background(), "f-1"); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if f.automation.count()+f.templates.sendCount()+f.send.count() != 0 {
				t.Fatal("a follow-up went out on a conversation a person owns")
			}
			got := f.repo.get("f-1")
			if got.Status != sm.StatusCanceled || got.CancelReason == nil || *got.CancelReason != sm.CancelAutomationStoodDown {
				t.Fatalf("stored = %+v", got)
			}
		})
	}
}

func TestAnUnreadableOwnerFailsTheFollowUpClosed(t *testing.T) {
	f := newDispatchFixture(t)
	f.followUp("f-1", sm.KindText)
	f.gate.err = errors.New("owners table unreachable")

	if err := f.uc.Execute(context.Background(), "f-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if f.automation.count() != 0 {
		t.Fatal("a follow-up went out without knowing who owns the conversation")
	}
	got := f.repo.get("f-1")
	if got.Status != sm.StatusFailed || got.FailureReason == nil || *got.FailureReason != sm.ReasonCheckUnavailable {
		t.Fatalf("stored = %+v", got)
	}
}

func TestAFollowUpOnAConversationThatIsGoneFails(t *testing.T) {
	f := newDispatchFixture(t)
	f.followUp("f-1", sm.KindText)
	f.gate.err = fmt.Errorf("automation of whatsapp entry-1: %w", conversation.ErrConversationNotFound)

	if err := f.uc.Execute(context.Background(), "f-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := f.repo.get("f-1")
	if got.FailureReason == nil || *got.FailureReason != sm.ReasonEntryUnavailable {
		t.Fatalf("stored = %+v", got)
	}
}

func TestAFollowUpRefusedByTheSendRulesFailsWithItsReason(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want sm.FailureReason
	}{
		{"opted out", wo.ErrLeadOptedOut, sm.ReasonContactIneligible},
		{"blocked", wo.ErrLeadBlocked, sm.ReasonContactIneligible},
		{"no balance", balance.ErrInsufficientBalance, sm.ReasonInsufficientBalance},
		{"monthly cap", balance.ErrMonthlySendCapReached, sm.ReasonMonthlySendCapReached},
		{"window shut", conversation.ErrOutboundWindowClosed, sm.ReasonWindowClosed},
		{"contact unreadable", fmt.Errorf("lead lookup: %w", conversation.ErrRecipientUnreadable), sm.ReasonCheckUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDispatchFixture(t)
			f.followUp("f-1", sm.KindText)
			f.automation.err = tc.err

			if err := f.uc.Execute(context.Background(), "f-1"); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			got := f.repo.get("f-1")
			if got.Status != sm.StatusFailed || got.FailureReason == nil || *got.FailureReason != tc.want {
				t.Fatalf("stored = %+v, want %s", got, tc.want)
			}
		})
	}
}

func TestAFollowUpTemplateNeedsNoPersonPermissionAndCarriesItsAuthor(t *testing.T) {
	f := newDispatchFixture(t)
	f.followUp("f-1", sm.KindTemplate)
	f.windows.set(false, nil)

	if err := f.uc.Execute(context.Background(), "f-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if f.permissions.calls() != 0 {
		t.Fatal("an automation follow-up has no person whose permission could be checked")
	}
	if f.templates.sendCount() != 1 {
		t.Fatalf("template sends = %d", f.templates.sendCount())
	}
	sent := f.templates.sends[0]
	if sent.Automation != "ai:agent-1" || sent.UserID != "" || sent.IdempotencyKey != "scheduled-message:f-1" {
		t.Fatalf("send = %+v", sent)
	}
	if f.gate.count() != 1 {
		t.Fatalf("the automation gate was read %d times, want once at send time", f.gate.count())
	}
}

func TestAFollowUpTemplateRefusedByTheSendRulesFails(t *testing.T) {
	f := newDispatchFixture(t)
	f.followUp("f-1", sm.KindTemplate)
	f.templates.sendErr = wo.ErrLeadOptedOut

	if err := f.uc.Execute(context.Background(), "f-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := f.repo.get("f-1"); got.FailureReason == nil || *got.FailureReason != sm.ReasonContactIneligible {
		t.Fatalf("stored = %+v", got)
	}
}

func TestAPersonsScheduledMessageNeverAsksTheAutomationGate(t *testing.T) {
	f := newDispatchFixture(t)
	f.pending("sched-1")
	f.gate.refuse = true

	if err := f.uc.Execute(context.Background(), "sched-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if f.gate.count() != 0 || f.send.count() != 1 {
		t.Fatalf("gate reads %d, operator sends %d", f.gate.count(), f.send.count())
	}
}
