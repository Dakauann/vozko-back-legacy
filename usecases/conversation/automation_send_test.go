package conversation_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/balance"
	"vozko/domain/conversation"
	"vozko/domain/lead"
	"vozko/domain/shared"
	wo "vozko/domain/whatsapp_outreach"
)

type automationSentText struct {
	entryID, entryType, text string
	origin                   conversation.SendOrigin
}

type automationSender struct {
	sent []automationSentText
	err  error
}

func (s *automationSender) SendTextMessage(entryID, entryType, text string, origin conversation.SendOrigin) (*conversation.Message, error) {
	s.sent = append(s.sent, automationSentText{entryID, entryType, text, origin})
	if s.err != nil {
		return nil, s.err
	}
	return &conversation.Message{ID: "msg-1", EntryID: entryID, EntryType: shared.EntryType(entryType), SentBy: origin.Author()}, nil
}

func (s *automationSender) SendMediaMessage(string, string, string, string, string, conversation.SendOrigin) (*conversation.Message, error) {
	return nil, errors.New("not used")
}

func (s *automationSender) SendButtonMessage(string, string, conversation.SendButtonInput, conversation.SendOrigin) (*conversation.Message, error) {
	return nil, errors.New("not used")
}

type automationBilling struct {
	conversation.ServiceMessageBilling
	err error
}

func (b automationBilling) AllowSend(string) error { return b.err }

type automationEntryLeads struct {
	leadID string
	err    error
}

func (l automationEntryLeads) LeadOfEntry(context.Context, string, shared.EntryRef) (string, error) {
	return l.leadID, l.err
}

type automationLeads struct {
	contact *lead.Lead
	err     error
}

func (l automationLeads) FindByID(string, string) (*lead.Lead, error) {
	return l.contact, l.err
}

type automationSendFixture struct {
	sender  *automationSender
	billing automationBilling
	entries automationEntryLeads
	leads   automationLeads
}

func newAutomationSendFixture() *automationSendFixture {
	return &automationSendFixture{
		sender:  &automationSender{},
		entries: automationEntryLeads{leadID: "lead-1"},
		leads:   automationLeads{contact: &lead.Lead{ID: "lead-1", Number: "5511999990000"}},
	}
}

func (f *automationSendFixture) build(t *testing.T) conversation.AutomationSendUseCase {
	t.Helper()
	uc, err := NewAutomationSend(AutomationSendDeps{Sender: f.sender, Billing: f.billing, EntryLeads: f.entries, Leads: f.leads})
	if err != nil {
		t.Fatalf("NewAutomationSend: %v", err)
	}
	return uc
}

func followUpSend() conversation.AutomationSendInput {
	return conversation.AutomationSendInput{
		EntryID: "entry-1", EntryType: string(shared.EntryTypeTelegram), WorkspaceID: "ws-1",
		Author: "workflow:wf-1", Text: "Oi, conseguiu enviar o comprovante?",
	}
}

func TestAnAutomationSendGoesOutAsItsAuthor(t *testing.T) {
	f := newAutomationSendFixture()
	message, err := f.build(t).Execute(context.Background(), followUpSend())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(f.sender.sent) != 1 {
		t.Fatalf("sends = %d", len(f.sender.sent))
	}
	origin := f.sender.sent[0].origin
	if origin.ByPerson() || origin.Author().Kind() != conversation.SenderWorkflow || origin.SenderUserID != "" {
		t.Fatalf("origin = %+v", origin)
	}
	if message.SentBy.Participant() != "wf-1" {
		t.Fatalf("message = %+v", message)
	}
}

func TestAnAutomationSendKeepsTheSendRules(t *testing.T) {
	optedOut := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		setup func(*automationSendFixture)
		want  error
	}{
		{"no balance", func(f *automationSendFixture) { f.billing.err = balance.ErrInsufficientBalance }, balance.ErrInsufficientBalance},
		{"opted out", func(f *automationSendFixture) { f.leads.contact.OptedOutAt = &optedOut }, wo.ErrLeadOptedOut},
		{"blocked", func(f *automationSendFixture) { f.leads.contact.Blocked = true }, wo.ErrLeadBlocked},
		{"lead unreadable", func(f *automationSendFixture) { f.entries.err = errors.New("db down") }, conversation.ErrRecipientUnreadable},
		{"contact unreadable", func(f *automationSendFixture) { f.leads.err = errors.New("db down") }, conversation.ErrRecipientUnreadable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAutomationSendFixture()
			tc.setup(f)
			if _, err := f.build(t).Execute(context.Background(), followUpSend()); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if len(f.sender.sent) != 0 {
				t.Fatal("a refused automation send went out")
			}
		})
	}
}

func TestAConversationWithoutALeadIsStillReachable(t *testing.T) {
	f := newAutomationSendFixture()
	f.entries = automationEntryLeads{err: lead.ErrLeadNotFound}
	if _, err := f.build(t).Execute(context.Background(), followUpSend()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestAnAutomationSendNeedsAnAutomationAuthorAndText(t *testing.T) {
	f := newAutomationSendFixture()
	uc := f.build(t)
	person := followUpSend()
	person.Author = "user-1"
	if _, err := uc.Execute(context.Background(), person); !errors.Is(err, conversation.ErrAutomationAuthorRequired) {
		t.Fatalf("err = %v", err)
	}
	blank := followUpSend()
	blank.Text = "  "
	if _, err := uc.Execute(context.Background(), blank); !errors.Is(err, conversation.ErrMessageContentRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewAutomationSendNamesMissingDependencies(t *testing.T) {
	if _, err := NewAutomationSend(AutomationSendDeps{}); err == nil {
		t.Fatal("missing dependencies were accepted")
	}
}
