package whatsapp_campaign_usecase

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"vozko/domain/balance"
	wc "vozko/domain/whatsapp_campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
)

type waSummaryAggStub struct {
	counts    *wce.StatusCounts
	byCat     map[string]int64
	err       error
	catErr    error
	gotFilter wce.WorkspaceSummaryFilter
	catCalls  int
	onCount   func()
}

func (s *waSummaryAggStub) CountByStatusForWorkspace(f wce.WorkspaceSummaryFilter) (*wce.StatusCounts, error) {
	if s.onCount != nil {
		s.onCount()
	}
	s.gotFilter = f
	return s.counts, s.err
}

func (s *waSummaryAggStub) CountDispatchesByCategoryForWorkspace(f wce.WorkspaceSummaryFilter) (map[string]int64, error) {
	s.catCalls++
	if s.catErr != nil {
		return nil, s.catErr
	}
	return s.byCat, nil
}

type waChargeAggStub struct {
	stats    *balance.WhatsAppChargeStats
	err      error
	got      balance.WhatsAppChargeFilter
	onCharge func()
}

func (s *waChargeAggStub) AggregateWhatsAppTemplateCharges(f balance.WhatsAppChargeFilter) (*balance.WhatsAppChargeStats, error) {
	if s.onCharge != nil {
		s.onCharge()
	}
	s.got = f
	return s.stats, s.err
}

func TestGetSummaryBuildsBilledMetricsAndPassesFilter(t *testing.T) {
	agg := &waSummaryAggStub{
		counts: &wce.StatusCounts{
			Total: 50, Pending: 5, Sent: 30, Delivered: 5, Read: 2, Failed: 4, NotEligiblePossibleSpam: 4,
		},
		byCat: map[string]int64{
			"MARKETING":      20,
			"UTILITY":        15,
			"AUTHENTICATION": 2,
		},
	}
	uc := NewGetSummaryUseCase(agg, nil)

	filter := wce.WorkspaceSummaryFilter{WorkspaceID: "ws-1", Type: "standard"}
	m, err := uc.Execute(filter)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if got, want := m.Dispatches, int64(37); got != want {
		t.Errorf("Dispatches = %d, want %d", got, want)
	}
	if m.ByCategory == nil {
		t.Fatal("expected ByCategory volume split")
	}
	if got, want := m.ByCategory.Marketing, int64(20); got != want {
		t.Errorf("Marketing = %d, want %d", got, want)
	}
	if agg.gotFilter.WorkspaceID != "ws-1" || agg.gotFilter.Type != "standard" {
		t.Errorf("filter not passed through: %+v", agg.gotFilter)
	}
}

