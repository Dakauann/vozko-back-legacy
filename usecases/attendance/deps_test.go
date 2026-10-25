package attendance_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/agent_presence"
	"vozko/domain/cache"
	"vozko/domain/callsession"
	"vozko/domain/queue_event"
	"vozko/domain/shared/deps"
)

type quietQueue struct{}

func (quietQueue) Stats(string, *time.Time, *time.Time) (*queue_event.Stats, error) {
	return &queue_event.Stats{}, nil
}

type quietOccupancy struct{}

func (quietOccupancy) Occupancy(string, *time.Time, *time.Time) ([]agent_presence.OccupancyRow, error) {
	return nil, nil
}

type quietPresence struct{}

func (quietPresence) ListPresence(string) []callsession.MemberPresence { return nil }

type forgetfulMemo struct{}

func (forgetfulMemo) Remember(ctx context.Context, _ string, _ time.Duration, compute func(context.Context) ([]byte, error)) ([]byte, error) {
	return compute(ctx)
}

type freeGate struct{}

func (freeGate) Acquire(context.Context) (func(), error) { return func() {}, nil }

func passthroughCaching() SectionCaching {
	return SectionCaching{Memo: forgetfulMemo{}, Gate: freeGate{}, Versions: newMemoryVersions(), TTL: time.Minute}
}

func completeOverviewDeps() OverviewDeps {
	resolver := NewScheduleResolver(stubConfigReader{config: businessConfig()}, stubDepartmentSchedules{})
	return OverviewDeps{
		Repo:      &stubOverviewRepo{},
		Queue:     quietQueue{},
		Presence:  quietOccupancy{},
		Live:      quietPresence{},
		Schedules: resolver,
		Targets:   mustTargets(&stubTargetRepo{}, resolver, newMemoryVersions()),
		Caching:   passthroughCaching(),
	}
}

func mustTargets(repo *stubTargetRepo, resolver *ScheduleResolver, versions cache.Versions) *TargetsService {
	svc, err := NewTargetsService(TargetsDeps{Repo: repo, Schedules: resolver, Versions: versions})
	if err != nil {
		panic(err)
	}
	return svc
}

func TestNewGetOverviewUseCaseRefusesEveryMissingDependency(t *testing.T) {
	var typedNilTargets *TargetsService
	cases := map[string]func(*OverviewDeps){
		"overview repository": func(d *OverviewDeps) { d.Repo = nil },
		"queue stats":         func(d *OverviewDeps) { d.Queue = nil },
		"occupancy":           func(d *OverviewDeps) { d.Presence = nil },
		"live presence":       func(d *OverviewDeps) { d.Live = nil },
		"schedules":           func(d *OverviewDeps) { d.Schedules = nil },
		"targets":             func(d *OverviewDeps) { d.Targets = typedNilTargets },
		"section memo":        func(d *OverviewDeps) { d.Caching.Memo = nil },
		"analytics gate":      func(d *OverviewDeps) { d.Caching.Gate = nil },
		"section versions":    func(d *OverviewDeps) { d.Caching.Versions = nil },
	}
	for name, drop := range cases {
		t.Run(name, func(t *testing.T) {
			d := completeOverviewDeps()
			drop(&d)
			uc, err := NewGetOverviewUseCase(d)
			var missing *deps.MissingError
			if uc != nil || !errors.As(err, &missing) || len(missing.Missing) != 1 || missing.Missing[0] != name {
				t.Fatalf("uc = %v err = %v, want %s reported missing", uc, err, name)
			}
		})
	}
}

func TestNewGetOverviewUseCaseRefusesAnUncachedTTL(t *testing.T) {
	d := completeOverviewDeps()
	d.Caching.TTL = 0
	if uc, err := NewGetOverviewUseCase(d); uc != nil || !errors.Is(err, ErrSectionTTLRequired) {
		t.Fatalf("uc = %v err = %v, want ErrSectionTTLRequired", uc, err)
	}
}

func TestNewTargetsServiceRefusesEveryMissingDependency(t *testing.T) {
	resolver := NewScheduleResolver(stubConfigReader{config: businessConfig()}, stubDepartmentSchedules{})
	cases := map[string]TargetsDeps{
		"targets repository": {Schedules: resolver, Versions: newMemoryVersions()},
		"schedules":          {Repo: &stubTargetRepo{}, Versions: newMemoryVersions()},
		"section versions":   {Repo: &stubTargetRepo{}, Schedules: resolver},
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			svc, err := NewTargetsService(d)
			var missing *deps.MissingError
			if svc != nil || !errors.As(err, &missing) || missing.Missing[0] != name {
				t.Fatalf("svc = %v err = %v, want %s reported missing", svc, err, name)
			}
		})
	}
}

func TestATargetEditBumpsTheCachedSectionsVersion(t *testing.T) {
	resolver := NewScheduleResolver(stubConfigReader{config: businessConfig()}, stubDepartmentSchedules{})
	versions := newMemoryVersions()
	svc, err := NewTargetsService(TargetsDeps{Repo: &stubTargetRepo{}, Schedules: resolver, Versions: versions, Clock: func() time.Time {
		return time.Date(2026, 9, 28, 15, 36, 0, 0, time.UTC)
	}})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := versions.Version("ws1")
	if _, err := svc.Upsert(context.Background(), "ws1", septemberInput(), TargetAccess{IsAdmin: true, UserID: "u1"}); err != nil {
		t.Fatal(err)
	}
	if after, _ := versions.Version("ws1"); after == before {
		t.Fatal("a saved target must invalidate the cached overview sections")
	}
}
