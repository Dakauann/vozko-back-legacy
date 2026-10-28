package siteanalytics_usecase

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"vozko/domain/opportunity"
	"vozko/domain/shared"
	"vozko/domain/sitepublish"
	"vozko/domain/workspace"
)

type fakeSource struct {
	since   time.Time
	stats   []sitepublish.DailyStat
	sources []sitepublish.SourceStat
	err     error
	srcErr  error
}

func (f *fakeSource) Daily(_ context.Context, since time.Time) ([]sitepublish.DailyStat, error) {
	f.since = since
	return f.stats, f.err
}

func (f *fakeSource) Sources(_ context.Context, since time.Time) ([]sitepublish.SourceStat, error) {
	f.since = since
	return f.sources, f.srcErr
}

type fakeStore struct {
	upserted        []sitepublish.DailyStat
	upsertedSources []sitepublish.SourceStat
	rows            []sitepublish.DailyStat
	sourceRows      []sitepublish.SourceStat
	from, to        time.Time
	err             error
	lastDay         time.Time
	lastErr         error
}

func (f *fakeStore) Upsert(_ context.Context, stats []sitepublish.DailyStat) error {
	f.upserted = append(f.upserted, stats...)
	return f.err
}

func (f *fakeStore) UpsertSources(_ context.Context, stats []sitepublish.SourceStat) error {
	f.upsertedSources = append(f.upsertedSources, stats...)
	return f.err
}

func (f *fakeStore) List(_ context.Context, _ string, from, to time.Time) ([]sitepublish.DailyStat, error) {
	f.from, f.to = from, to
	return f.rows, f.err
}

func (f *fakeStore) ListSources(_ context.Context, _ string, from, to time.Time) ([]sitepublish.SourceStat, error) {
	return f.sourceRows, f.err
}

type fakeProjects struct{ err error }

func (f fakeProjects) Owns(_ context.Context, workspaceID, projectID string) error { return f.err }

type fakeAccess struct{ err error }

func (f fakeAccess) Execute(string, string, workspace.Resource, workspace.Action) error { return f.err }

type fakeLeads struct {
	leads    []string
	err      error
	from, to time.Time
}

func (f *fakeLeads) Leads(_ context.Context, _, _ string, from, to time.Time) ([]string, error) {
	f.from, f.to = from, to
	return f.leads, f.err
}

type fakeDeals struct {
	scope    opportunity.DealScope
	scopeErr error
	won      []opportunity.WonTotal
	err      error
	by       shared.Person
	leads    []string
	got      opportunity.DealScope
}

func (f *fakeDeals) Scope(by shared.Person, _ string) (opportunity.DealScope, error) {
	f.by = by
	return f.scope, f.scopeErr
}

func (f *fakeDeals) WonDealsOfLeads(_ context.Context, _ string, leadIDs []string, _, _ time.Time, scope opportunity.DealScope) ([]opportunity.WonTotal, error) {
	f.leads, f.got = leadIDs, scope
	return f.won, f.err
}

type fakeHostnames struct {
	hosts []string
	err   error
}

func (f fakeHostnames) ActiveHostnames(context.Context, string) ([]string, error) {
	return f.hosts, f.err
}

var now = time.Date(2026, 10, 28, 15, 30, 0, 0, time.UTC)

type world struct {
	source    *fakeSource
	store     *fakeStore
	access    fakeAccess
	projects  fakeProjects
	leads     *fakeLeads
	deals     *fakeDeals
	hostnames fakeHostnames
}

func newWorld() *world {
	return &world{source: &fakeSource{}, store: &fakeStore{}, leads: &fakeLeads{}, deals: &fakeDeals{}}
}

func (w *world) service() *Service {
	return New(Deps{
		Source: w.source, Store: w.store, Access: w.access, Projects: w.projects, Leads: w.leads, Deals: w.deals, Hostnames: w.hostnames,
		Now: func() time.Time { return now },
	})
}

func TestRollupRecomputesYesterdayAndToday(t *testing.T) {
	w := newWorld()
	w.source.stats = []sitepublish.DailyStat{{ProjectID: "p1", Day: now.Truncate(24 * time.Hour), Route: "/", Variant: "control", Views: 4}}
	w.source.sources = []sitepublish.SourceStat{{ProjectID: "p1", Day: now.Truncate(24 * time.Hour), Kind: sitepublish.SourceUTM, Value: "instagram", Views: 3}}
	if err := w.service().Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !w.source.since.Equal(time.Date(2026, 10, 27, 3, 0, 0, 0, time.UTC)) || len(w.store.upserted) != 1 || len(w.store.upsertedSources) != 1 {
		t.Fatalf("since = %s upserted = %+v sources = %+v", w.source.since, w.store.upserted, w.store.upsertedSources)
	}
}

