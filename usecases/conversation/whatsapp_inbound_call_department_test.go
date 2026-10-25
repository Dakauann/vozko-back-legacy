package conversation_usecase

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	callsession "vozko/domain/callsession"
	conversation_domain "vozko/domain/conversation"
	businessphone "vozko/domain/whatsapp/business_phone"
	wce "vozko/domain/whatsapp_campaign_entry"
)

type inboundPhones struct{}

func (inboundPhones) FindByMetaPhoneNumberID(string) (*businessphone.WhatsAppBusinessPhoneNumber, error) {
	return &businessphone.WhatsAppBusinessPhoneNumber{ID: "phone-1", OwnerWorkspaceID: "ws-1"}, nil
}

type inboundEntries struct{}

func (inboundEntries) FindInboundRouteByNumberAndBusinessPhone(string, string) (*wce.WhatsAppCampaignEntry, error) {
	return &wce.WhatsAppCampaignEntry{ID: "entry-1"}, nil
}

type unassignedEntries struct{}

func (unassignedEntries) GetAssignedUserID(string, string, string) string { return "" }

type unreadableDepartments struct{}

func (unreadableDepartments) GetEntryDepartmentID(string, string) (string, error) {
	return "", errors.New("routes down")
}

type askedEligible struct {
	workspaceAsked  int
	departmentAsked int
}

func (e *askedEligible) GetEligibleUsersForWorkspace(string, bool) []string {
	e.workspaceAsked++
	return []string{"ana"}
}

func (e *askedEligible) GetEligibleUsersForWorkspaceDepartment(string, string, bool) []string {
	e.departmentAsked++
	return []string{"ana"}
}

type rejectingSignaling struct {
	conversation_domain.WhatsAppCallSignaling
	rejected []string
}

func (s *rejectingSignaling) RejectCall(_ context.Context, _ string, callID string) error {
	s.rejected = append(s.rejected, callID)
	return nil
}

type noAvailableSessions struct {
	callsession.CallSessionRegistry
}

func (noAvailableSessions) ListAvailable(string) []callsession.CallSession { return nil }

func (noAvailableSessions) FindByUser(string, string) (callsession.CallSession, bool) {
	return nil, false
}

type recordedCallEvents struct {
	conversation_domain.MessageRepository
	texts []string
}

func (r *recordedCallEvents) Create(m *conversation_domain.Message) error {
	r.texts = append(r.texts, m.Text)
	return nil
}

func TestAnUnreadableDepartmentRefusesTheCallInsteadOfRingingTheWorkspace(t *testing.T) {
	eligible := &askedEligible{}
	signaling := &rejectingSignaling{}
	events := &recordedCallEvents{}
	uc := NewWhatsAppInboundCallUseCase(WhatsAppInboundConfig{
		Signaling:   signaling,
		Phones:      inboundPhones{},
		Entries:     inboundEntries{},
		Assignment:  unassignedEntries{},
		Departments: unreadableDepartments{},
		Eligible:    eligible,
		Sessions:    noAvailableSessions{},
		Messages:    events,
		Logger:      log.New(io.Discard, "", 0),
	})

	uc.handle(conversation_domain.WhatsAppInboundConnect{CallID: "call-1", MetaPhoneNumberID: "meta-1", FromNumber: "5511999990000"})

	if eligible.workspaceAsked != 0 || eligible.departmentAsked != 0 {
		t.Fatalf("an unreadable department consulted the ring (workspace %d, department %d)", eligible.workspaceAsked, eligible.departmentAsked)
	}
	if len(signaling.rejected) != 1 || signaling.rejected[0] != "call-1" {
		t.Fatalf("rejected = %v, want the call refused", signaling.rejected)
	}
	if len(events.texts) == 0 || events.texts[len(events.texts)-1] != whatsappInboundNobodyAvailable {
		t.Fatalf("timeline = %v, want the missed call recorded as nobody available", events.texts)
	}
}
