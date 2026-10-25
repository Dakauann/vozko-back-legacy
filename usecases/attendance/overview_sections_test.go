package attendance_usecase

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"vozko/domain/attendance"
	at "vozko/domain/attendance_target"
	"vozko/domain/cache"
)

type memoryMemo struct {
	mu     sync.Mutex
	values map[string][]byte
}

func newMemoryMemo() *memoryMemo { return &memoryMemo{values: map[string][]byte{}} }

func (m *memoryMemo) Remember(
	ctx context.Context,
	key string,
	_ time.Duration,
	compute func(context.Context) ([]byte, error),
) ([]byte, error) {
	m.mu.Lock()
	cached, ok := m.values[key]
	m.mu.Unlock()
	if ok {
		return cached, nil
	}
	value, err := compute(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.values[key] = value
	m.mu.Unlock()
	return value, nil
}

type memoryVersions struct {
	mu       sync.Mutex
	versions map[string]int
	readErr  error
	bumpErr  error
}

func newMemoryVersions() *memoryVersions { return &memoryVersions{versions: map[string]int{}} }

func (v *memoryVersions) Version(scope string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.readErr != nil {
		return "", v.readErr
	}
	return strconv.Itoa(v.versions[scope]), nil
}

func (v *memoryVersions) Bump(scope string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.bumpErr != nil {
		return v.bumpErr
	}
	v.versions[scope]++
	return nil
}

type trackingGate struct {
	mu       sync.Mutex
	held     int
	acquired int
	busyErr  error
}

func (g *trackingGate) acquisitions() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.acquired
}

func (g *trackingGate) Acquire(context.Context) (func(), error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.busyErr != nil {
		return nil, g.busyErr
	}
	g.held++
	g.acquired++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			g.held--
			g.mu.Unlock()
		})
	}, nil
}

func (g *trackingGate) holding() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.held
}

func cachedUseCase(repo *stubOverviewRepo, targets *stubTargetRepo) (*getOverviewUseCase, *memoryVersions, *trackingGate) {
	versions := newMemoryVersions()
	gate := &trackingGate{}
	uc := overviewWith(repo, businessConfig(), nil, targets, SectionCaching{Memo: newMemoryMemo(), Gate: gate, Versions: versions, TTL: time.Minute})
	return uc, versions, gate
}

func TestASectionIsReadOnceWhileItIsCached(t *testing.T) {
	repo := &stubOverviewRepo{}
	uc, _, _ := cachedUseCase(repo, &stubTargetRepo{})
	ctx := context.Background()

	for range 3 {
		if _, err := uc.Summary(ctx, "ws1", attendance.OverviewFilter{}); err != nil {
			t.Fatalf("Summary() error = %v", err)
		}
		if _, err := uc.Stages(ctx, "ws1", attendance.OverviewFilter{}); err != nil {
			t.Fatalf("Stages() error = %v", err)
		}
	}
	if repo.calls("summary") != 1 || repo.calls("stages") != 1 {
		t.Fatalf("reads = %d summary / %d stages, want 1 each", repo.calls("summary"), repo.calls("stages"))
	}
}

func TestTrendAndTeamReuseTheCachedSummary(t *testing.T) {
	repo := &stubOverviewRepo{}
	uc, _, _ := cachedUseCase(repo, &stubTargetRepo{})
	ctx := context.Background()

	if _, err := uc.Summary(ctx, "ws1", attendance.OverviewFilter{}); err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if _, err := uc.Trend(ctx, "ws1", attendance.OverviewFilter{}); err != nil {
		t.Fatalf("Trend() error = %v", err)
	}
	if _, err := uc.Team(ctx, "ws1", attendance.OverviewFilter{}); err != nil {
		t.Fatalf("Team() error = %v", err)
	}
	if repo.calls("summary") != 1 {
		t.Fatalf("summary was read %d times, want 1: trend and team must reuse it", repo.calls("summary"))
	}
}

func TestChangingTheRankingMetricRereadsOnlyTheTeam(t *testing.T) {
	repo := &stubOverviewRepo{}
	uc, _, _ := cachedUseCase(repo, &stubTargetRepo{})
	ctx := context.Background()

	for _, metric := range []string{"resolved", "volume", "revenue_cents"} {
		if _, err := uc.Team(ctx, "ws1", attendance.OverviewFilter{RankMetric: metric}); err != nil {
			t.Fatalf("Team(%s) error = %v", metric, err)
		}
	}
	if repo.calls("summary") != 1 || repo.calls("team") != 3 {
		t.Fatalf("reads = %d summary / %d team, want 1 / 3", repo.calls("summary"), repo.calls("team"))
	}
}

func TestSavingATargetRecomputesTheWorkspaceSections(t *testing.T) {
	repo := &stubOverviewRepo{}
	targets := &stubTargetRepo{}
	uc, _, _ := cachedUseCase(repo, targets)
	ctx := context.Background()

	if _, err := uc.Summary(ctx, "ws1", attendance.OverviewFilter{}); err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	_, err := uc.targets.Upsert(ctx, "ws1", UpsertTargetInput{
		Scope:     at.ScopeWorkspace,
		MetricKey: attendance.MetricFinished,
		Value:     100,
	}, TargetAccess{IsAdmin: true})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if _, err := uc.Summary(ctx, "ws1", attendance.OverviewFilter{}); err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if repo.calls("summary") != 2 {
		t.Fatalf("summary was read %d times, want 2: a saved target must not be hidden behind the cache", repo.calls("summary"))
	}
}

