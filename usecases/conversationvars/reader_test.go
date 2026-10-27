package conversationvars

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"vozko/domain/conversation"
	domain "vozko/domain/conversationvars"
	"vozko/domain/shared"
)

type fakeFacts struct {
	info conversation.EntryInfo
	err  error
}

func (f fakeFacts) GetEntryInfo(string, string) (conversation.EntryInfo, error) { return f.info, f.err }

type fakeCampaigns struct {
	campaign domain.Campaign
	err      error
}

func (f fakeCampaigns) CampaignOf(context.Context, string, shared.EntryType) (domain.Campaign, error) {
	return f.campaign, f.err
}

type memStore struct {
	values map[string]string
	err    error
	saved  []map[string]string
	refs   []domain.Ref
}

func (m *memStore) Values(context.Context, string, shared.EntryType) (map[string]string, error) {
	return m.values, m.err
}

func (m *memStore) Save(_ context.Context, ref domain.Ref, values map[string]string, _ string) error {
	m.refs = append(m.refs, ref)
	m.saved = append(m.saved, values)
	return m.err
}

func TestReaderLayersEveryPlaceAConversationGetsValuesFrom(t *testing.T) {
	r, err := NewReader(ReaderDeps{
		Facts:     fakeFacts{info: conversation.EntryInfo{LeadName: "Ana", LeadNumber: "5511"}},
		Campaigns: fakeCampaigns{campaign: domain.Campaign{Name: "Volta", Values: map[string]string{"escola": "Prisma", "turma": "A"}}},
		Store:     &memStore{values: map[string]string{"turma": "B"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Values(context.Background(), "e1", shared.EntryTypeInstagram)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"escola": "Prisma", "turma": "B", "lead_name": "Ana", "lead_number": "5511", "campaign_name": "Volta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("values = %v", got)
	}
}

func TestReaderFailsClosedWhenAnyLayerCannotBeRead(t *testing.T) {
	down := errors.New("db down")
	cases := []struct {
		name string
		deps ReaderDeps
	}{
		{"the lead", ReaderDeps{Facts: fakeFacts{err: down}, Campaigns: fakeCampaigns{}, Store: &memStore{}}},
		{"the campaign", ReaderDeps{Facts: fakeFacts{}, Campaigns: fakeCampaigns{err: down}, Store: &memStore{}}},
		{"the stored values", ReaderDeps{Facts: fakeFacts{}, Campaigns: fakeCampaigns{}, Store: &memStore{err: down}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewReader(tc.deps)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Values(context.Background(), "e1", shared.EntryTypeWhatsApp); !errors.Is(err, domain.ErrUnavailable) {
				t.Fatalf("err = %v, want ErrUnavailable", err)
			}
		})
	}
}

func TestNewReaderRefusesMissingDependencies(t *testing.T) {
	if _, err := NewReader(ReaderDeps{}); err == nil {
		t.Fatal("a reader without its sources must not be built")
	}
}