func TestRollupNeverWritesZerosWhenTheSourceFails(t *testing.T) {
	w := newWorld()
	w.source.err = errors.New("down")
	if err := w.service().Execute(context.Background()); err == nil || len(w.store.upserted) != 0 {
		t.Fatalf("got %v %+v", err, w.store.upserted)
	}
	w = newWorld()
	w.source.srcErr = errors.New("down")
	if err := w.service().Execute(context.Background()); err == nil || len(w.store.upsertedSources) != 0 {
		t.Fatalf("a failed breakdown writes nothing: %v %+v", err, w.store.upsertedSources)
	}
}

func TestReadTotalsTheDays(t *testing.T) {
	w := newWorld()
	w.store.rows = []sitepublish.DailyStat{
		{Day: now, Route: "/", Variant: "control", Views: 10, Visitors: 6, FormSubmits: 2, WhatsApp: 1},
		{Day: now, Route: "/obrigado", Variant: "control", Views: 2, Visitors: 2},
	}
	report, err := w.service().Read(context.Background(), Actor{UserID: "u", WorkspaceID: "w"}, "p1", "2026-10-01", "2026-10-28")
	if err != nil {
		t.Fatal(err)
	}
	if report.Totals.Views != 12 || report.Totals.Visitors != 8 || report.Totals.Conversions != 3 || report.ConversionRate != 3750 || len(report.Days) != 2 {
		t.Fatalf("got %+v", report)
	}
	if !w.store.from.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) || !w.store.to.Equal(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("range %s %s", w.store.from, w.store.to)
	}
}

func TestReadBreaksVisitsDownBySourceWithoutThePageItself(t *testing.T) {
	w := newWorld()
	w.hostnames.hosts = []string{"loja.paginas.test"}
	w.store.sourceRows = []sitepublish.SourceStat{
		{Kind: sitepublish.SourceUTM, Value: "instagram", Views: 8, Submits: 2},
		{Kind: sitepublish.SourceReferrer, Value: "https://www.google.com", Views: 5},
		{Kind: sitepublish.SourceReferrer, Value: "https://loja.paginas.test", Views: 40},
	}
	report, err := w.service().Read(context.Background(), Actor{UserID: "u", WorkspaceID: "w"}, "p1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sources.UTM) != 1 || report.Sources.UTM[0] != (UTMSource{Source: "instagram", Views: 8, Submissions: 2}) {
		t.Fatalf("utm %+v", report.Sources.UTM)
	}
	if len(report.Sources.Referrers) != 1 || report.Sources.Referrers[0] != (ReferrerSource{Origin: "https://www.google.com", Views: 5}) {
		t.Fatalf("referrers %+v", report.Sources.Referrers)
	}
	empty, err := newWorld().service().Read(context.Background(), Actor{UserID: "u", WorkspaceID: "w"}, "p1", "", "")
	if err != nil || empty.Sources.UTM == nil || empty.Sources.Referrers == nil || empty.WonRevenueByCurrency == nil || empty.WonRevenueCurrency != "BRL" {
		t.Fatalf("empty lists are lists: %+v %v", empty, err)
	}
}

func TestReadJoinsTheLeadsAndTheWonDealsOfThePage(t *testing.T) {
	w := newWorld()
	w.leads.leads = []string{"lead-1", "lead-2"}
	w.deals.scope = opportunity.DealScope{Restrict: true, AssigneeOverride: "u"}
	w.deals.won = []opportunity.WonTotal{{Currency: "BRL", Count: 2, ValueCents: 300000}, {Currency: "USD", Count: 1, ValueCents: 5000}}
	report, err := w.service().Read(context.Background(), Actor{UserID: "u", WorkspaceID: "w", IsAdmin: true}, "p1", "2026-10-01", "2026-10-28")
	if err != nil {
		t.Fatal(err)
	}
	if report.Leads != 2 || report.WonDeals != 3 || !report.DealsVisible {
		t.Fatalf("got %+v", report)
	}
	if report.WonRevenue != 3000 || report.WonRevenueCurrency != "BRL" || len(report.WonRevenueByCurrency) != 2 || report.WonRevenueByCurrency[1] != (WonRevenue{Currency: "USD", ValueCents: 5000}) {
		t.Fatalf("revenue %+v", report)
	}
	if !w.leads.from.Equal(time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)) || !w.leads.to.Equal(time.Date(2026, 10, 29, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("the range covers the whole last day: %s %s", w.leads.from, w.leads.to)
	}
	if w.deals.by != (shared.Person{UserID: "u", SystemAdmin: true}) || w.deals.got.AssigneeOverride != "u" || len(w.deals.leads) != 2 {
		t.Fatalf("the deals follow the deal scope of the viewer: %+v %+v", w.deals.by, w.deals.got)
	}
}

