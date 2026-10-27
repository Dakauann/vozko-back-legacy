package automationneeds

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
)

type memRecords struct {
	open     *domain.SwitchOffRecord
	resolved int
}

func (m *memRecords) Open(context.Context, string, domain.SlotKind, string) (*domain.SwitchOffRecord, error) {
	return m.open, nil
}

func (m *memRecords) Resolve(context.Context, string, domain.SlotKind, string, time.Time) (int, error) {
	m.resolved++
	m.open = nil
	return 1, nil
}

func newReceptive(t *testing.T, records *memRecords) (*Receptive, *recordedPublisher) {
	t.Helper()
	publisher := &recordedPublisher{}
	alerts, _ := NewAlerts(publisher)
	r, err := NewReceptive(newService(t, fakeValues{}), records, alerts)
	if err != nil {
		t.Fatal(err)
	}
	return r, publisher
}

func TestReEnablingAChannelSettlesItsSwitchOffNotice(t *testing.T) {
	records := &memRecords{open: &domain.SwitchOffRecord{SlotKind: domain.SlotInstagram, SlotID: "ig", Automation: domain.Link{Automation: conversation.Automation{Kind: conversation.AutomationAgent, ID: "escola"}}}}
	r, publisher := newReceptive(t, records)
	r.Saved(context.Background(), "ws", domain.SlotInstagram, "ig", conversation.ChannelAutomation{AgentID: id("escola")})
	if records.resolved != 0 {
		t.Fatal("saving the same agent still off keeps the notice")
	}
	r.Saved(context.Background(), "ws", domain.SlotInstagram, "ig", conversation.ChannelAutomation{AgentID: id("livre"), EnableAgentResponses: true})
	if records.resolved != 1 || len(publisher.resolved) != 1 || publisher.resolved[0] != "automation_switch_off:instagram:ig" {
		t.Fatalf("resolved = %d, episodes = %v", records.resolved, publisher.resolved)
	}
}

func TestReceptiveAdmitUsesTheSharedRule(t *testing.T) {
	r, _ := newReceptive(t, &memRecords{})
	err := r.Admit("ws", conversation.ChannelAutomation{AgentID: id("escola"), EnableAgentResponses: true})
	if !errors.Is(err, domain.ErrReceptiveUnmet) {
		t.Fatalf("err = %v", err)
	}
}
