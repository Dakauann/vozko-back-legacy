package attendance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"vozko/domain/shared"
)

const SectionServiceMessages Section = "service_messages"

var (
	ErrServiceMessagesNotOnPlan       = errors.New("attendance: the workspace plan does not charge service messages")
	ErrServiceMessagePriceUnavailable = errors.New("attendance: the service message price of the workspace could not be resolved")
)

type ServiceMessageGranularity string

const (
	GranularityDay   ServiceMessageGranularity = "day"
	GranularityWeek  ServiceMessageGranularity = "week"
	GranularityMonth ServiceMessageGranularity = "month"
)

const (
	maxDailyServiceMessageDays  = 92
	maxWeeklyServiceMessageDays = 371
	monthBucketLayout           = "2006-01"
)

func AvailableSections(chargesServiceMessages bool) []Section {
	sections := Sections()
	if chargesServiceMessages {
		sections = append(sections, SectionServiceMessages)
	}
	return sections
}

type AvailableSectionsResponse struct {
	Sections []Section `json:"sections"`
}

func ChargesServiceMessages(priceMicros int64) bool {
	return priceMicros > 0
}

type ServiceMessageQuery struct {
	Filter      OverviewFilter
	Departments []string
}

func (q ServiceMessageQuery) Validate() error {
	if q.Filter.DateFrom == nil || q.Filter.DateTo == nil {
		return fmt.Errorf("%w: service messages need a start and an end day", ErrInvalidWindow)
	}
	if q.Filter.DateTo.Before(*q.Filter.DateFrom) {
		return fmt.Errorf("%w: the end is before the start", ErrInvalidWindow)
	}
	return nil
}

func (q ServiceMessageQuery) Applies() bool {
	channel := strings.TrimSpace(q.Filter.Channel)
	return channel == "" || channel == string(shared.EntryTypeWhatsApp)
}

func (q ServiceMessageQuery) days() int {
	from, to := dayOf(*q.Filter.DateFrom), dayOf(*q.Filter.DateTo)
	return int(to.Sub(from)/(24*time.Hour)) + 1
}

func (q ServiceMessageQuery) Granularity() ServiceMessageGranularity {
	switch days := q.days(); {
	case days <= maxDailyServiceMessageDays:
		return GranularityDay
	case days <= maxWeeklyServiceMessageDays:
		return GranularityWeek
	}
	return GranularityMonth
}

func (q ServiceMessageQuery) Fingerprint() string {
	departments := slices.Clone(q.Departments)
	slices.Sort(departments)
	parts := []string{
		fingerprintVersion,
		string(SectionServiceMessages),
		q.Filter.Fingerprint(SectionServiceMessages),
		strings.Join(departments, ","),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

type ServiceMessageCount struct {
	Bucket   time.Time `json:"bucket"`
	Messages int64     `json:"messages"`
}

type ServiceMessageReader interface {
	ReadServiceMessages(ctx context.Context, workspaceID string, query ServiceMessageQuery) ([]ServiceMessageCount, error)
}

type ServiceMessageBucket struct {
	Bucket   string `json:"bucket"`
	Messages int64  `json:"messages"`
}

type ServiceMessageSpend struct {
	UnitPriceBRL float64 `json:"unit_price_brl"`
	TotalBRL     float64 `json:"total_brl"`
}

type ServiceMessagesSection struct {
	Channel     string                    `json:"channel"`
	Applicable  bool                      `json:"applicable"`
	Total       *int64                    `json:"total"`
	Granularity ServiceMessageGranularity `json:"granularity,omitempty"`
	Buckets     []ServiceMessageBucket    `json:"buckets"`
	Spend       *ServiceMessageSpend      `json:"spend"`
}

func NotApplicableServiceMessages() ServiceMessagesSection {
	return ServiceMessagesSection{Channel: string(shared.EntryTypeWhatsApp), Buckets: []ServiceMessageBucket{}}
}

func BuildServiceMessages(q ServiceMessageQuery, counts []ServiceMessageCount) ServiceMessagesSection {
	granularity := q.Granularity()
	byBucket := make(map[string]int64, len(counts))
	for _, count := range counts {
		byBucket[bucketKey(granularity, bucketStart(granularity, count.Bucket))] += count.Messages
	}
	var total int64
	buckets := []ServiceMessageBucket{}
	last := bucketStart(granularity, *q.Filter.DateTo)
	for at := bucketStart(granularity, *q.Filter.DateFrom); !at.After(last); at = nextBucket(granularity, at) {
		key := bucketKey(granularity, at)
		buckets = append(buckets, ServiceMessageBucket{Bucket: key, Messages: byBucket[key]})
		total += byBucket[key]
	}
	return ServiceMessagesSection{
		Channel:     string(shared.EntryTypeWhatsApp),
		Applicable:  true,
		Total:       &total,
		Granularity: granularity,
		Buckets:     buckets,
	}
}

func (s *ServiceMessagesSection) PriceAt(priceMicros int64, usdToBRL float64) {
	if s.Total == nil || priceMicros <= 0 || usdToBRL <= 0 {
		s.Spend = nil
		return
	}
	s.Spend = &ServiceMessageSpend{
		UnitPriceBRL: shared.EstimateBRLFromMicros(priceMicros, usdToBRL),
		TotalBRL:     shared.EstimateBRLFromMicros(*s.Total*priceMicros, usdToBRL),
	}
}

func dayOf(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

func bucketStart(granularity ServiceMessageGranularity, t time.Time) time.Time {
	day := dayOf(t)
	switch granularity {
	case GranularityWeek:
		return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	case GranularityMonth:
		return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return day
}

func nextBucket(granularity ServiceMessageGranularity, t time.Time) time.Time {
	switch granularity {
	case GranularityWeek:
		return t.AddDate(0, 0, 7)
	case GranularityMonth:
		return t.AddDate(0, 1, 0)
	}
	return t.AddDate(0, 0, 1)
}

func bucketKey(granularity ServiceMessageGranularity, t time.Time) string {
	if granularity == GranularityMonth {
		return t.Format(monthBucketLayout)
	}
	return t.Format(DayLayout)
}

type ServiceMessageSectionsUseCase interface {
	AvailableSections(ctx context.Context, workspaceID string) []Section
	ServiceMessages(ctx context.Context, workspaceID string, query ServiceMessageQuery) (*ServiceMessagesSection, error)
}

type ServiceMessagesDigest struct {
	Channel     string                    `json:"channel"`
	Applicable  bool                      `json:"applicable"`
	Total       *int64                    `json:"total"`
	Granularity ServiceMessageGranularity `json:"granularity,omitempty"`
	Busiest     *ServiceMessageBucket     `json:"busiest,omitempty"`
	Spend       *ServiceMessageSpend      `json:"spend_estimate_brl"`
}

func DigestServiceMessages(s ServiceMessagesSection) ServiceMessagesDigest {
	digest := ServiceMessagesDigest{
		Channel:     s.Channel,
		Applicable:  s.Applicable,
		Total:       s.Total,
		Granularity: s.Granularity,
		Spend:       s.Spend,
	}
	for i := range s.Buckets {
		if s.Buckets[i].Messages > 0 && (digest.Busiest == nil || s.Buckets[i].Messages > digest.Busiest.Messages) {
			busiest := s.Buckets[i]
			digest.Busiest = &busiest
		}
	}
	return digest
}
