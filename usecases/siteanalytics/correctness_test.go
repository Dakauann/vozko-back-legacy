package siteanalytics_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/sitepublish"
)

func (f *fakeStore) LastDay(context.Context) (time.Time, bool, error) {
	return f.lastDay, !f.lastDay.IsZero(), f.lastErr
}

func TestRollupStartsFromTheLastRolledDayAfterAnOutage(t *testing.T) {
	w := newWorld()
	w.store.lastDay = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	if err := w.service().Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC); !w.source.since.Equal(want) {
		t.Fatalf("since = %s, want the Sao Paulo midnight of the last rolled day %s", w.source.since, want)
	}
	w = newWorld()
	w.store.lastDay = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := w.service().Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 9, 28, 3, 0, 0, 0, time.UTC); !w.source.since.Equal(want) {
		t.Fatalf("the lookback is bounded: %s", w.source.since)
	}
	w = newWorld()
	w.store.lastErr = errors.New("down")
	if err := w.service().Execute(context.Background()); err == nil || len(w.store.upserted) != 0 {
		t.Fatal("an unreadable last day refuses")
	}
}

func TestTheConversionRateNeverPassesAHundredPercent(t *testing.T) {
	w := newWorld()
	w.store.rows = []sitepublish.DailyStat{{Day: now, Route: "/", Views: 3, Visitors: 2, FormSubmits: 3, WhatsApp: 2}}
	report, err := w.service().Read(context.Background(), Actor{UserID: "u", WorkspaceID: "w"}, "p1", "2026-10-01", "2026-10-28")
	if err != nil {
		t.Fatal(err)
	}
	if report.ConversionRate != basisPoints {
		t.Fatalf("got %d", report.ConversionRate)
	}
}

func TestReportDaysFollowSaoPaulo(t *testing.T) {
	w := newWorld()
	late := time.Date(2026, 10, 29, 1, 0, 0, 0, time.UTC)
	s := New(Deps{Source: w.source, Store: w.store, Access: w.access, Projects: w.projects, Leads: w.leads, Deals: w.deals, Hostnames: w.hostnames, Now: func() time.Time { return late }})
	report, err := s.Read(context.Background(), Actor{UserID: "u", WorkspaceID: "w"}, "p1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if report.To != "2026-10-28" {
		t.Fatalf("at 22h in Sao Paulo the day is still the 28th: %s", report.To)
	}
	if want := time.Date(2026, 9, 29, 3, 0, 0, 0, time.UTC); !w.leads.from.Equal(want) {
		t.Fatalf("leads from %s want %s", w.leads.from, want)
	}
	if want := time.Date(2026, 10, 29, 3, 0, 0, 0, time.UTC); !w.leads.to.Equal(want) {
		t.Fatalf("leads until %s want %s", w.leads.to, want)
	}
}