func TestAnUnknownVersionNeverServesACachedSection(t *testing.T) {
	repo := &stubOverviewRepo{}
	uc, versions, _ := cachedUseCase(repo, &stubTargetRepo{})
	versions.readErr = errors.New("redis down")
	ctx := context.Background()

	for range 2 {
		if _, err := uc.Summary(ctx, "ws1", attendance.OverviewFilter{}); err != nil {
			t.Fatalf("Summary() error = %v", err)
		}
	}
	if repo.calls("summary") != 2 {
		t.Fatalf("summary was read %d times, want 2 when the version cannot be read", repo.calls("summary"))
	}
}

func TestABusyGateRefusesInsteadOfReadingAnyway(t *testing.T) {
	repo := &stubOverviewRepo{}
	uc, _, gate := cachedUseCase(repo, &stubTargetRepo{})
	gate.busyErr = cache.ErrGateBusy

	_, err := uc.Backlog(context.Background(), "ws1", attendance.OverviewFilter{})
	if !errors.Is(err, cache.ErrGateBusy) {
		t.Fatalf("Backlog() error = %v, want ErrGateBusy", err)
	}
	if repo.calls("backlog") != 0 {
		t.Fatalf("the backlog was read although the gate refused")
	}
}

func TestADependentSectionNeverHoldsASlotWhileItWaitsForTheSummary(t *testing.T) {
	repo := &stubOverviewRepo{}
	uc, _, gate := cachedUseCase(repo, &stubTargetRepo{})
	repo.onRead = func(string) {
		if held := gate.holding(); held != 1 {
			t.Errorf("a repository read ran with %d slots held, want exactly its own", held)
		}
	}

	if _, err := uc.Trend(context.Background(), "ws1", attendance.OverviewFilter{}); err != nil {
		t.Fatalf("Trend() error = %v", err)
	}
	if _, err := uc.Team(context.Background(), "ws1", attendance.OverviewFilter{MemberID: "u1"}); err != nil {
		t.Fatalf("Team() error = %v", err)
	}
	if gate.holding() != 0 {
		t.Fatalf("%d slots were never released", gate.holding())
	}
}

