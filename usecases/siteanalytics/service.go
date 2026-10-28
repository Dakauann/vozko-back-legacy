package siteanalytics_usecase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"vozko/domain/opportunity"
	"vozko/domain/shared"
	"vozko/domain/sitepublish"
	"vozko/domain/workspace"
)

const (
	maxPeriodDays   = 400
	basisPoints     = 10_000
	maxRollupDays   = 30
	dateLayout      = "2006-01-02"
	defaultLookback = 29
	maxSourceRows   = 25
	defaultCurrency = "BRL"
	centsPerUnit    = 100
)

var (
	ErrInvalidPeriod   = errors.New("siteanalytics: the period is invalid")
	ErrAccessDenied    = errors.New("siteanalytics: the actor may not read this page")
	ErrProjectNotFound = errors.New("siteanalytics: the page was not found")
	ErrUnavailable     = errors.New("siteanalytics: the numbers cannot be read now")
)

type Actor struct {
	UserID      string
	WorkspaceID string
	IsAdmin     bool
}

type Source interface {
	Daily(ctx context.Context, since time.Time) ([]sitepublish.DailyStat, error)
	Sources(ctx context.Context, since time.Time) ([]sitepublish.SourceStat, error)
}

type Store interface {
	Upsert(ctx context.Context, stats []sitepublish.DailyStat) error
	List(ctx context.Context, projectID string, from, to time.Time) ([]sitepublish.DailyStat, error)
	UpsertSources(ctx context.Context, stats []sitepublish.SourceStat) error
	ListSources(ctx context.Context, projectID string, from, to time.Time) ([]sitepublish.SourceStat, error)
	LastDay(ctx context.Context) (time.Time, bool, error)
}

type Projects interface {
	Owns(ctx context.Context, workspaceID, projectID string) error
}

type AccessChecker interface {
	Execute(userID, workspaceID string, resource workspace.Resource, action workspace.Action) error
}

type PageLeads interface {
	Leads(ctx context.Context, workspaceID, projectID string, from, to time.Time) ([]string, error)
}

type Deals interface {
	Scope(by shared.Person, workspaceID string) (opportunity.DealScope, error)
	WonDealsOfLeads(ctx context.Context, workspaceID string, leadIDs []string, from, to time.Time, scope opportunity.DealScope) ([]opportunity.WonTotal, error)
}

type Hostnames interface {
	ActiveHostnames(ctx context.Context, projectID string) ([]string, error)
}

type Totals struct {
	Views       int64 `json:"views"`
	Visitors    int64 `json:"visitors"`
	CTA         int64 `json:"cta"`
	FormViews   int64 `json:"formViews"`
	FormStarts  int64 `json:"formStarts"`
	FormSubmits int64 `json:"formSubmits"`
	WhatsApp    int64 `json:"whatsapp"`
	Conversions int64 `json:"conversions"`
}

type UTMSource struct {
	Source      string `json:"source" example:"instagram"`
	Views       int64  `json:"views"`
	Submissions int64  `json:"submissions"`
}

type ReferrerSource struct {
	Origin      string `json:"origin" example:"https://www.google.com"`
	Views       int64  `json:"views"`
	Submissions int64  `json:"submissions"`
}

type Sources struct {
	UTM       []UTMSource      `json:"utm"`
	Referrers []ReferrerSource `json:"referrers"`
}

type WonRevenue struct {
	Currency   string `json:"currency" example:"BRL"`
	ValueCents int64  `json:"valueCents" example:"150000"`
}

type Breakdown struct {
	Key         string `json:"key" example:"/"`
	Views       int64  `json:"views"`
	Visitors    int64  `json:"visitors"`
	CTA         int64  `json:"cta"`
	FormSubmits int64  `json:"formSubmits"`
	WhatsApp    int64  `json:"whatsapp"`
	Conversions int64  `json:"conversions"`
}

type DailyPoint struct {
	Day         string `json:"day" example:"2026-10-28"`
	Views       int64  `json:"views"`
	Visitors    int64  `json:"visitors"`
	Conversions int64  `json:"conversions"`
}