func TestGetSummaryPrefersLedgerCharges(t *testing.T) {
	agg := &waSummaryAggStub{
		counts: &wce.StatusCounts{
			Total: 10, Pending: 8, Sent: 1, Delivered: 0, Read: 1, Failed: 0,
		},
	}
	charges := &waChargeAggStub{
		stats: &balance.WhatsAppChargeStats{
			NetDispatches:  99,
			Marketing:      70,
			Utility:        25,
			Authentication: 4,
		},
	}
	uc := NewGetSummaryUseCase(agg, charges)

	m, err := uc.Execute(wce.WorkspaceSummaryFilter{WorkspaceID: "ws-1", Type: "standard"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got, want := m.Dispatches, int64(99); got != want {
		t.Errorf("Dispatches from ledger = %d, want %d", got, want)
	}
	if m.ByCategory == nil || m.ByCategory.Marketing != 70 || m.ByCategory.Utility != 25 {
		t.Errorf("ByCategory from ledger unexpected: %+v", m.ByCategory)
	}
	if got, want := m.Read, int64(1); got != want {
		t.Errorf("Read should stay entry-based, got %d", got)
	}
	if charges.got.WorkspaceID != "ws-1" || charges.got.CampaignType != "standard" {
		t.Errorf("charge filter not passed: %+v", charges.got)
	}
}

func TestGetSummaryPropagatesError(t *testing.T) {
	sentinel := errors.New("boom")
	uc := NewGetSummaryUseCase(&waSummaryAggStub{err: sentinel}, nil)
	if _, err := uc.Execute(wce.WorkspaceSummaryFilter{}); !errors.Is(err, sentinel) {
		t.Fatalf("expected error to propagate, got %v", err)
	}
}

func TestGetSummaryCategoryErrorFailsTheSummary(t *testing.T) {
	sentinel := errors.New("category join failed")
	agg := &waSummaryAggStub{
		counts: &wce.StatusCounts{Total: 10, Sent: 10},
		catErr: sentinel,
	}
	uc := NewGetSummaryUseCase(agg, nil)
	m, err := uc.Execute(wce.WorkspaceSummaryFilter{WorkspaceID: "ws-1"})
	if !errors.Is(err, sentinel) || m != nil {
		t.Fatalf("a category error must fail the summary, got %+v, %v", m, err)
	}
}

func TestGetSummaryLedgerErrorFailsTheSummary(t *testing.T) {
	sentinel := errors.New("ledger down")
	agg := &waSummaryAggStub{
		counts: &wce.StatusCounts{Total: 5, Sent: 5},
		byCat:  map[string]int64{"UTILITY": 5},
	}
	uc := NewGetSummaryUseCase(agg, &waChargeAggStub{err: sentinel})

	m, err := uc.Execute(wce.WorkspaceSummaryFilter{WorkspaceID: "ws-1"})
	if !errors.Is(err, sentinel) || m != nil {
		t.Fatalf("a ledger error must fail the summary, got %+v, %v", m, err)
	}
	if agg.catCalls != 0 {
		t.Errorf("the entry-based split must not stand in for the ledger, it ran %d time(s)", agg.catCalls)
	}
}

func TestGetSummaryReadsEntriesAndLedgerInParallel(t *testing.T) {
	countStarted, chargeStarted := make(chan struct{}), make(chan struct{})
	waitFor := func(other chan struct{}) {
		select {
		case <-other:
		case <-time.After(5 * time.Second):
		}
	}
	agg := &waSummaryAggStub{
		counts:  &wce.StatusCounts{Total: 3, Sent: 3},
		onCount: func() { close(countStarted); waitFor(chargeStarted) },
	}
	charges := &waChargeAggStub{
		stats:    &balance.WhatsAppChargeStats{NetDispatches: 3, Utility: 3},
		onCharge: func() { close(chargeStarted); waitFor(countStarted) },
	}

	started := time.Now()
	m, err := NewGetSummaryUseCase(agg, charges).Execute(wce.WorkspaceSummaryFilter{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("the two reads ran one after the other (%v)", elapsed)
	}
	if m.Dispatches != 3 || m.ByCategory == nil || m.ByCategory.Utility != 3 {
		t.Fatalf("metrics = %+v", m)
	}
}

func TestGetSummaryMatchesTheSequentialReads(t *testing.T) {
	counts := &wce.StatusCounts{Total: 12, Pending: 2, Sent: 6, Delivered: 2, Read: 1, Failed: 1}
	from, to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 31, 23, 59, 59, 0, time.UTC)
	scoped := wce.WorkspaceSummaryFilter{WorkspaceID: "ws-1", DepartmentIDs: []string{"d1"}, Type: "standard", CreatedFrom: &from, CreatedTo: &to}

	cases := []struct {
		name    string
		filter  wce.WorkspaceSummaryFilter
		agg     waSummaryAggStub
		charges *waChargeAggStub
	}{
		{"no ledger", scoped, waSummaryAggStub{counts: counts, byCat: map[string]int64{"marketing ": 4, "UTILITY": 3, "OTHER": 9}}, nil},
		{"no workspace", wce.WorkspaceSummaryFilter{Type: "standard"}, waSummaryAggStub{counts: counts, byCat: map[string]int64{"AUTHENTICATION": 1}}, &waChargeAggStub{stats: &balance.WhatsAppChargeStats{NetDispatches: 50}}},
		{"blank workspace", wce.WorkspaceSummaryFilter{WorkspaceID: "  "}, waSummaryAggStub{counts: counts}, &waChargeAggStub{stats: &balance.WhatsAppChargeStats{NetDispatches: 50}}},
		{"no charges", scoped, waSummaryAggStub{counts: counts, byCat: map[string]int64{"UTILITY": 7}}, &waChargeAggStub{}},
		{"no charges and no categories", scoped, waSummaryAggStub{counts: counts}, &waChargeAggStub{}},
		{"no charges and only unknown categories", scoped, waSummaryAggStub{counts: counts, byCat: map[string]int64{"SERVICE": 7}}, &waChargeAggStub{}},
		{"no charges and a category error", scoped, waSummaryAggStub{counts: counts, catErr: errors.New("join failed")}, &waChargeAggStub{}},
		{"zero charges", scoped, waSummaryAggStub{counts: counts, byCat: map[string]int64{"UTILITY": 7}}, &waChargeAggStub{stats: &balance.WhatsAppChargeStats{}}},
		{"net dispatches only", scoped, waSummaryAggStub{counts: counts}, &waChargeAggStub{stats: &balance.WhatsAppChargeStats{NetDispatches: 4}}},
		{"charged categories", scoped, waSummaryAggStub{counts: counts}, &waChargeAggStub{stats: &balance.WhatsAppChargeStats{NetDispatches: 9, Marketing: 5, Utility: 3, Authentication: 1}}},
		{"no entries", scoped, waSummaryAggStub{}, &waChargeAggStub{stats: &balance.WhatsAppChargeStats{NetDispatches: 2, Marketing: 2}}},
		{"entries error", scoped, waSummaryAggStub{err: errors.New("entries down")}, &waChargeAggStub{stats: &balance.WhatsAppChargeStats{NetDispatches: 2}}},
		{"ledger error", scoped, waSummaryAggStub{counts: counts, byCat: map[string]int64{"UTILITY": 7}}, &waChargeAggStub{err: errors.New("ledger down")}},
		{"entries and ledger errors", scoped, waSummaryAggStub{err: errors.New("entries down")}, &waChargeAggStub{err: errors.New("ledger down")}},
		{"ledger error without a workspace", wce.WorkspaceSummaryFilter{}, waSummaryAggStub{counts: counts, byCat: map[string]int64{"UTILITY": 7}}, &waChargeAggStub{err: errors.New("ledger down")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantAgg, gotAgg := tc.agg, tc.agg
			var wantCharges, gotCharges balance.WhatsAppChargeAggregator
			var wantStub, gotStub waChargeAggStub
			if tc.charges != nil {
				wantStub, gotStub = *tc.charges, *tc.charges
				wantCharges, gotCharges = &wantStub, &gotStub
			}

			want, wantErr := sequentialSummary(&wantAgg, wantCharges, tc.filter)
			got, gotErr := NewGetSummaryUseCase(&gotAgg, gotCharges).Execute(tc.filter)

			if (wantErr == nil) != (gotErr == nil) || (wantErr != nil && wantErr.Error() != gotErr.Error()) {
				t.Fatalf("error = %v, sequential %v", gotErr, wantErr)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("metrics = %+v (%+v), sequential %+v (%+v)", got, categoryOf(got), want, categoryOf(want))
			}
			sequentialReachedTheLedger := tc.agg.err == nil
			if !reflect.DeepEqual(gotAgg.gotFilter, wantAgg.gotFilter) || (sequentialReachedTheLedger && !reflect.DeepEqual(gotStub.got, wantStub.got)) {
				t.Fatalf("filters differ: entries %+v vs %+v, ledger %+v vs %+v", gotAgg.gotFilter, wantAgg.gotFilter, gotStub.got, wantStub.got)
			}
		})
	}
}

func categoryOf(m *wc.CampaignMetrics) *wc.CategoryDispatches {
	if m == nil {
		return nil
	}
	return m.ByCategory
}

func sequentialSummary(aggregator wce.SummaryAggregator, chargeAgg balance.WhatsAppChargeAggregator, filter wce.WorkspaceSummaryFilter) (*wc.CampaignMetrics, error) {
	counts, err := aggregator.CountByStatusForWorkspace(filter)
	if err != nil {
		return nil, err
	}
	metrics := wc.NewCampaignMetrics(counts)

	if chargeAgg != nil && strings.TrimSpace(filter.WorkspaceID) != "" {
		stats, chargeErr := chargeAgg.AggregateWhatsAppTemplateCharges(balance.WhatsAppChargeFilter{
			WorkspaceID:   filter.WorkspaceID,
			DepartmentIDs: filter.DepartmentIDs,
			CampaignType:  filter.Type,
			From:          filter.CreatedFrom,
			To:            filter.CreatedTo,
		})
		if chargeErr != nil {
			return nil, chargeErr
		}
		if stats != nil {
			metrics.Dispatches = stats.NetDispatches
			if stats.Marketing > 0 || stats.Utility > 0 || stats.Authentication > 0 || stats.NetDispatches > 0 {
				metrics.ByCategory = &wc.CategoryDispatches{
					Marketing:      stats.Marketing,
					Utility:        stats.Utility,
					Authentication: stats.Authentication,
				}
			}
			return metrics, nil
		}
	}

	byCat, err := aggregator.CountDispatchesByCategoryForWorkspace(filter)
	if err != nil {
		return nil, err
	}
	if len(byCat) == 0 {
		return metrics, nil
	}
	cd := &wc.CategoryDispatches{}
	for cat, n := range byCat {
		switch strings.ToUpper(strings.TrimSpace(cat)) {
		case "MARKETING":
			cd.Marketing += n
		case "UTILITY":
			cd.Utility += n
		case "AUTHENTICATION":
			cd.Authentication += n
		}
	}
	if cd.Marketing > 0 || cd.Utility > 0 || cd.Authentication > 0 {
		metrics.ByCategory = cd
	}
	return metrics, nil
}
