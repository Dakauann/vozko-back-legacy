package attendance

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"vozko/domain/shared"
)

func serviceMessageWindow(from, to string) ServiceMessageQuery {
	start, _ := ParseDay(from)
	end, _ := ParseDay(to)
	end = EndOfDay(end)
	return ServiceMessageQuery{Filter: OverviewFilter{DateFrom: &start, DateTo: &end}}
}

func TestOnlyAPositiveResolvedPriceChargesServiceMessages(t *testing.T) {
	for price, want := range map[int64]bool{-1: false, 0: false, 1: true, 45_000: true} {
		if got := ChargesServiceMessages(price); got != want {
			t.Fatalf("ChargesServiceMessages(%d) = %v, want %v", price, got, want)
		}
	}
}

func TestTheServiceMessageSectionIsOfferedOnlyToWorkspacesThatPayForIt(t *testing.T) {
	free := AvailableSections(false)
	if slices.Contains(free, SectionServiceMessages) {
		t.Fatalf("a workspace that does not pay for service messages is offered them: %v", free)
	}
	if !slices.Equal(free, Sections()) {
		t.Fatalf("a free workspace sees %v, want the ungated sections %v", free, Sections())
	}
	paying := AvailableSections(true)
	if !slices.Contains(paying, SectionServiceMessages) {
		t.Fatalf("a paying workspace is not offered service messages: %v", paying)
	}
}

func TestTheGatedSectionNeverPassesForAnUngatedOne(t *testing.T) {
	if slices.Contains(Sections(), SectionServiceMessages) {
		t.Fatalf("service messages must not be listed as an always available section")
	}
	if _, ok := ParseSection(string(SectionServiceMessages)); ok {
		t.Fatalf("the ungated section route accepts service messages, which would skip the plan gate")
	}
}

func TestServiceMessagesNeedAPeriod(t *testing.T) {
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	before := day.AddDate(0, 0, -1)
	for name, q := range map[string]ServiceMessageQuery{
		"no start":         {Filter: OverviewFilter{DateTo: &day}},
		"no end":           {Filter: OverviewFilter{DateFrom: &day}},
		"end before start": {Filter: OverviewFilter{DateFrom: &day, DateTo: &before}},
		"no period at all": {},
	} {
		if err := q.Validate(); !errors.Is(err, ErrInvalidWindow) {
			t.Fatalf("%s: Validate() = %v, want ErrInvalidWindow", name, err)
		}
	}
	if err := serviceMessageWindow("2026-10-01", "2026-10-01").Validate(); err != nil {
		t.Fatalf("a one day period is refused: %v", err)
	}
}

func TestServiceMessagesExistOnlyOnTheOfficialWhatsAppChannel(t *testing.T) {
	for channel, want := range map[string]bool{
		"":                               true,
		string(shared.EntryTypeWhatsApp): true,
		string(shared.EntryTypeUnofficialWhatsApp): false,
		string(shared.EntryTypeInstagram):          false,
	} {
		q := serviceMessageWindow("2026-10-01", "2026-10-07")
		q.Filter.Channel = channel
		if got := q.Applies(); got != want {
			t.Fatalf("channel %q: Applies() = %v, want %v", channel, got, want)
		}
	}
}