type Report struct {
	From                 string                  `json:"from"`
	To                   string                  `json:"to"`
	Totals               Totals                  `json:"totals"`
	ConversionRate       int64                   `json:"conversionRate"`
	Days                 []sitepublish.DailyStat `json:"days"`
	ByRoute              []Breakdown             `json:"byRoute"`
	ByVariant            []Breakdown             `json:"byVariant"`
	Daily                []DailyPoint            `json:"daily"`
	Sources              Sources                 `json:"sources"`
	Leads                int64                   `json:"leads"`
	WonDeals             int64                   `json:"wonDeals"`
	WonRevenue           float64                 `json:"wonRevenue" example:"1500.5"`
	WonRevenueCurrency   string                  `json:"wonRevenueCurrency" example:"BRL"`
	WonRevenueByCurrency []WonRevenue            `json:"wonRevenueByCurrency"`
	DealsVisible         bool                    `json:"dealsVisible"`
}

type Deps struct {
	Source    Source
	Store     Store
	Access    AccessChecker
	Projects  Projects
	Leads     PageLeads
	Deals     Deals
	Hostnames Hostnames
	Now       func() time.Time
}

type Service struct {
	source    Source
	store     Store
	access    AccessChecker
	projects  Projects
	leads     PageLeads
	deals     Deals
	hostnames Hostnames
	now       func() time.Time
}

func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{source: d.Source, store: d.Store, access: d.Access, projects: d.Projects, leads: d.Leads, deals: d.Deals, hostnames: d.Hostnames, now: d.Now}
}

func (s *Service) rollupSince(ctx context.Context) (time.Time, error) {
	today := sitepublish.AnalyticsDay(s.now())
	from := today.AddDate(0, 0, -1)
	last, found, err := s.store.LastDay(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("siteanalytics: the last rolled day: %w", err)
	}
	if found && last.Before(from) {
		from = last
	}
	if floor := today.AddDate(0, 0, -maxRollupDays); from.Before(floor) {
		from = floor
	}
	return sitepublish.AnalyticsDayStart(from), nil
}

func (s *Service) Execute(ctx context.Context) error {
	since, err := s.rollupSince(ctx)
	if err != nil {
		return err
	}
	stats, err := s.source.Daily(ctx, since)
	if err != nil {
		return fmt.Errorf("siteanalytics: rollup source: %w", err)
	}
	sources, err := s.source.Sources(ctx, since)
	if err != nil {
		return fmt.Errorf("siteanalytics: rollup sources: %w", err)
	}
	if err := s.store.Upsert(ctx, stats); err != nil {
		return err
	}
	return s.store.UpsertSources(ctx, sources)
}

func (s *Service) period(fromRaw, toRaw string) (time.Time, time.Time, error) {
	today := sitepublish.AnalyticsDay(s.now())
	from, to := today.AddDate(0, 0, -defaultLookback), today
	var err error
	if fromRaw != "" {
		if from, err = time.Parse(dateLayout, fromRaw); err != nil {
			return from, to, ErrInvalidPeriod
		}
	}
	if toRaw != "" {
		if to, err = time.Parse(dateLayout, toRaw); err != nil {
			return from, to, ErrInvalidPeriod
		}
	}
	if to.Before(from) || to.Sub(from) > maxPeriodDays*24*time.Hour {
		return from, to, ErrInvalidPeriod
	}
	return from, to, nil
}

