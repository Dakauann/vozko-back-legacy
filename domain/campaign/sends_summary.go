package campaign

import (
	"errors"
	"slices"
	"strings"
	"time"
)

type SendWindow struct {
	From *time.Time
	To   *time.Time
}

func (w SendWindow) Bounded() bool {
	return w.From != nil || w.To != nil
}

func (w SendWindow) Contains(moment time.Time) bool {
	if w.From != nil && moment.Before(*w.From) {
		return false
	}
	if w.To != nil && moment.After(*w.To) {
		return false
	}
	return true
}

func OutcomeMilestones() []Milestone {
	return []Milestone{MilestoneSent, MilestoneFailed}
}

func DecidedWithoutSending() []SendStatus {
	return []SendStatus{SendStatusNotEligiblePossibleSpam, SendStatusSkippedNotOnWhatsApp}
}

type EntryOutcome struct {
	Status    SendStatus
	ErrorCode int
	SentAt    *time.Time
	FailedAt  *time.Time
	CreatedAt time.Time
}

func (e EntryOutcome) stamp(m Milestone) *time.Time {
	switch m {
	case MilestoneSent:
		return e.SentAt
	case MilestoneFailed:
		return e.FailedAt
	default:
		return nil
	}
}

func (e EntryOutcome) decidedBeforeSending() bool {
	if slices.Contains(DecidedWithoutSending(), e.Status) {
		return true
	}
	return e.Status == SendStatusFailed && slices.Contains(SkipFailureCodes(), e.ErrorCode)
}

func EntryMoment(e EntryOutcome) (time.Time, bool) {
	var first *time.Time
	for _, m := range OutcomeMilestones() {
		if stamped := e.stamp(m); stamped != nil && (first == nil || stamped.Before(*first)) {
			first = stamped
		}
	}
	if first != nil {
		return *first, true
	}
	if e.decidedBeforeSending() {
		return e.CreatedAt, true
	}
	return time.Time{}, false
}

func EntryUndated(e EntryOutcome) bool {
	if e.Status == SendStatusPending {
		return false
	}
	_, placed := EntryMoment(e)
	return !placed
}

func (c *CategoryDispatches) Add(category string, n int64) {
	switch strings.ToUpper(strings.TrimSpace(category)) {
	case "MARKETING":
		c.Marketing += n
	case "UTILITY":
		c.Utility += n
	case "AUTHENTICATION":
		c.Authentication += n
	}
}

func (c CategoryDispatches) empty() bool {
	return c.Marketing == 0 && c.Utility == 0 && c.Authentication == 0
}

func (c Counts) Plus(o Counts) Counts {
	return Counts{
		Total:                   c.Total + o.Total,
		Pending:                 c.Pending + o.Pending,
		Sent:                    c.Sent + o.Sent,
		Delivered:               c.Delivered + o.Delivered,
		Read:                    c.Read + o.Read,
		Failed:                  c.Failed + o.Failed,
		NotEligiblePossibleSpam: c.NotEligiblePossibleSpam + o.NotEligiblePossibleSpam,
		SkippedNotOnWhatsApp:    c.SkippedNotOnWhatsApp + o.SkippedNotOnWhatsApp,
	}
}

type SendsTally struct {
	Campaigns  Counts
	OneOff     Counts
	ByCategory CategoryDispatches
	Undated    int64
}

type SourceDispatches struct {
	Campaigns int64 `json:"campaigns"`
	OneOff    int64 `json:"oneOff"`
}

type SendsSummary struct {
	Metrics
	BySource SourceDispatches `json:"bySource"`
	Undated  int64            `json:"undated"`
}

func NewSendsSummary(t SendsTally) *SendsSummary {
	all := t.Campaigns.Plus(t.OneOff)
	summary := &SendsSummary{
		Metrics: *NewMetrics(&all),
		BySource: SourceDispatches{
			Campaigns: t.Campaigns.Dispatches(),
			OneOff:    t.OneOff.Dispatches(),
		},
		Undated: t.Undated,
	}
	if !t.ByCategory.empty() {
		categories := t.ByCategory
		summary.ByCategory = &categories
	}
	return summary
}

type SendSource string

const (
	SendSourceCampaign SendSource = "campaign"
	SendSourceOneOff   SendSource = "one_off"
)

func (c *Counts) Add(status SendStatus, n int64) {
	c.Total += n
	switch status {
	case SendStatusPending:
		c.Pending += n
	case SendStatusSent:
		c.Sent += n
	case SendStatusDelivered:
		c.Delivered += n
	case SendStatusRead:
		c.Read += n
	case SendStatusFailed:
		c.Failed += n
	case SendStatusNotEligiblePossibleSpam:
		c.NotEligiblePossibleSpam += n
	case SendStatusSkippedNotOnWhatsApp:
		c.SkippedNotOnWhatsApp += n
	}
}

func (t *SendsTally) Add(source SendSource, status SendStatus, category string, n int64) {
	counts := &t.Campaigns
	if source == SendSourceOneOff {
		counts = &t.OneOff
	}
	counts.Add(status, n)
	if slices.Contains(status.Milestones(), MilestoneSent) {
		t.ByCategory.Add(category, n)
	}
}

var ErrSendsTallyMissing = errors.New("campaign: the sends could not be tallied")

func SummarizeSends(t *SendsTally) (*SendsSummary, error) {
	if t == nil {
		return nil, ErrSendsTallyMissing
	}
	return NewSendsSummary(*t), nil
}
