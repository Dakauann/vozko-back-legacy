package campaign

import (
	"testing"
	"time"
)

func hourAt(hour int) *time.Time {
	t := time.Date(2026, 10, 20, hour, 0, 0, 0, time.UTC)
	return &t
}

func TestEntryMomentPlacesEveryOutcomeWhereItHappened(t *testing.T) {
	joined := *hourAt(1)
	cases := []struct {
		name   string
		entry  EntryOutcome
		want   *time.Time
		placed bool
	}{
		{"a send is placed when it left", EntryOutcome{Status: SendStatusRead, SentAt: hourAt(3), CreatedAt: joined}, hourAt(3), true},
		{"a send failure is placed when it failed", EntryOutcome{Status: SendStatusFailed, FailedAt: hourAt(4), CreatedAt: joined}, hourAt(4), true},
		{"a send that failed later keeps the moment it left", EntryOutcome{Status: SendStatusFailed, SentAt: hourAt(3), FailedAt: hourAt(5), CreatedAt: joined}, hourAt(3), true},
		{"a spam hold is placed when the lead joined", EntryOutcome{Status: SendStatusNotEligiblePossibleSpam, CreatedAt: joined}, &joined, true},
		{"a number off WhatsApp is placed when the lead joined", EntryOutcome{Status: SendStatusSkippedNotOnWhatsApp, CreatedAt: joined}, &joined, true},
		{"a refusal decided before sending is placed when the lead joined", EntryOutcome{Status: SendStatusFailed, ErrorCode: SkipOptedOut.FailureCode(), CreatedAt: joined}, &joined, true},
		{"a refusal decided at the queue keeps its failure stamp", EntryOutcome{Status: SendStatusFailed, ErrorCode: SkipBlocked.FailureCode(), FailedAt: hourAt(6), CreatedAt: joined}, hourAt(6), true},
		{"a send older than the stamps has no moment", EntryOutcome{Status: SendStatusDelivered, CreatedAt: joined}, nil, false},
		{"a send failure older than the stamps has no moment", EntryOutcome{Status: SendStatusFailed, ErrorCode: 131026, CreatedAt: joined}, nil, false},
		{"a queued lead has not happened yet", EntryOutcome{Status: SendStatusPending, CreatedAt: joined}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, placed := EntryMoment(tc.entry)
			if placed != tc.placed {
				t.Fatalf("placed = %v, want %v", placed, tc.placed)
			}
			if tc.want != nil && !got.Equal(*tc.want) {
				t.Fatalf("moment = %v, want %v", got, *tc.want)
			}
		})
	}
}

func TestEntryUndatedOnlyForAnOutcomeWithoutMoment(t *testing.T) {
	joined := *hourAt(1)
	if !EntryUndated(EntryOutcome{Status: SendStatusSent, CreatedAt: joined}) {
		t.Fatal("a send without a stamp must be reported as undated, never dropped silently")
	}
	if EntryUndated(EntryOutcome{Status: SendStatusPending, CreatedAt: joined}) {
		t.Fatal("a queued lead is not an undated send, it is no send at all")
	}
	if EntryUndated(EntryOutcome{Status: SendStatusSent, SentAt: hourAt(2), CreatedAt: joined}) {
		t.Fatal("a stamped send is dated")
	}
}

func TestSendWindowContains(t *testing.T) {
	w := SendWindow{From: hourAt(2), To: hourAt(4)}
	if !w.Bounded() || (SendWindow{}).Bounded() {
		t.Fatal("a window with any bound is bounded, an empty one is the whole history")
	}
	for hour, want := range map[int]bool{1: false, 2: true, 3: true, 4: true, 5: false} {
		if got := w.Contains(*hourAt(hour)); got != want {
			t.Errorf("Contains(%d:00) = %v, want %v", hour, got, want)
		}
	}
	if !(SendWindow{From: hourAt(2)}).Contains(*hourAt(9)) {
		t.Fatal("an open end must not cut sends off")
	}
}

