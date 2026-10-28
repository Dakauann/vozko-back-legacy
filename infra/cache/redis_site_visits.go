package cache

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

const (
	siteVisitsRetention = 8 * 24 * time.Hour
	siteVisitsPrefix    = "site:visits:"
	fieldSeparator      = "|"
)

type SiteVisits struct {
	client *redis.Client
	now    func() time.Time
}

func (p *RedisProvider) SiteVisits(now func() time.Time) *SiteVisits {
	return &SiteVisits{client: p.client, now: now}
}

type statKey struct {
	project, route, variant string
}

type sourceKey struct {
	project string
	event   sitepublish.VisitEvent
	kind    sitepublish.SourceKind
	value   string
}

func dayLabel(day time.Time) string {
	return day.Format(time.DateOnly)
}

func countsKey(day string) string {
	return siteVisitsPrefix + "counts:" + day
}

func sourcesKey(day string) string {
	return siteVisitsPrefix + "sources:" + day
}

func peopleKey(day string, k statKey) string {
	return siteVisitsPrefix + "people:" + day + ":" + strings.Join([]string{k.project, k.route, k.variant}, fieldSeparator)
}

func countField(v sitepublish.Visit) string {
	return strings.Join([]string{v.ProjectID, v.Route, v.Variant, string(v.Event)}, fieldSeparator)
}

func sourceField(v sitepublish.Visit, kind sitepublish.SourceKind, value string) string {
	return strings.Join([]string{v.ProjectID, string(v.Event), string(kind), value}, fieldSeparator)
}

func parseCountField(field string) (statKey, sitepublish.VisitEvent, bool) {
	parts := strings.Split(field, fieldSeparator)
	if len(parts) != 4 || parts[0] == "" || !site.ValidRoutePath(parts[1]) || !sitepublish.ValidVariantKey(parts[2]) {
		return statKey{}, "", false
	}
	event, ok := sitepublish.ParseVisitEvent(parts[3])
	return statKey{project: parts[0], route: parts[1], variant: parts[2]}, event, ok
}

func parseSourceField(field string) (sourceKey, bool) {
	parts := strings.SplitN(field, fieldSeparator, 4)
	if len(parts) != 4 || parts[0] == "" {
		return sourceKey{}, false
	}
	event, ok := sitepublish.ParseVisitEvent(parts[1])
	kind := sitepublish.SourceKind(parts[2])
	if !ok || !sitepublish.ValidSource(kind, parts[3]) {
		return sourceKey{}, false
	}
	return sourceKey{project: parts[0], event: event, kind: kind, value: parts[3]}, true
}

func (s *SiteVisits) Record(ctx context.Context, v sitepublish.Visit) error {
	day := dayLabel(v.Day)
	_, err := s.client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HIncrBy(ctx, countsKey(day), countField(v), 1)
		pipe.Expire(ctx, countsKey(day), siteVisitsRetention)
		if v.Event == sitepublish.VisitView {
			people := peopleKey(day, statKey{project: v.ProjectID, route: v.Route, variant: v.Variant})
			pipe.PFAdd(ctx, people, v.Visitor)
			pipe.Expire(ctx, people, siteVisitsRetention)
		}
		if !v.CountsForSources() {
			return nil
		}
		for _, kind := range sitepublish.SourceKinds() {
			if value := v.Source(kind); value != "" {
				pipe.HIncrBy(ctx, sourcesKey(day), sourceField(v, kind, value), 1)
				pipe.Expire(ctx, sourcesKey(day), siteVisitsRetention)
			}
		}
		return nil
	})
	return err
}

func (s *SiteVisits) days(since time.Time) []time.Time {
	var out []time.Time
	last := sitepublish.AnalyticsDay(s.now())
	for day := sitepublish.AnalyticsDay(since); !day.After(last); day = day.AddDate(0, 0, 1) {
		out = append(out, day)
	}
	return out
}

func (s *SiteVisits) Daily(ctx context.Context, since time.Time) ([]sitepublish.DailyStat, error) {
	var out []sitepublish.DailyStat
	for _, day := range s.days(since) {
		stats, err := s.dailyOf(ctx, day)
		if err != nil {
			return nil, err
		}
		out = append(out, stats...)
	}
	return out, nil
}

func (s *SiteVisits) dailyOf(ctx context.Context, day time.Time) ([]sitepublish.DailyStat, error) {
	label := dayLabel(day)
	fields, err := s.client.HGetAll(ctx, countsKey(label)).Result()
	if err != nil {
		return nil, err
	}
	byKey := map[statKey]*sitepublish.DailyStat{}
	for field, raw := range fields {
		k, event, ok := parseCountField(field)
		n, err := strconv.ParseInt(raw, 10, 64)
		if !ok || err != nil {
			continue
		}
		stat, found := byKey[k]
		if !found {
			stat = &sitepublish.DailyStat{ProjectID: k.project, Day: day, Route: k.route, Variant: k.variant}
			byKey[k] = stat
		}
		stat.Count(event, n)
	}
	keys := make([]statKey, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.Join([]string{keys[i].project, keys[i].route, keys[i].variant}, fieldSeparator) < strings.Join([]string{keys[j].project, keys[j].route, keys[j].variant}, fieldSeparator)
	})
	counts := make([]*redis.IntCmd, len(keys))
	if _, err := s.client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for i, k := range keys {
			counts[i] = pipe.PFCount(ctx, peopleKey(label, k))
		}
		return nil
	}); err != nil {
		return nil, err
	}
	out := make([]sitepublish.DailyStat, len(keys))
	for i, k := range keys {
		stat := byKey[k]
		stat.Visitors = min(counts[i].Val(), stat.Views)
		out[i] = *stat
	}
	return out, nil
}

func (s *SiteVisits) Sources(ctx context.Context, since time.Time) ([]sitepublish.SourceStat, error) {
	var out []sitepublish.SourceStat
	for _, day := range s.days(since) {
		fields, err := s.client.HGetAll(ctx, sourcesKey(dayLabel(day))).Result()
		if err != nil {
			return nil, err
		}
		type valueKey struct {
			project, value string
			kind           sitepublish.SourceKind
		}
		byKey := map[valueKey]*sitepublish.SourceStat{}
		for field, raw := range fields {
			k, ok := parseSourceField(field)
			n, err := strconv.ParseInt(raw, 10, 64)
			if !ok || err != nil {
				continue
			}
			vk := valueKey{project: k.project, value: k.value, kind: k.kind}
			stat, found := byKey[vk]
			if !found {
				stat = &sitepublish.SourceStat{ProjectID: k.project, Day: day, Kind: k.kind, Value: k.value}
				byKey[vk] = stat
			}
			stat.Count(k.event, n)
		}
		for _, stat := range byKey {
			out = append(out, *stat)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if !a.Day.Equal(b.Day) {
			return a.Day.Before(b.Day)
		}
		return strings.Join([]string{a.ProjectID, string(a.Kind), a.Value}, fieldSeparator) < strings.Join([]string{b.ProjectID, string(b.Kind), b.Value}, fieldSeparator)
	})
	return out, nil
}
