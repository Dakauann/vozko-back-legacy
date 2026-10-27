package whatsapp_campaign_usecase

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"vozko/domain/campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
)

type sendsTallyStub struct {
	tally     *campaign.SendsTally
	err       error
	gotFilter wce.WorkspaceSummaryFilter
}

func (s *sendsTallyStub) TallySends(f wce.WorkspaceSummaryFilter) (*campaign.SendsTally, error) {
	s.gotFilter = f
	return s.tally, s.err
}

func TestGetSummaryCountsCampaignAndOneOffSendsInTheSameTiles(t *testing.T) {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	stub := &sendsTallyStub{tally: &campaign.SendsTally{
		Campaigns:  campaign.Counts{Total: 6, Pending: 1, Delivered: 3, Failed: 1, NotEligiblePossibleSpam: 1},
		OneOff:     campaign.Counts{Total: 2, Read: 1, Sent: 1},
		ByCategory: campaign.CategoryDispatches{Marketing: 4, Utility: 1},
		Undated:    3,
	}}
	filter := wce.WorkspaceSummaryFilter{WorkspaceID: "ws-1", DepartmentIDs: []string{"dep-1"}, Window: campaign.SendWindow{From: &from}}

	got, err := NewGetSummaryUseCase(stub).Execute(filter)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if !reflect.DeepEqual(stub.gotFilter, filter) {
		t.Fatalf("filter = %+v, want %+v", stub.gotFilter, filter)
	}
	if got.Dispatches != 5 || got.BySource.Campaigns != 3 || got.BySource.OneOff != 2 {
		t.Fatalf("dispatches %d by source %+v, want 5 split 3 campaign and 2 one-off", got.Dispatches, got.BySource)
	}
	if got.Delivered != 3 || got.Read != 1 || got.Sent != 1 || got.Failed != 1 || got.NotEligiblePossibleSpam != 1 {
		t.Fatalf("tiles = %+v", got.Metrics)
	}
	if got.ByCategory == nil || got.ByCategory.Marketing != 4 || got.ByCategory.Utility != 1 {
		t.Fatalf("categories = %+v", got.ByCategory)
	}
	if got.Undated != 3 {
		t.Fatalf("undated = %d, want 3 so the page can say what the period left out", got.Undated)
	}
}

func TestGetSummaryFailsWhenTheSendsCannotBeRead(t *testing.T) {
	stub := &sendsTallyStub{err: errors.New("db down")}
	if _, err := NewGetSummaryUseCase(stub).Execute(wce.WorkspaceSummaryFilter{WorkspaceID: "ws-1"}); err == nil {
		t.Fatal("a failed read must fail the summary, never show zeros")
	}
}

func TestGetSummaryRefusesAMissingTally(t *testing.T) {
	if _, err := NewGetSummaryUseCase(&sendsTallyStub{}).Execute(wce.WorkspaceSummaryFilter{WorkspaceID: "ws-1"}); err == nil {
		t.Fatal("a nil tally is no answer, the summary must fail instead of showing zeros")
	}
}