func TestReadHidesTheDealsOfAViewerWithoutDealAccess(t *testing.T) {
	w := newWorld()
	w.leads.leads = []string{"lead-1"}
	w.deals.scopeErr = opportunity.ErrScopeDenied
	w.deals.won = []opportunity.WonTotal{{Currency: "BRL", Count: 1, ValueCents: 100}}
	report, err := w.service().Read(context.Background(), Actor{UserID: "u", WorkspaceID: "w"}, "p1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if report.DealsVisible || report.WonDeals != 0 || report.WonRevenue != 0 || len(report.WonRevenueByCurrency) != 0 || report.Leads != 1 || w.deals.leads != nil {
		t.Fatalf("got %+v", report)
	}
}

func TestReadRefusals(t *testing.T) {
	a := Actor{UserID: "u", WorkspaceID: "w"}
	broken := errors.New("db")
	cases := map[string]struct {
		change   func(*world)
		from, to string
		want     error
	}{
		"bad period":       {func(*world) {}, "2026-10-28", "2026-10-01", ErrInvalidPeriod},
		"too long":         {func(*world) {}, "2024-01-01", "2026-10-01", ErrInvalidPeriod},
		"not a date":       {func(*world) {}, "ontem", "2026-10-01", ErrInvalidPeriod},
		"denied":           {func(w *world) { w.access.err = errors.New("no") }, "2026-10-01", "2026-10-02", ErrAccessDenied},
		"other page":       {func(w *world) { w.projects.err = ErrProjectNotFound }, "2026-10-01", "2026-10-02", ErrProjectNotFound},
		"store broken":     {func(w *world) { w.store.err = broken }, "2026-10-01", "2026-10-02", ErrUnavailable},
		"leads broken":     {func(w *world) { w.leads.err = broken }, "2026-10-01", "2026-10-02", ErrUnavailable},
		"deals broken":     {func(w *world) { w.leads.leads = []string{"l"}; w.deals.err = broken }, "2026-10-01", "2026-10-02", ErrUnavailable},
		"scope broken":     {func(w *world) { w.deals.scopeErr = broken }, "2026-10-01", "2026-10-02", ErrUnavailable},
		"hostnames broken": {func(w *world) { w.hostnames.err = broken }, "2026-10-01", "2026-10-02", ErrUnavailable},
	}
	for name, c := range cases {
		w := newWorld()
		c.change(w)
		if _, err := w.service().Read(context.Background(), a, "p1", c.from, c.to); !errors.Is(err, c.want) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
}

func TestReadBreaksTheDaysDownByRouteVariantAndDay(t *testing.T) {
	w := newWorld()
	yesterday := now.Add(-24 * time.Hour)
	w.store.rows = []sitepublish.DailyStat{
		{Day: yesterday, Route: "/", Variant: "control", Views: 10, Visitors: 6, CTA: 3, FormSubmits: 2, WhatsApp: 1},
		{Day: now, Route: "/", Variant: "b", Views: 4, Visitors: 3, FormSubmits: 1},
		{Day: now, Route: "/obrigado", Variant: "control", Views: 4, Visitors: 4, WhatsApp: 2},
	}
	report, err := w.service().Read(context.Background(), Actor{UserID: "u", WorkspaceID: "w"}, "p1", "2026-10-01", "2026-10-28")
	if err != nil {
		t.Fatal(err)
	}
	routes := []Breakdown{
		{Key: "/", Views: 14, Visitors: 9, CTA: 3, FormSubmits: 3, WhatsApp: 1, Conversions: 4},
		{Key: "/obrigado", Views: 4, Visitors: 4, WhatsApp: 2, Conversions: 2},
	}
	variants := []Breakdown{
		{Key: "control", Views: 14, Visitors: 10, CTA: 3, FormSubmits: 2, WhatsApp: 3, Conversions: 5},
		{Key: "b", Views: 4, Visitors: 3, FormSubmits: 1, Conversions: 1},
	}
	daily := []DailyPoint{
		{Day: yesterday.Format("2006-01-02"), Views: 10, Visitors: 6, Conversions: 3},
		{Day: now.Format("2006-01-02"), Views: 8, Visitors: 7, Conversions: 3},
	}
	if !slices.Equal(report.ByRoute, routes) || !slices.Equal(report.ByVariant, variants) || !slices.Equal(report.Daily, daily) {
		t.Fatalf("got %+v\n%+v\n%+v", report.ByRoute, report.ByVariant, report.Daily)
	}
	w.store.rows = nil
	empty, _ := w.service().Read(context.Background(), Actor{UserID: "u", WorkspaceID: "w"}, "p1", "2026-10-01", "2026-10-28")
	if empty.ByRoute == nil || empty.ByVariant == nil || empty.Daily == nil {
		t.Fatal("empty breakdowns are lists")
	}
}
