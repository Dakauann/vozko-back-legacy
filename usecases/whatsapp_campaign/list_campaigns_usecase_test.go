package whatsapp_campaign_usecase

import (
	"errors"
	"testing"

	"vozko/domain/shared"
	wc "vozko/domain/whatsapp_campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
)

type listWAEntryStub struct {
	wce.Repository
	counts          map[string]*wce.StatusCounts
	gotIDs          []string
	countCalls      int
	countErr        error
	recentByID      map[string][]wce.WhatsAppCampaignEntry
	recentCallCount int
	recentErr       error
	recentIDs       []string
	recentLimit     int
}

func (s *listWAEntryStub) CountByStatusForCampaigns(ids []string) (map[string]*wce.StatusCounts, error) {
	s.countCalls++
	s.gotIDs = ids
	if s.countErr != nil {
		return nil, s.countErr
	}
	return s.counts, nil
}

func (s *listWAEntryStub) ListRecentlyUpdatedForCampaigns(campaignIDs []string, limit int) (map[string][]wce.WhatsAppCampaignEntry, error) {
	s.recentCallCount++
	s.recentIDs = campaignIDs
	s.recentLimit = limit
	if s.recentErr != nil {
		return nil, s.recentErr
	}
	return s.recentByID, nil
}

type listWACampaignStub struct {
	wc.Repository
	result  *shared.PaginatedResult[*wc.Campaign]
	listErr error
}

func (s *listWACampaignStub) List(_ wc.ListCampaignsInput) (*shared.PaginatedResult[*wc.Campaign], error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.result, nil
}

func TestListCampaignsBatchesMetricsInOneCall(t *testing.T) {
	campaigns := []*wc.Campaign{{ID: "c1"}, {ID: "c2"}}
	campaignRepo := &listWACampaignStub{
		result: &shared.PaginatedResult[*wc.Campaign]{Items: campaigns},
	}
	entryRepo := &listWAEntryStub{
		counts: map[string]*wce.StatusCounts{
			"c1": {Total: 10, Sent: 4, Delivered: 3, Read: 1, Failed: 1, NotEligiblePossibleSpam: 1},
		},
	}

	uc := NewListCampaignsUseCase(campaignRepo, entryRepo, nil)
	result, err := uc.Execute(wc.ListCampaignsInput{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if entryRepo.countCalls != 1 {
		t.Errorf("CountByStatusForCampaigns called %d times, want exactly 1", entryRepo.countCalls)
	}
	if len(entryRepo.gotIDs) != 2 {
		t.Fatalf("batched call received %d ids, want 2 (%v)", len(entryRepo.gotIDs), entryRepo.gotIDs)
	}

	c1 := result.Items[0]
	if c1.Metrics == nil {
		t.Fatal("c1 metrics is nil")
	}
	if got, want := c1.Metrics.Dispatches, int64(8); got != want {
		t.Errorf("c1 Dispatches = %d, want %d", got, want)
	}
	if got, want := c1.Metrics.TotalNumbers, int64(10); got != want {
		t.Errorf("c1 TotalNumbers = %d, want %d", got, want)
	}

	c2 := result.Items[1]
	if c2.Metrics == nil {
		t.Fatal("c2 metrics is nil; expected zeroed metrics")
	}
	if c2.Metrics.TotalNumbers != 0 || c2.Metrics.Dispatches != 0 {
		t.Errorf("c2 expected zeroed metrics, got %+v", c2.Metrics)
	}
}

func TestListCampaignsReadsRecentEntriesForThePageInOneCall(t *testing.T) {
	campaigns := []*wc.Campaign{{ID: "c1"}, {ID: "c2"}, {ID: "c3"}}
	campaignRepo := &listWACampaignStub{
		result: &shared.PaginatedResult[*wc.Campaign]{Items: campaigns},
	}
	entryRepo := &listWAEntryStub{
		recentByID: map[string][]wce.WhatsAppCampaignEntry{
			"c1": {{ID: "e2"}, {ID: "e1"}},
			"c2": {},
			"c3": {{ID: "e3"}},
		},
	}

	uc := NewListCampaignsUseCase(campaignRepo, entryRepo, nil)
	result, err := uc.Execute(wc.ListCampaignsInput{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if entryRepo.recentCallCount != 1 {
		t.Fatalf("recent entries read %d times, want exactly 1 for the page", entryRepo.recentCallCount)
	}
	if len(entryRepo.recentIDs) != 3 || entryRepo.recentIDs[0] != "c1" || entryRepo.recentIDs[2] != "c3" {
		t.Errorf("recent entries ids = %v, want the page ids", entryRepo.recentIDs)
	}
	if entryRepo.recentLimit != recentEntriesLimit {
		t.Errorf("recent entries limit = %d, want %d", entryRepo.recentLimit, recentEntriesLimit)
	}
	if got := result.Items[0].RecentEntries; len(got) != 2 || got[0].ID != "e2" || got[1].ID != "e1" {
		t.Errorf("c1 recent entries = %+v, want e2, e1", got)
	}
	if got := result.Items[1].RecentEntries; got == nil || len(got) != 0 {
		t.Errorf("c2 recent entries = %#v, want an empty list", got)
	}
	if got := result.Items[2].RecentEntries; len(got) != 1 || got[0].ID != "e3" {
		t.Errorf("c3 recent entries = %+v, want e3", got)
	}
}

func TestListCampaignsEmptyResultSkipsAggregation(t *testing.T) {
	campaignRepo := &listWACampaignStub{
		result: &shared.PaginatedResult[*wc.Campaign]{Items: nil},
	}
	entryRepo := &listWAEntryStub{}

	uc := NewListCampaignsUseCase(campaignRepo, entryRepo, nil)
	if _, err := uc.Execute(wc.ListCampaignsInput{}); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if entryRepo.countCalls != 0 {
		t.Errorf("aggregation should be skipped for an empty page, but it ran %d time(s)", entryRepo.countCalls)
	}
}

func TestListCampaignsPropagatesErrors(t *testing.T) {
	sentinel := errors.New("boom")
	items := []*wc.Campaign{{ID: "c1"}}

	t.Run("list error", func(t *testing.T) {
		uc := NewListCampaignsUseCase(&listWACampaignStub{listErr: sentinel}, &listWAEntryStub{}, nil)
		if _, err := uc.Execute(wc.ListCampaignsInput{}); !errors.Is(err, sentinel) {
			t.Fatalf("expected list error to propagate, got %v", err)
		}
	})

	t.Run("aggregation error", func(t *testing.T) {
		campaignRepo := &listWACampaignStub{result: &shared.PaginatedResult[*wc.Campaign]{Items: items}}
		uc := NewListCampaignsUseCase(campaignRepo, &listWAEntryStub{countErr: sentinel}, nil)
		if _, err := uc.Execute(wc.ListCampaignsInput{}); !errors.Is(err, sentinel) {
			t.Fatalf("expected aggregation error to propagate, got %v", err)
		}
	})

	t.Run("recent entries error", func(t *testing.T) {
		campaignRepo := &listWACampaignStub{result: &shared.PaginatedResult[*wc.Campaign]{Items: items}}
		uc := NewListCampaignsUseCase(campaignRepo, &listWAEntryStub{recentErr: sentinel}, nil)
		if _, err := uc.Execute(wc.ListCampaignsInput{}); !errors.Is(err, sentinel) {
			t.Fatalf("expected recent-entries error to propagate, got %v", err)
		}
	})
}
