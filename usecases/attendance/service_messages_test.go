package attendance_usecase

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"vozko/domain/attendance"
	"vozko/domain/shared/deps"
	"vozko/domain/workspace/workspace_pricing"
)

type planPrices struct {
	mu     sync.Mutex
	prices map[string]int64
	err    error
}

func (p *planPrices) ServiceMessagePrice(workspaceID string) (workspace_pricing.PriceResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return workspace_pricing.PriceResult{}, p.err
	}
	return workspace_pricing.PriceResult{PriceMicros: p.prices[workspaceID], CostMicros: 1}, nil
}

func (p *planPrices) set(workspaceID string, price int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prices[workspaceID] = price
}

type countingReader struct {
	mu      sync.Mutex
	reads   []string
	queries []attendance.ServiceMessageQuery
	err     error
}

func (r *countingReader) ReadServiceMessages(_ context.Context, workspaceID string, q attendance.ServiceMessageQuery) ([]attendance.ServiceMessageCount, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads = append(r.reads, workspaceID)
	r.queries = append(r.queries, q)
	if r.err != nil {
		return nil, r.err
	}
	return []attendance.ServiceMessageCount{{Bucket: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), Messages: 40}}, nil
}

func (r *countingReader) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reads)
}

type fixedRate struct {
	rate float64
	err  error
}

func (f fixedRate) CurrentRate() (float64, error) { return f.rate, f.err }

type serviceMessageFixture struct {
	sections *ServiceMessageSections
	prices   *planPrices
	reader   *countingReader
	memo     *memoryMemo
	gate     *trackingGate
}

