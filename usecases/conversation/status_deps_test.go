package conversation_usecase

import (
	"context"
	"errors"
	"testing"

	aa "vozko/domain/ai_attendance"
	"vozko/domain/conversation"
	conv_event "vozko/domain/conversation_event"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	wce "vozko/domain/whatsapp_campaign_entry"
)

type statusNoOutcomes struct{}

func (statusNoOutcomes) OutcomeCaptureFor(context.Context, string) (*conversation.OutcomeCapture, error) {
	return nil, nil
}

type statusQuietEvents struct{}

func (statusQuietEvents) Log(*conv_event.ConversationEvent) {}

type statusQuietAnnouncer struct{}

func (statusQuietAnnouncer) AnnounceStatus(string, string, conversation.ConversationStatus, conversation.CloseRecord) {
}

type statusQuietSessions struct{}

func (statusQuietSessions) End(aa.EndRequest) {}

type statusNoDelegations struct {
	conversation.DelegationRepository
}

func (statusNoDelegations) Delete(context.Context, string, shared.EntryType) error { return nil }

type statusUnusedDepartments struct{ EntryDepartmentResolver }

type statusNoAgentRuns struct{}

func (statusNoAgentRuns) CancelAgentRunsOfFinishedConversation(string, string) error { return nil }

func statusWith(repo wce.Repository, d StatusDeps) *ConversationStatusService {
	d.WhatsApp = repo
	if d.Delegations == nil {
		d.Delegations = statusNoDelegations{}
	}
	if d.Events == nil {
		d.Events = statusQuietEvents{}
	}
	if d.WorkspaceOf == nil {
		d.WorkspaceOf = func(string, string) string { return "ws1" }
	}
	if d.AISessions == nil {
		d.AISessions = statusQuietSessions{}
	}
	if d.Outcomes == nil {
		d.Outcomes = statusNoOutcomes{}
	}
	if d.Announcer == nil {
		d.Announcer = statusQuietAnnouncer{}
	}
	if d.Departments == nil {
		d.Departments = statusUnusedDepartments{}
	}
	if d.AgentRuns == nil {
		d.AgentRuns = statusNoAgentRuns{}
	}
	svc, err := NewConversationStatusService(d)
	if err != nil {
		panic(err)
	}
	return svc
}

func TestTheStatusServiceRefusesEveryMissingDependency(t *testing.T) {
	svc, err := NewConversationStatusService(StatusDeps{})
	var missing *deps.MissingError
	if svc != nil || !errors.As(err, &missing) || len(missing.Missing) != 9 {
		t.Fatalf("svc = %v err = %v, want every dependency refused", svc, err)
	}
}

func TestAStatusChangeOnAChannelWithNoStoreIsRefused(t *testing.T) {
	svc := statusWith(&statusEntryRepo{}, StatusDeps{})
	err := svc.SetConversationStatus("entry-1", string(shared.EntryTypeTelegram), conversation.ConversationStatusOngoing)
	if !errors.Is(err, ErrStatusStoreMissing) {
		t.Fatalf("err = %v, want ErrStatusStoreMissing", err)
	}
}
