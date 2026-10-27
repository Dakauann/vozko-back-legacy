package unofficial_whatsapp_campaign

import (
	"errors"
	"testing"

	"vozko/domain/campaign"
	uwc "vozko/domain/unofficial_whatsapp_campaign"
)

type tallyStub struct {
	tally *campaign.SendsTally
	err   error
}

func (s tallyStub) TallySends(uwc.WorkspaceSummaryFilter) (*campaign.SendsTally, error) {
	return s.tally, s.err
}

func TestUnofficialSummaryReportsWhatThePeriodLeftOut(t *testing.T) {
	got, err := NewGetSummaryUseCase(tallyStub{tally: &campaign.SendsTally{
		Campaigns: campaign.Counts{Total: 3, Read: 2, Failed: 1},
		Undated:   4,
	}}).Execute(uwc.WorkspaceSummaryFilter{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got.Dispatches != 2 || got.BySource.Campaigns != 2 || got.BySource.OneOff != 0 || got.Undated != 4 {
		t.Fatalf("summary = %+v", got)
	}
}

func TestUnofficialSummaryFailsWithoutATally(t *testing.T) {
	for _, stub := range []tallyStub{{err: errors.New("db down")}, {}} {
		if _, err := NewGetSummaryUseCase(stub).Execute(uwc.WorkspaceSummaryFilter{WorkspaceID: "ws-1"}); err == nil {
			t.Fatalf("%+v: a summary without sends must fail, never show zeros", stub)
		}
	}
}