func TestTheTrendGetsCoarserAsThePeriodGrows(t *testing.T) {
	cases := []struct {
		from, to string
		want     ServiceMessageGranularity
	}{
		{"2026-10-01", "2026-10-01", GranularityDay},
		{"2026-08-01", "2026-10-31", GranularityDay},
		{"2026-07-31", "2026-10-31", GranularityWeek},
		{"2025-10-27", "2026-10-31", GranularityWeek},
		{"2025-10-25", "2026-10-31", GranularityMonth},
	}
	for _, tc := range cases {
		if got := serviceMessageWindow(tc.from, tc.to).Granularity(); got != tc.want {
			t.Fatalf("%s to %s: granularity = %s, want %s", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestEveryBucketOfThePeriodIsPresentAndCountsAddUp(t *testing.T) {
	q := serviceMessageWindow("2026-10-01", "2026-10-04")
	counts := []ServiceMessageCount{
		{Bucket: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Messages: 3},
		{Bucket: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), Messages: 5},
	}
	got := BuildServiceMessages(q, counts)

	if !got.Applicable || got.Channel != string(shared.EntryTypeWhatsApp) || got.Granularity != GranularityDay {
		t.Fatalf("section = %+v", got)
	}
	if got.Total == nil || *got.Total != 8 {
		t.Fatalf("total = %v, want 8", got.Total)
	}
	want := []ServiceMessageBucket{{"2026-10-01", 3}, {"2026-10-02", 0}, {"2026-10-03", 5}, {"2026-10-04", 0}}
	if !slices.Equal(got.Buckets, want) {
		t.Fatalf("buckets = %v, want %v", got.Buckets, want)
	}
}

func TestWeeklyBucketsStartOnMondayAndMonthlyOnTheFirst(t *testing.T) {
	weekly := BuildServiceMessages(serviceMessageWindow("2026-06-01", "2026-09-30"), []ServiceMessageCount{
		{Bucket: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC), Messages: 2},
	})
	if weekly.Granularity != GranularityWeek || weekly.Buckets[0].Bucket != "2026-06-01" || weekly.Buckets[len(weekly.Buckets)-1] != (ServiceMessageBucket{"2026-09-28", 2}) {
		t.Fatalf("weekly buckets = %v", weekly.Buckets)
	}
	monthly := BuildServiceMessages(serviceMessageWindow("2025-01-15", "2026-10-26"), nil)
	if monthly.Granularity != GranularityMonth || len(monthly.Buckets) != 22 || monthly.Buckets[0].Bucket != "2025-01" || monthly.Buckets[21].Bucket != "2026-10" {
		t.Fatalf("monthly buckets = %v", monthly.Buckets)
	}
}

func TestAQuietPeriodIsAZeroCountNotAMissingOne(t *testing.T) {
	got := BuildServiceMessages(serviceMessageWindow("2026-10-01", "2026-10-02"), nil)
	if got.Total == nil || *got.Total != 0 {
		t.Fatalf("a period without service messages reads %v, want a known 0", got.Total)
	}
}

func TestAnotherChannelLeavesTheCountUnknownRatherThanZero(t *testing.T) {
	got := NotApplicableServiceMessages()
	if got.Applicable || got.Total != nil || got.Spend != nil || got.Buckets == nil || len(got.Buckets) != 0 {
		t.Fatalf("not applicable section = %+v", got)
	}
	raw, _ := json.Marshal(got)
	var body map[string]json.RawMessage
	_ = json.Unmarshal(raw, &body)
	if string(body["total"]) != "null" || string(body["spend"]) != "null" {
		t.Fatalf("unknown values must travel as null: %s", raw)
	}
}

func TestSpendIsTheCountAtTheCustomerPriceInReais(t *testing.T) {
	section := BuildServiceMessages(serviceMessageWindow("2026-10-01", "2026-10-01"), []ServiceMessageCount{
		{Bucket: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Messages: 1000},
	})
	section.PriceAt(20_000, 5.5)
	if section.Spend == nil {
		t.Fatalf("spend missing with a price and a rate")
	}
	if section.Spend.UnitPriceBRL != shared.EstimateBRLFromMicros(20_000, 5.5) || section.Spend.TotalBRL != 110 {
		t.Fatalf("spend = %+v, want unit %v and total 110", section.Spend, shared.EstimateBRLFromMicros(20_000, 5.5))
	}
}

func TestSpendIsUnknownWithoutAnExchangeRate(t *testing.T) {
	section := BuildServiceMessages(serviceMessageWindow("2026-10-01", "2026-10-01"), nil)
	section.PriceAt(20_000, 0)
	if section.Spend != nil {
		t.Fatalf("spend = %+v without a rate, want unknown", section.Spend)
	}
	unknown := NotApplicableServiceMessages()
	unknown.PriceAt(20_000, 5.5)
	if unknown.Spend != nil {
		t.Fatalf("an unknown count was priced: %+v", unknown.Spend)
	}
}

func TestTheFingerprintSeparatesEveryScopeThatChangesTheCount(t *testing.T) {
	base := serviceMessageWindow("2026-10-01", "2026-10-07")
	variants := map[string]ServiceMessageQuery{}
	for name, mutate := range map[string]func(*ServiceMessageQuery){
		"department":   func(q *ServiceMessageQuery) { q.Filter.DepartmentID = "d1" },
		"member":       func(q *ServiceMessageQuery) { q.Filter.MemberID = "m1" },
		"campaign":     func(q *ServiceMessageQuery) { q.Filter.CampaignID = "c1" },
		"origin":       func(q *ServiceMessageQuery) { q.Filter.Origin = "receptive" },
		"channel":      func(q *ServiceMessageQuery) { q.Filter.Channel = "whatsapp" },
		"departments":  func(q *ServiceMessageQuery) { q.Departments = []string{"d1"} },
		"two depts":    func(q *ServiceMessageQuery) { q.Departments = []string{"d1", "d2"} },
		"later period": func(q *ServiceMessageQuery) { later := q.Filter.DateTo.AddDate(0, 0, 1); q.Filter.DateTo = &later },
	} {
		q := base
		q.Departments = slices.Clone(base.Departments)
		mutate(&q)
		variants[name] = q
	}
	seen := map[string]string{base.Fingerprint(): "base"}
	for name, q := range variants {
		fp := q.Fingerprint()
		if other, dup := seen[fp]; dup {
			t.Fatalf("%s and %s share a cache key", name, other)
		}
		seen[fp] = name
	}
	if base.Fingerprint() == base.Filter.Fingerprint(SectionSummary) {
		t.Fatalf("service messages share a key with the summary")
	}
}

func TestTheFingerprintIgnoresTheOrderOfTheDepartmentScope(t *testing.T) {
	a, b := serviceMessageWindow("2026-10-01", "2026-10-07"), serviceMessageWindow("2026-10-01", "2026-10-07")
	a.Departments = []string{"d1", "d2"}
	b.Departments = []string{"d2", "d1"}
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatalf("the same scope in another order missed the cache")
	}
	if b.Departments[0] != "d2" {
		t.Fatalf("Fingerprint reordered the caller's slice")
	}
}

func TestTheAssistantDigestCarriesTheTotalTheBusiestBucketAndTheSpend(t *testing.T) {
	section := BuildServiceMessages(serviceMessageWindow("2026-10-01", "2026-10-03"), []ServiceMessageCount{
		{Bucket: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Messages: 4},
		{Bucket: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), Messages: 9},
	})
	section.PriceAt(20_000, 5)
	digest := DigestServiceMessages(section)
	if digest.Total == nil || *digest.Total != 13 || digest.Busiest == nil || *digest.Busiest != (ServiceMessageBucket{"2026-10-02", 9}) {
		t.Fatalf("digest = %+v", digest)
	}
	if digest.Spend == nil || digest.Granularity != GranularityDay || digest.Channel != "whatsapp" || !digest.Applicable {
		t.Fatalf("digest = %+v", digest)
	}
}

func TestTheAssistantDigestOfAQuietOrUnknownPeriodHasNoBusiestBucket(t *testing.T) {
	quiet := DigestServiceMessages(BuildServiceMessages(serviceMessageWindow("2026-10-01", "2026-10-03"), nil))
	if quiet.Busiest != nil || quiet.Total == nil || *quiet.Total != 0 {
		t.Fatalf("quiet digest = %+v", quiet)
	}
	unknown := DigestServiceMessages(NotApplicableServiceMessages())
	if unknown.Total != nil || unknown.Busiest != nil || unknown.Applicable {
		t.Fatalf("unknown digest = %+v", unknown)
	}
}
