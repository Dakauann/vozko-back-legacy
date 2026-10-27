package automationneeds

import (
	"context"
	"errors"
	"testing"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/conversationvars"
	"vozko/domain/shared"
)

type memValueStore struct {
	saved map[string]string
	ref   conversationvars.Ref
	calls int
}

func (m *memValueStore) Values(context.Context, string, shared.EntryType) (map[string]string, error) {
	return m.saved, nil
}

func (m *memValueStore) Save(_ context.Context, ref conversationvars.Ref, values map[string]string, _ string) error {
	m.ref, m.saved = ref, values
	m.calls++
	return nil
}

type access bool

func (a access) CanAccessEntry(string, string, string, string, bool) bool { return bool(a) }

func newQueries(t *testing.T, records *memRecords, store *memValueStore, allowed bool) *Queries {
	t.Helper()
	return newQueriesWith(t, records, store, allowed, nil)
}

func newQueriesWith(t *testing.T, records *memRecords, store *memValueStore, allowed bool, adjust func(*QueriesDeps)) *Queries {
	t.Helper()
	service := newService(t, fakeValues{})
	alerts, _ := NewAlerts(&recordedPublisher{})
	guard, err := NewPublishGuard(PublishGuardDeps{Service: service, Slots: fakeSlots{{Kind: domain.SlotInstagram, ID: "ig", Running: []conversation.Automation{{Kind: conversation.AutomationAgent, ID: "livre"}}}}, Alerts: alerts, Held: &fakeHeld{}, HandOff: &recordedHandOffs{}, Containers: &forgotten{}})
	if err != nil {
		t.Fatal(err)
	}
	d := QueriesDeps{
		Service: service, Guard: guard,
		Agents:    fakeAgents{"livre": {ID: "livre", WorkspaceID: "ws", Name: "Livre", MessagingPrompt: "agora {{turma}}"}},
		Workflows: fakeWorkflows{},
		Records:   records, Store: store, Access: access(allowed),
		Conversations: &fakeSnapshots{}, Definitions: fakeDefinitions{}, Subjects: &fakeSubjects{}, Profiles: &fakeProfiles{},
	}
	if adjust != nil {
		adjust(&d)
	}
	q, err := NewQueries(d)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestEligibilityMarksWhatReceptiveCannotRun(t *testing.T) {
	q := newQueries(t, &memRecords{}, &memValueStore{}, true)
	got, err := q.Eligibility("ws", []conversation.Automation{{Kind: conversation.AutomationAgent, ID: "escola"}, {Kind: conversation.AutomationAgent, ID: "livre"}, {Kind: conversation.AutomationAgent, ID: "sumiu"}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Receptive() || got[0].Needs.Names()[0] != "turma" || got[0].Automation.Name != "Escola" {
		t.Fatalf("escola = %+v", got[0])
	}
	if !got[1].Receptive() || got[1].Automation.Name != "Livre" {
		t.Fatalf("livre = %+v", got[1])
	}
	if got[2].Readable || got[2].Receptive() {
		t.Fatalf("an unreadable automation is never eligible: %+v", got[2])
	}
	if _, err := q.Eligibility("ws", make([]conversation.Automation, 101)); !errors.Is(err, ErrTooMany) {
		t.Fatalf("err = %v", err)
	}
}

func TestPublishImpactReadsTheDraft(t *testing.T) {
	q := newQueries(t, &memRecords{}, &memValueStore{}, true)
	got, err := q.PublishImpact(context.Background(), "ws", conversation.Automation{Kind: conversation.AutomationAgent, ID: "livre"})
	if err != nil || len(got) != 1 || got[0].Slot.ID != "ig" {
		t.Fatalf("impact = %+v, %v", got, err)
	}
	if _, err := q.PublishImpact(context.Background(), "other", conversation.Automation{Kind: conversation.AutomationAgent, ID: "livre"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("another workspace = %v", err)
	}
}

func TestTheOpenSwitchOffSaysWhetherItCanBeTurnedBackOn(t *testing.T) {
	records := &memRecords{open: &domain.SwitchOffRecord{Automation: domain.Link{Automation: conversation.Automation{Kind: conversation.AutomationAgent, ID: "livre"}}}}
	got, err := newQueries(t, records, &memValueStore{}, true).OpenSwitchOff(context.Background(), "ws", domain.SlotInstagram, "ig")
	if err != nil || got == nil || !got.EligibleNow {
		t.Fatalf("open = %+v, %v; the published livre needs nothing", got, err)
	}
	records.open = &domain.SwitchOffRecord{Automation: domain.Link{Automation: conversation.Automation{Kind: conversation.AutomationAgent, ID: "escola"}}}
	got, _ = newQueries(t, records, &memValueStore{}, true).OpenSwitchOff(context.Background(), "ws", domain.SlotInstagram, "ig")
	if got.EligibleNow {
		t.Fatal("escola still needs turma")
	}
}