func newServiceMessageFixture(t *testing.T, rate fixedRate) serviceMessageFixture {
	t.Helper()
	f := serviceMessageFixture{
		prices: &planPrices{prices: map[string]int64{"paying": 20_000, "free": 0}},
		reader: &countingReader{},
		memo:   newMemoryMemo(),
		gate:   &trackingGate{},
	}
	sections, err := NewServiceMessageSections(ServiceMessagesDeps{
		Reader:  f.reader,
		Prices:  f.prices,
		Rates:   rate,
		Caching: SectionCaching{Memo: f.memo, Gate: f.gate, Versions: newMemoryVersions(), TTL: time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.sections = sections
	return f
}

func octoberWeek() attendance.ServiceMessageQuery {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := attendance.EndOfDay(time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC))
	return attendance.ServiceMessageQuery{Filter: attendance.OverviewFilter{DateFrom: &from, DateTo: &to}}
}

func (f serviceMessageFixture) memoKeys() int {
	f.memo.mu.Lock()
	defer f.memo.mu.Unlock()
	return len(f.memo.values)
}

func TestAPayingWorkspaceGetsItsServiceMessagesAndWhatTheyCost(t *testing.T) {
	f := newServiceMessageFixture(t, fixedRate{rate: 5})
	got, err := f.sections.ServiceMessages(context.Background(), "paying", octoberWeek())
	if err != nil {
		t.Fatalf("ServiceMessages() error = %v", err)
	}
	if got.Total == nil || *got.Total != 40 || !got.Applicable || len(got.Buckets) != 7 {
		t.Fatalf("section = %+v", got)
	}
	if got.Spend == nil || got.Spend.TotalBRL != 4 || got.Spend.UnitPriceBRL != 0.1 {
		t.Fatalf("spend = %+v, want 40 messages at R$ 0,10 = R$ 4,00", got.Spend)
	}
	if f.gate.acquisitions() != 1 {
		t.Fatalf("the read took %d analytics slots, want 1", f.gate.acquisitions())
	}
}

func TestAWorkspaceThatDoesNotPayGetsNothingAndNothingIsRead(t *testing.T) {
	f := newServiceMessageFixture(t, fixedRate{rate: 5})
	got, err := f.sections.ServiceMessages(context.Background(), "free", octoberWeek())
	if !errors.Is(err, attendance.ErrServiceMessagesNotOnPlan) || got != nil {
		t.Fatalf("got %+v, %v; want nothing and ErrServiceMessagesNotOnPlan", got, err)
	}
	if f.reader.count() != 0 || f.memoKeys() != 0 || f.gate.acquisitions() != 0 {
		t.Fatalf("a free workspace reached the data: reads %d, cache keys %d, slots %d", f.reader.count(), f.memoKeys(), f.gate.acquisitions())
	}
}

func TestAPriceThatCannotBeResolvedReturnsNothing(t *testing.T) {
	f := newServiceMessageFixture(t, fixedRate{rate: 5})
	f.prices.err = errors.New("plan lookup failed")
	got, err := f.sections.ServiceMessages(context.Background(), "paying", octoberWeek())
	if !errors.Is(err, attendance.ErrServiceMessagePriceUnavailable) || got != nil {
		t.Fatalf("got %+v, %v; want nothing and ErrServiceMessagePriceUnavailable", got, err)
	}
	if f.reader.count() != 0 || f.memoKeys() != 0 {
		t.Fatalf("an unresolved price still read %d times", f.reader.count())
	}
}

func TestABlankWorkspaceNeverPassesTheGate(t *testing.T) {
	f := newServiceMessageFixture(t, fixedRate{rate: 5})
	f.prices.set("", 20_000)
	if _, err := f.sections.ServiceMessages(context.Background(), " ", octoberWeek()); !errors.Is(err, attendance.ErrServiceMessagesNotOnPlan) {
		t.Fatalf("a blank workspace got %v", err)
	}
	if slices.Contains(f.sections.AvailableSections(context.Background(), ""), attendance.SectionServiceMessages) {
		t.Fatalf("a blank workspace was offered service messages")
	}
}

func TestACachedCountNeverReachesAWorkspaceThatFailsTheGate(t *testing.T) {
	f := newServiceMessageFixture(t, fixedRate{rate: 5})
	ctx := context.Background()
	if _, err := f.sections.ServiceMessages(ctx, "paying", octoberWeek()); err != nil {
		t.Fatal(err)
	}
	if got, err := f.sections.ServiceMessages(ctx, "free", octoberWeek()); !errors.Is(err, attendance.ErrServiceMessagesNotOnPlan) || got != nil {
		t.Fatalf("the free workspace got %+v, %v after the paying one warmed the cache", got, err)
	}
	f.prices.set("paying", 0)
	if got, err := f.sections.ServiceMessages(ctx, "paying", octoberWeek()); !errors.Is(err, attendance.ErrServiceMessagesNotOnPlan) || got != nil {
		t.Fatalf("a workspace that stopped paying still got its cached count: %+v, %v", got, err)
	}
	if f.reader.count() != 1 {
		t.Fatalf("reads = %d, want only the paying workspace's first read", f.reader.count())
	}
}

func TestTheCountIsCachedButThePriceIsAppliedFresh(t *testing.T) {
	f := newServiceMessageFixture(t, fixedRate{rate: 5})
	ctx := context.Background()
	if _, err := f.sections.ServiceMessages(ctx, "paying", octoberWeek()); err != nil {
		t.Fatal(err)
	}
	f.prices.set("paying", 40_000)
	got, err := f.sections.ServiceMessages(ctx, "paying", octoberWeek())
	if err != nil {
		t.Fatal(err)
	}
	if f.reader.count() != 1 {
		t.Fatalf("the second request read again: %d reads", f.reader.count())
	}
	if got.Spend == nil || got.Spend.TotalBRL != 8 {
		t.Fatalf("spend = %+v, want the new price applied to the cached count", got.Spend)
	}
}

func TestEveryScopeIsReadAndCachedOnItsOwn(t *testing.T) {
	f := newServiceMessageFixture(t, fixedRate{rate: 5})
	ctx := context.Background()
	whole := octoberWeek()
	scoped := octoberWeek()
	scoped.Departments = []string{"d1"}
	scoped.Filter.MemberID = "m1"
	for _, q := range []attendance.ServiceMessageQuery{whole, scoped, whole, scoped} {
		if _, err := f.sections.ServiceMessages(ctx, "paying", q); err != nil {
			t.Fatal(err)
		}
	}
	if f.reader.count() != 2 {
		t.Fatalf("reads = %d, want one per scope", f.reader.count())
	}
	last := f.reader.queries[1]
	if !slices.Equal(last.Departments, []string{"d1"}) || last.Filter.MemberID != "m1" {
		t.Fatalf("the scope did not reach the reader: %+v", last)
	}
}

func TestAnotherChannelAnswersNotApplicableWithoutReading(t *testing.T) {
	f := newServiceMessageFixture(t, fixedRate{rate: 5})
	q := octoberWeek()
	q.Filter.Channel = "instagram"
	got, err := f.sections.ServiceMessages(context.Background(), "paying", q)
	if err != nil || got.Applicable || got.Total != nil {
		t.Fatalf("got %+v, %v; want a not applicable section", got, err)
	}
	if f.reader.count() != 0 {
		t.Fatalf("another channel was read")
	}
}

func TestAnInvalidPeriodIsRefusedBeforeAnyRead(t *testing.T) {
	f := newServiceMessageFixture(t, fixedRate{rate: 5})
	if _, err := f.sections.ServiceMessages(context.Background(), "paying", attendance.ServiceMessageQuery{}); !errors.Is(err, attendance.ErrInvalidWindow) {
		t.Fatalf("err = %v, want ErrInvalidWindow", err)
	}
	if f.reader.count() != 0 {
		t.Fatalf("an invalid period was read")
	}
}

func TestAReadFailureIsReturnedAndNotCached(t *testing.T) {
	f := newServiceMessageFixture(t, fixedRate{rate: 5})
	f.reader.err = errors.New("timeout")
	if _, err := f.sections.ServiceMessages(context.Background(), "paying", octoberWeek()); err == nil {
		t.Fatalf("a failed read came back as data")
	}
	if f.memoKeys() != 0 {
		t.Fatalf("a failed read was cached")
	}
}

func TestWithoutAnExchangeRateTheCostIsUnknownButTheCountStays(t *testing.T) {
	f := newServiceMessageFixture(t, fixedRate{err: errors.New("no rate")})
	got, err := f.sections.ServiceMessages(context.Background(), "paying", octoberWeek())
	if err != nil || got.Total == nil || *got.Total != 40 || got.Spend != nil {
		t.Fatalf("got %+v, %v; want the count and an unknown cost", got, err)
	}
}

func TestTheAvailableSectionsFollowTheResolvedPrice(t *testing.T) {
	f := newServiceMessageFixture(t, fixedRate{rate: 5})
	ctx := context.Background()
	if !slices.Contains(f.sections.AvailableSections(ctx, "paying"), attendance.SectionServiceMessages) {
		t.Fatalf("a paying workspace is not offered service messages")
	}
	if slices.Contains(f.sections.AvailableSections(ctx, "free"), attendance.SectionServiceMessages) {
		t.Fatalf("a free workspace is offered service messages")
	}
	f.prices.err = errors.New("plan lookup failed")
	if got := f.sections.AvailableSections(ctx, "paying"); slices.Contains(got, attendance.SectionServiceMessages) || !slices.Equal(got, attendance.Sections()) {
		t.Fatalf("an unresolved price offered %v, want only the ungated sections", got)
	}
	if f.reader.count() != 0 {
		t.Fatalf("listing the sections read data")
	}
}

func TestServiceMessageSectionsRefuseMissingDependencies(t *testing.T) {
	sections, err := NewServiceMessageSections(ServiceMessagesDeps{})
	var missing *deps.MissingError
	if sections != nil || !errors.As(err, &missing) {
		t.Fatalf("sections = %v err = %v", sections, err)
	}
	for _, name := range []string{"service message reader", "service message prices", "exchange rate", "section memo", "analytics gate", "section versions"} {
		if !slices.Contains(missing.Missing, name) {
			t.Errorf("%s is not reported missing: %v", name, missing.Missing)
		}
	}
	_, err = NewServiceMessageSections(ServiceMessagesDeps{
		Reader: &countingReader{}, Prices: &planPrices{}, Rates: fixedRate{},
		Caching: SectionCaching{Memo: newMemoryMemo(), Gate: &trackingGate{}, Versions: newMemoryVersions()},
	})
	if !errors.Is(err, ErrSectionTTLRequired) {
		t.Fatalf("a zero TTL was accepted: %v", err)
	}
}