func TestNewSendsSummaryAddsBothSourcesIntoOneSetOfTiles(t *testing.T) {
	tally := SendsTally{
		Campaigns:  Counts{Total: 10, Pending: 2, Sent: 1, Delivered: 3, Read: 2, Failed: 1, NotEligiblePossibleSpam: 1},
		OneOff:     Counts{Total: 4, Sent: 1, Delivered: 1, Read: 1, Failed: 1},
		ByCategory: CategoryDispatches{Marketing: 5, Utility: 4},
		Undated:    7,
	}
	s := NewSendsSummary(tally)

	if s.TotalNumbers != 14 || s.Delivered != 4 || s.Read != 3 || s.Sent != 2 || s.Failed != 2 || s.NotEligiblePossibleSpam != 1 {
		t.Fatalf("tiles must add both sources: %+v", s.Metrics)
	}
	if s.Dispatches != 9 {
		t.Fatalf("dispatches = %d, want 9 (6 from campaigns + 3 one-off)", s.Dispatches)
	}
	if s.BySource.Campaigns != 6 || s.BySource.OneOff != 3 {
		t.Fatalf("by source = %+v, want 6 campaigns and 3 one-off", s.BySource)
	}
	if s.ByCategory == nil || s.ByCategory.Marketing != 5 || s.ByCategory.Utility != 4 {
		t.Fatalf("categories lost: %+v", s.ByCategory)
	}
	if s.Undated != 7 {
		t.Fatalf("undated = %d, want 7", s.Undated)
	}
}

func TestNewSendsSummaryOmitsEmptyCategories(t *testing.T) {
	if s := NewSendsSummary(SendsTally{}); s.ByCategory != nil {
		t.Fatalf("no sends means no category breakdown, got %+v", s.ByCategory)
	}
}

func TestCategoryDispatchesAddNormalisesTheMetaCategory(t *testing.T) {
	var c CategoryDispatches
	c.Add(" marketing ", 2)
	c.Add("UTILITY", 3)
	c.Add("Authentication", 1)
	c.Add("UNKNOWN", 9)
	c.Add("", 9)
	if c != (CategoryDispatches{Marketing: 2, Utility: 3, Authentication: 1}) {
		t.Fatalf("got %+v", c)
	}
}

func TestSendsTallyAddCountsEveryStatusAndOnlySendsByCategory(t *testing.T) {
	var tally SendsTally
	tally.Add(SendSourceCampaign, SendStatusDelivered, "MARKETING", 3)
	tally.Add(SendSourceCampaign, SendStatusFailed, "MARKETING", 2)
	tally.Add(SendSourceCampaign, SendStatusNotEligiblePossibleSpam, "MARKETING", 1)
	tally.Add(SendSourceCampaign, SendStatusPending, "MARKETING", 4)
	tally.Add(SendSourceOneOff, SendStatusRead, "utility", 5)
	tally.Add(SendSourceOneOff, SendStatusSent, "AUTHENTICATION", 1)

	if tally.Campaigns != (Counts{Total: 10, Delivered: 3, Failed: 2, NotEligiblePossibleSpam: 1, Pending: 4}) {
		t.Fatalf("campaigns = %+v", tally.Campaigns)
	}
	if tally.OneOff != (Counts{Total: 6, Read: 5, Sent: 1}) {
		t.Fatalf("one-off = %+v", tally.OneOff)
	}
	if tally.ByCategory != (CategoryDispatches{Marketing: 3, Utility: 5, Authentication: 1}) {
		t.Fatalf("a failure, a spam hold or a queued lead is no send of its category: %+v", tally.ByCategory)
	}
}

func TestCountsAddKnowsEveryStatus(t *testing.T) {
	var c Counts
	for _, s := range []SendStatus{SendStatusPending, SendStatusSent, SendStatusDelivered, SendStatusRead,
		SendStatusFailed, SendStatusNotEligiblePossibleSpam, SendStatusSkippedNotOnWhatsApp} {
		c.Add(s, 1)
	}
	c.Add(SendStatus("SOMETHING_ELSE"), 1)
	want := Counts{Total: 8, Pending: 1, Sent: 1, Delivered: 1, Read: 1, Failed: 1, NotEligiblePossibleSpam: 1, SkippedNotOnWhatsApp: 1}
	if c != want {
		t.Fatalf("got %+v, want %+v", c, want)
	}
}

func TestSummarizeSendsRefusesAMissingTally(t *testing.T) {
	if _, err := SummarizeSends(nil); err != ErrSendsTallyMissing {
		t.Fatalf("err = %v, want ErrSendsTallyMissing so a missing read never shows zeros", err)
	}
	got, err := SummarizeSends(&SendsTally{OneOff: Counts{Total: 1, Read: 1}})
	if err != nil || got.Dispatches != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
}