func (s *Service) Read(ctx context.Context, a Actor, projectID, fromRaw, toRaw string) (Report, error) {
	if a.UserID == "" || a.WorkspaceID == "" {
		return Report{}, ErrAccessDenied
	}
	if err := s.access.Execute(a.UserID, a.WorkspaceID, workspace.ResourceSites, workspace.ActionRead); err != nil {
		return Report{}, fmt.Errorf("%w: %w", ErrAccessDenied, err)
	}
	from, to, err := s.period(fromRaw, toRaw)
	if err != nil {
		return Report{}, err
	}
	if err := s.projects.Owns(ctx, a.WorkspaceID, projectID); err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return Report{}, err
		}
		return Report{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	days, err := s.store.List(ctx, projectID, from, to)
	if err != nil {
		return Report{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	report := Report{From: from.Format(dateLayout), To: to.Format(dateLayout), Days: days}
	if report.Days == nil {
		report.Days = []sitepublish.DailyStat{}
	}
	for _, d := range days {
		t := &report.Totals
		t.Views += d.Views
		t.Visitors += d.Visitors
		t.CTA += d.CTA
		t.FormViews += d.FormViews
		t.FormStarts += d.FormStarts
		t.FormSubmits += d.FormSubmits
		t.WhatsApp += d.WhatsApp
	}
	report.Totals.Conversions = conversions(report.Totals.FormSubmits, report.Totals.WhatsApp)
	report.ByRoute = breakdown(days, func(d sitepublish.DailyStat) string { return d.Route })
	report.ByVariant = breakdown(days, func(d sitepublish.DailyStat) string { return d.Variant })
	report.Daily = daily(days)
	report.ConversionRate = conversionRate(report.Totals)
	if report.Sources, err = s.sources(ctx, projectID, from, to); err != nil {
		return Report{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if err := s.outcomes(ctx, a, projectID, sitepublish.AnalyticsDayStart(from), sitepublish.AnalyticsDayStart(to.AddDate(0, 0, 1)), &report); err != nil {
		return Report{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return report, nil
}

func (s *Service) sources(ctx context.Context, projectID string, from, to time.Time) (Sources, error) {
	rows, err := s.store.ListSources(ctx, projectID, from, to)
	if err != nil {
		return Sources{}, err
	}
	hosts, err := s.hostnames.ActiveHostnames(ctx, projectID)
	if err != nil {
		return Sources{}, err
	}
	own := func(origin string) bool {
		u, err := url.Parse(origin)
		return err == nil && slices.Contains(hosts, strings.ToLower(u.Hostname()))
	}
	out := Sources{UTM: []UTMSource{}, Referrers: []ReferrerSource{}}
	for _, t := range sitepublish.RankSources(rows, sitepublish.SourceUTM, nil, maxSourceRows) {
		out.UTM = append(out.UTM, UTMSource{Source: t.Value, Views: t.Views, Submissions: t.Submits})
	}
	for _, t := range sitepublish.RankSources(rows, sitepublish.SourceReferrer, own, maxSourceRows) {
		out.Referrers = append(out.Referrers, ReferrerSource{Origin: t.Value, Views: t.Views, Submissions: t.Submits})
	}
	return out, nil
}

func (s *Service) outcomes(ctx context.Context, a Actor, projectID string, from, until time.Time, report *Report) error {
	report.WonRevenueByCurrency, report.WonRevenueCurrency = []WonRevenue{}, defaultCurrency
	leads, err := s.leads.Leads(ctx, a.WorkspaceID, projectID, from, until)
	if err != nil {
		return err
	}
	report.Leads = int64(len(leads))
	scope, err := s.deals.Scope(shared.Person{UserID: a.UserID, SystemAdmin: a.IsAdmin}, a.WorkspaceID)
	if errors.Is(err, opportunity.ErrScopeDenied) {
		return nil
	}
	if err != nil {
		return err
	}
	report.DealsVisible = true
	if len(leads) == 0 {
		return nil
	}
	won, err := s.deals.WonDealsOfLeads(ctx, a.WorkspaceID, leads, from, until, scope)
	if err != nil {
		return err
	}
	for _, w := range won {
		report.WonDeals += w.Count
		report.WonRevenueByCurrency = append(report.WonRevenueByCurrency, WonRevenue{Currency: w.Currency, ValueCents: w.ValueCents})
	}
	if len(won) > 0 {
		report.WonRevenueCurrency = won[0].Currency
		report.WonRevenue = float64(won[0].ValueCents) / centsPerUnit
	}
	return nil
}
