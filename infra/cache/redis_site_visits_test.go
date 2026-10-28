package cache

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/sitepublish"
	"vozko/infra/testenv"
)

func siteVisit(project, route, variant string, event sitepublish.VisitEvent, visitor string) sitepublish.Visit {
	return sitepublish.Visit{
		ProjectID: project, Day: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), Route: route, Variant: variant, Event: event,
		Visitor: strings.Repeat(visitor, sitepublish.VisitorHashLength),
	}
}

func TestVisitFieldsRoundTripAndRefuseNoise(t *testing.T) {
	v := siteVisit("p1", "/promo", "b", sitepublish.VisitCTA, "a")
	got, event, ok := parseCountField(countField(v))
	if !ok || got != (statKey{project: "p1", route: "/promo", variant: "b"}) || event != sitepublish.VisitCTA {
		t.Fatalf("got %+v %s %v", got, event, ok)
	}
	src, ok := parseSourceField(sourceField(v, sitepublish.SourceUTM, "black | friday"))
	if !ok || src != (sourceKey{project: "p1", event: sitepublish.VisitCTA, kind: sitepublish.SourceUTM, value: "black | friday"}) {
		t.Fatalf("got %+v %v", src, ok)
	}
	for _, noise := range []string{"", "p1|/promo|b", "p1|/Promo|b|cta", "p1|/promo|b|click", "p1|/promo|B!|cta"} {
		if _, _, ok := parseCountField(noise); ok {
			t.Fatalf("%q must be skipped", noise)
		}
	}
	for _, noise := range []string{"p1|view|device|mobile", "p1|click|utm|x", "p1|view|referrer|https://a.test/path"} {
		if _, ok := parseSourceField(noise); ok {
			t.Fatalf("%q must be skipped", noise)
		}
	}
}

func TestSiteVisitsAgainstRedis(t *testing.T) {
	target := testenv.Redis(t)
	now := time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC)
	visits := NewRedisProvider(target.Addr, target.Password).SiteVisits(func() time.Time { return now })
	project := uuid.NewString()
	ctx := context.Background()
	record := func(v sitepublish.Visit) {
		t.Helper()
		if err := visits.Record(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	first := siteVisit(project, "/", "control", sitepublish.VisitView, "a")
	first.UTMSource, first.Referrer = "instagram", "https://www.instagram.com"
	record(first)
	record(first)
	record(siteVisit(project, "/", "control", sitepublish.VisitView, "b"))
	submit := siteVisit(project, "/", "control", sitepublish.VisitFormSubmit, "a")
	submit.UTMSource = "instagram"
	record(submit)
	cta := siteVisit(project, "/", "control", sitepublish.VisitCTA, "a")
	cta.UTMSource = "instagram"
	record(cta)

	stats, err := visits.Daily(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var mine []sitepublish.DailyStat
	for _, s := range stats {
		if s.ProjectID == project {
			mine = append(mine, s)
		}
	}
	if len(mine) != 1 || mine[0].Views != 3 || mine[0].Visitors != 2 || mine[0].FormSubmits != 1 || mine[0].CTA != 1 || mine[0].Route != "/" || mine[0].Variant != "control" {
		t.Fatalf("got %+v", mine)
	}
	sources, err := visits.Sources(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	totals := map[sitepublish.SourceKind]sitepublish.SourceStat{}
	for _, s := range sources {
		if s.ProjectID == project {
			totals[s.Kind] = s
		}
	}
	if utm := totals[sitepublish.SourceUTM]; utm.Value != "instagram" || utm.Views != 2 || utm.Submits != 1 {
		t.Fatalf("got %+v", totals)
	}
	if ref := totals[sitepublish.SourceReferrer]; ref.Value != "https://www.instagram.com" || ref.Views != 2 || ref.Submits != 0 {
		t.Fatalf("got %+v", totals)
	}
}