func TestExecuteComposesEverySection(t *testing.T) {
	repo := &stubOverviewRepo{
		summary: &attendance.SummarySection{KPIs: attendance.OverviewKPIs{Engaged: 9}},
		team:    &attendance.TeamSection{ByMember: []attendance.MemberRow{{ActorID: "u1", ActorKind: attendance.ActorKindHuman}}},
		backlog: attendance.BacklogXray{Total: 4, Available: true},
		stages:  attendance.OverviewStages{StagedEngaged: 3, Available: true},
		rework:  attendance.OverviewRework{Available: true},
	}
	uc, _, _ := cachedUseCase(repo, &stubTargetRepo{})

	out, err := uc.Execute("ws1", attendance.OverviewFilter{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.KPIs.Engaged != 9 || len(out.ByMember) != 1 || out.BacklogXray.Total != 4 ||
		out.Stages.StagedEngaged != 3 || !out.Rework.Available || !out.Trend.Available {
		t.Fatalf("Execute() lost a section: %+v", out)
	}
}

type countingMemo struct {
	memoryMemo
	calls int
}

func (m *countingMemo) Remember(
	ctx context.Context,
	key string,
	ttl time.Duration,
	compute func(context.Context) ([]byte, error),
) ([]byte, error) {
	m.calls++
	return m.memoryMemo.Remember(ctx, key, ttl, compute)
}

func TestLiveIsNeverServedFromTheCache(t *testing.T) {
	repo := &stubOverviewRepo{}
	memo := &countingMemo{memoryMemo: memoryMemo{values: map[string][]byte{}}}
	uc := overviewWith(repo, businessConfig(), nil, &stubTargetRepo{}, SectionCaching{Memo: memo, Gate: &trackingGate{}, Versions: newMemoryVersions(), TTL: time.Minute})

	out, err := uc.Live(context.Background(), "ws1", attendance.OverviewFilter{})
	if err != nil {
		t.Fatalf("Live() error = %v", err)
	}
	if out == nil {
		t.Fatalf("Live() returned nothing")
	}
	if memo.calls != 0 {
		t.Fatalf("Live() went through the cache %d times; presence must always be current", memo.calls)
	}
	if len(repo.reads) != 0 {
		t.Fatalf("Live() read overview sections %v; it must never build the scope", repo.reads)
	}
}

func sameShare(t *testing.T, repo *stubOverviewRepo, sections ...string) {
	t.Helper()
	want := 0
	for _, section := range sections {
		shares := repo.sharesOf(section)
		if len(shares) != 1 || shares[0] == 0 {
			t.Fatalf("%s reads ran in shared scopes %v, want exactly one shared scope", section, shares)
		}
		if want == 0 {
			want = shares[0]
		}
		if shares[0] != want {
			t.Fatalf("%s read the scope of request %d, want %d: one request builds its scope once", section, shares[0], want)
		}
	}
}

func TestADependentSectionReadsItsSummaryFromTheSameScope(t *testing.T) {
	for _, section := range []string{"team", "trend"} {
		t.Run(section, func(t *testing.T) {
			repo := &stubOverviewRepo{}
			uc, _, gate := cachedUseCase(repo, &stubTargetRepo{})
			ctx := context.Background()

			var err error
			if section == "team" {
				_, err = uc.Team(ctx, "ws1", attendance.OverviewFilter{})
			} else {
				_, err = uc.Trend(ctx, "ws1", attendance.OverviewFilter{})
			}
			if err != nil {
				t.Fatalf("%s() error = %v", section, err)
			}
			sameShare(t, repo, "summary", section)
			if gate.acquisitions() != 1 {
				t.Fatalf("the request took %d slots, want 1 held from the summary to its own rows", gate.acquisitions())
			}
			if gate.holding() != 0 {
				t.Fatalf("%d slots were never released", gate.holding())
			}
		})
	}
}

func TestADependentSectionWithACachedSummaryTakesItsOwnSlot(t *testing.T) {
	repo := &stubOverviewRepo{}
	uc, _, gate := cachedUseCase(repo, &stubTargetRepo{})
	ctx := context.Background()

	if _, err := uc.Summary(ctx, "ws1", attendance.OverviewFilter{}); err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if _, err := uc.Team(ctx, "ws1", attendance.OverviewFilter{}); err != nil {
		t.Fatalf("Team() error = %v", err)
	}
	if repo.calls("summary") != 1 || repo.calls("team") != 1 {
		t.Fatalf("reads = %d summary / %d team, want 1 each", repo.calls("summary"), repo.calls("team"))
	}
	if gate.acquisitions() != 2 || gate.holding() != 0 {
		t.Fatalf("slots taken = %d, still held = %d, want 2 and 0", gate.acquisitions(), gate.holding())
	}
}

func TestExecuteBuildsOneScopeForTheWholeReport(t *testing.T) {
	repo := &stubOverviewRepo{}
	uc, _, gate := cachedUseCase(repo, &stubTargetRepo{})

	if _, err := uc.Execute("ws1", attendance.OverviewFilter{}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	sameShare(t, repo, "summary", "trend", "team", "stages", "backlog", "rework")
	if gate.acquisitions() != 1 || gate.holding() != 0 {
		t.Fatalf("slots taken = %d, still held = %d, want 1 and 0", gate.acquisitions(), gate.holding())
	}
}

func TestABusySlotStopsASharedRequestBeforeItReads(t *testing.T) {
	repo := &stubOverviewRepo{}
	uc, _, gate := cachedUseCase(repo, &stubTargetRepo{})
	gate.busyErr = cache.ErrGateBusy

	if _, err := uc.Team(context.Background(), "ws1", attendance.OverviewFilter{}); !errors.Is(err, cache.ErrGateBusy) {
		t.Fatalf("Team() error = %v, want ErrGateBusy", err)
	}
	if _, err := uc.Execute("ws1", attendance.OverviewFilter{}); !errors.Is(err, cache.ErrGateBusy) {
		t.Fatalf("Execute() error = %v, want ErrGateBusy", err)
	}
	if len(repo.reads) != 0 {
		t.Fatalf("sections were read although the gate refused: %v", repo.reads)
	}
}

type detachedMemo struct {
	memoryMemo
}

func (m *detachedMemo) Remember(
	ctx context.Context,
	key string,
	ttl time.Duration,
	compute func(context.Context) ([]byte, error),
) ([]byte, error) {
	type result struct {
		value []byte
		err   error
	}
	done := make(chan result, 1)
	go func() {
		value, err := m.memoryMemo.Remember(context.WithoutCancel(ctx), key, ttl, compute)
		done <- result{value: value, err: err}
	}()
	res := <-done
	return res.value, res.err
}

func TestASharedScopeCrossesTheMemosOwnGoroutine(t *testing.T) {
	repo := &stubOverviewRepo{}
	gate := &trackingGate{}
	uc := overviewWith(repo, businessConfig(), nil, &stubTargetRepo{}, SectionCaching{
		Memo: &detachedMemo{memoryMemo: memoryMemo{values: map[string][]byte{}}}, Gate: gate, Versions: newMemoryVersions(), TTL: time.Minute,
	})

	if _, err := uc.Team(context.Background(), "ws1", attendance.OverviewFilter{}); err != nil {
		t.Fatalf("Team() error = %v", err)
	}
	sameShare(t, repo, "summary", "team")
	if gate.acquisitions() != 1 || gate.holding() != 0 {
		t.Fatalf("slots taken = %d, still held = %d, want 1 and 0", gate.acquisitions(), gate.holding())
	}
}
