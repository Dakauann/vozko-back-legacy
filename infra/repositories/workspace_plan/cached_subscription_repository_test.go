package workspace_plan_repository

import (
	"errors"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"vozko/domain/cache"
	workspace_plan "vozko/domain/workspace/workspace_plan"
	infracache "vozko/infra/cache"
)

type memoryState struct {
	cache.SharedState
	mu      sync.Mutex
	values  map[string]string
	ttls    map[string]time.Duration
	readErr error
	incrErr error
}

func newMemoryState() *memoryState {
	return &memoryState{values: map[string]string{}, ttls: map[string]time.Duration{}}
}

func (m *memoryState) GetString(key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readErr != nil {
		return "", m.readErr
	}
	return m.values[key], nil
}

func (m *memoryState) SetString(key, value string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = value
	m.ttls[key] = ttl
	return nil
}

func (m *memoryState) Del(keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		delete(m.values, key)
	}
	return nil
}

func (m *memoryState) Incr(key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.incrErr != nil {
		return 0, m.incrErr
	}
	n, _ := strconv.ParseInt(m.values[key], 10, 64)
	n++
	m.values[key] = strconv.FormatInt(n, 10)
	return n, nil
}

type tableSubscriptions struct {
	workspace_plan.SubscriptionRepository
	rows       []*workspace_plan.WorkspaceSubscription
	reads      int
	readErr    error
	writeErr   error
	expiredIDs []string
}

func (s *tableSubscriptions) GetCurrentByWorkspaceID(workspaceID string, at time.Time) (*workspace_plan.WorkspaceSubscription, error) {
	s.reads++
	if s.readErr != nil {
		return nil, s.readErr
	}
	bound := at.Truncate(time.Microsecond)
	var best *workspace_plan.WorkspaceSubscription
	for _, row := range s.rows {
		eligible := row.WorkspaceID == workspaceID &&
			(row.Status == workspace_plan.SubscriptionStatusActive || row.Status == workspace_plan.SubscriptionStatusCancelled) &&
			!row.CurrentPeriodEnd.Before(bound)
		if eligible && (best == nil || row.CurrentPeriodEnd.After(best.CurrentPeriodEnd)) {
			best = row
		}
	}
	if best == nil {
		return nil, workspace_plan.ErrSubscriptionNotCurrent
	}
	copied := *best
	return &copied, nil
}

func (s *tableSubscriptions) Create(sub *workspace_plan.WorkspaceSubscription) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	copied := *sub
	s.rows = append(s.rows, &copied)
	return nil
}

func (s *tableSubscriptions) Update(sub *workspace_plan.WorkspaceSubscription) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	for i, row := range s.rows {
		if row.ID == sub.ID {
			copied := *sub
			s.rows[i] = &copied
		}
	}
	return nil
}

func (s *tableSubscriptions) ExpireOverdue(at time.Time, _ int) ([]string, error) {
	if s.writeErr != nil {
		return nil, s.writeErr
	}
	return s.expiredIDs, nil
}

var periodEnd = time.Date(2026, 11, 23, 23, 59, 59, 0, time.UTC)

func activeSubscription(workspaceID string) *workspace_plan.WorkspaceSubscription {
	cancelled := periodEnd.Add(-48 * time.Hour)
	return &workspace_plan.WorkspaceSubscription{
		ID: "sub-" + workspaceID, WorkspaceID: workspaceID, PlanDefinitionID: "plan-1", PlanName: "Pro", MaxCallChannels: 3,
		BillingCycle: workspace_plan.BillingCycle("monthly"), Status: workspace_plan.SubscriptionStatusCancelled,
		CurrentPeriodStart: periodEnd.AddDate(0, -1, 0), CurrentPeriodEnd: periodEnd, CancelledAt: &cancelled,
		CreatedAt: periodEnd.AddDate(0, -2, 0), UpdatedAt: cancelled,
	}
}

func cachedSubscriptions(inner *tableSubscriptions) (*CachedSubscriptionRepository, *memoryState) {
	state := newMemoryState()
	repo := NewCachedSubscriptionRepository(inner, state, infracache.NewSharedStateVersions(state, SubscriptionCachePrefix))
	return repo.(*CachedSubscriptionRepository), state
}

type readAt struct {
	at        time.Time
	wantInner bool
}

func TestTheCachedSubscriptionAnswersExactlyLikeTheTable(t *testing.T) {
	before := periodEnd.Add(-time.Hour)
	cases := []struct {
		name  string
		rows  []*workspace_plan.WorkspaceSubscription
		reads []readAt
	}{
		{
			name: "current row stays current until its period ends, then expires without a query",
			rows: []*workspace_plan.WorkspaceSubscription{activeSubscription("ws-1")},
			reads: []readAt{
				{at: before, wantInner: true},
				{at: before.Add(30 * time.Minute)},
				{at: periodEnd},
				{at: periodEnd.Add(400 * time.Nanosecond)},
				{at: periodEnd.Add(time.Microsecond)},
				{at: periodEnd.Add(24 * time.Hour)},
				{at: before.Add(-24 * time.Hour)},
			},
		},
		{
			name: "a missing subscription stays missing later and is asked again for an earlier moment",
			rows: []*workspace_plan.WorkspaceSubscription{activeSubscription("ws-1")},
			reads: []readAt{
				{at: periodEnd.Add(time.Hour), wantInner: true},
				{at: periodEnd.Add(2 * time.Hour)},
				{at: periodEnd.Add(time.Hour)},
				{at: before, wantInner: true},
				{at: periodEnd.Add(3 * time.Hour)},
			},
		},
		{
			name:  "no subscription at all",
			reads: []readAt{{at: before, wantInner: true}, {at: before.Add(time.Minute)}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			table := &tableSubscriptions{rows: tc.rows}
			truth := &tableSubscriptions{rows: tc.rows}
			repo, _ := cachedSubscriptions(table)
			for i, read := range tc.reads {
				readsBefore := table.reads
				got, gotErr := repo.GetCurrentByWorkspaceID("ws-1", read.at)
				want, wantErr := truth.GetCurrentByWorkspaceID("ws-1", read.at)
				if !errors.Is(gotErr, wantErr) || !reflect.DeepEqual(got, want) {
					t.Fatalf("read %d at %s = %+v, %v; the table says %+v, %v", i, read.at, got, gotErr, want, wantErr)
				}
				if asked := table.reads > readsBefore; asked != read.wantInner {
					t.Fatalf("read %d at %s asked the table = %v, want %v", i, read.at, asked, read.wantInner)
				}
			}
		})
	}
}

func TestEveryWriteMakesTheNextReadAskTheTable(t *testing.T) {
	at := periodEnd.Add(-time.Hour)
	cases := []struct {
		name  string
		write func(repo *CachedSubscriptionRepository, table *tableSubscriptions) error
		want  func(*workspace_plan.WorkspaceSubscription, error) bool
	}{
		{
			name: "create",
			write: func(repo *CachedSubscriptionRepository, _ *tableSubscriptions) error {
				return repo.Create(activeSubscription("ws-1"))
			},
			want: func(sub *workspace_plan.WorkspaceSubscription, err error) bool {
				return err == nil && sub.PlanName == "Pro"
			},
		},
		{
			name: "update",
			write: func(repo *CachedSubscriptionRepository, table *tableSubscriptions) error {
				table.rows = []*workspace_plan.WorkspaceSubscription{activeSubscription("ws-1")}
				changed := activeSubscription("ws-1")
				changed.PlanName = "Enterprise"
				return repo.Update(changed)
			},
			want: func(sub *workspace_plan.WorkspaceSubscription, err error) bool {
				return err == nil && sub.PlanName == "Enterprise"
			},
		},
		{
			name: "expire overdue",
			write: func(repo *CachedSubscriptionRepository, table *tableSubscriptions) error {
				table.rows = []*workspace_plan.WorkspaceSubscription{activeSubscription("ws-1")}
				table.expiredIDs = []string{"ws-1", "ws-1", "ws-2"}
				_, err := repo.ExpireOverdue(at, 100)
				return err
			},
			want: func(sub *workspace_plan.WorkspaceSubscription, err error) bool { return err == nil && sub != nil },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			table := &tableSubscriptions{}
			repo, _ := cachedSubscriptions(table)
			if _, err := repo.GetCurrentByWorkspaceID("ws-1", at); !errors.Is(err, workspace_plan.ErrSubscriptionNotCurrent) {
				t.Fatalf("first read error = %v, want not current", err)
			}
			if err := tc.write(repo, table); err != nil {
				t.Fatalf("write error = %v", err)
			}
			readsBefore := table.reads
			sub, err := repo.GetCurrentByWorkspaceID("ws-1", at)
			if table.reads != readsBefore+1 {
				t.Fatalf("the read after a write was served from the cache")
			}
			if !tc.want(sub, err) {
				t.Fatalf("read after write = %+v, %v", sub, err)
			}
		})
	}
}

func TestExpireOverdueMovesEveryAffectedWorkspace(t *testing.T) {
	table := &tableSubscriptions{expiredIDs: []string{"ws-1", "ws-2", "ws-1"}}
	repo, state := cachedSubscriptions(table)

	ids, err := repo.ExpireOverdue(periodEnd, 100)
	if err != nil || !reflect.DeepEqual(ids, table.expiredIDs) {
		t.Fatalf("ExpireOverdue() = %v, %v", ids, err)
	}
	for workspaceID, want := range map[string]string{"ws-1": "1", "ws-2": "1"} {
		if got := state.values[SubscriptionCachePrefix+":version:"+workspaceID]; got != want {
			t.Fatalf("version of %s = %q, want %q", workspaceID, got, want)
		}
	}
}

func TestAFailureIsNeverCached(t *testing.T) {
	boom := errors.New("db down")
	table := &tableSubscriptions{readErr: boom}
	repo, _ := cachedSubscriptions(table)

	for range 2 {
		if _, err := repo.GetCurrentByWorkspaceID("ws-1", periodEnd); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want the table failure", err)
		}
	}
	if table.reads != 2 {
		t.Fatalf("table reads = %d, want 2: a failure must never be remembered", table.reads)
	}
}

func TestAnUnreadableCacheAsksTheTable(t *testing.T) {
	table := &tableSubscriptions{rows: []*workspace_plan.WorkspaceSubscription{activeSubscription("ws-1")}}
	repo, state := cachedSubscriptions(table)
	state.readErr = errors.New("redis down")

	for range 2 {
		sub, err := repo.GetCurrentByWorkspaceID("ws-1", periodEnd.Add(-time.Hour))
		if err != nil || sub == nil {
			t.Fatalf("read = %+v, %v", sub, err)
		}
	}
	if table.reads != 2 {
		t.Fatalf("table reads = %d, want 2 while the cache cannot be read", table.reads)
	}
}

func TestAWriteWhoseCacheCannotMoveSaysSo(t *testing.T) {
	redisDown := errors.New("redis down")
	cases := []struct {
		name  string
		write func(repo *CachedSubscriptionRepository) error
	}{
		{name: "create", write: func(repo *CachedSubscriptionRepository) error { return repo.Create(activeSubscription("ws-1")) }},
		{name: "update", write: func(repo *CachedSubscriptionRepository) error { return repo.Update(activeSubscription("ws-1")) }},
		{name: "expire overdue", write: func(repo *CachedSubscriptionRepository) error {
			_, err := repo.ExpireOverdue(periodEnd, 10)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			table := &tableSubscriptions{expiredIDs: []string{"ws-1"}}
			repo, state := cachedSubscriptions(table)
			state.incrErr = redisDown
			if err := tc.write(repo); !errors.Is(err, redisDown) {
				t.Fatalf("write error = %v, want the cache failure", err)
			}
		})
	}
}

func TestAFailedWriteStillMovesTheCache(t *testing.T) {
	boom := errors.New("db down")
	table := &tableSubscriptions{writeErr: boom}
	repo, state := cachedSubscriptions(table)

	if err := repo.Update(activeSubscription("ws-1")); !errors.Is(err, boom) {
		t.Fatalf("Update() error = %v, want the table failure", err)
	}
	if got := state.values[SubscriptionCachePrefix+":version:ws-1"]; got != "1" {
		t.Fatalf("version = %q, want 1: a write that may have landed must not leave the old row cached", got)
	}
}

func TestTheCachedRowKeepsEveryField(t *testing.T) {
	full := activeSubscription("ws-1")
	value := reflect.ValueOf(full).Elem()
	for i := range value.NumField() {
		if value.Field(i).IsZero() {
			t.Fatalf("fixture leaves %s empty, so the round trip cannot prove it survives", value.Type().Field(i).Name)
		}
	}
	table := &tableSubscriptions{rows: []*workspace_plan.WorkspaceSubscription{full}}
	repo, _ := cachedSubscriptions(table)
	at := periodEnd.Add(-time.Hour)

	if _, err := repo.GetCurrentByWorkspaceID("ws-1", at); err != nil {
		t.Fatalf("first read: %v", err)
	}
	cached, err := repo.GetCurrentByWorkspaceID("ws-1", at)
	if err != nil {
		t.Fatalf("cached read: %v", err)
	}
	if table.reads != 1 || !reflect.DeepEqual(cached, full) {
		t.Fatalf("cached row %+v differs from the stored %+v", cached, full)
	}
	cached.PlanName = "mutated"
	again, _ := repo.GetCurrentByWorkspaceID("ws-1", at)
	if again.PlanName != "Pro" {
		t.Fatalf("a caller's change leaked into the cache")
	}
}

func TestTheCacheKeysAreStable(t *testing.T) {
	table := &tableSubscriptions{rows: []*workspace_plan.WorkspaceSubscription{activeSubscription("ws-1")}}
	repo, state := cachedSubscriptions(table)
	if _, err := repo.GetCurrentByWorkspaceID("ws-1", periodEnd.Add(-time.Hour)); err != nil {
		t.Fatalf("read: %v", err)
	}
	key := "ws:subscription:current:ws-1:0"
	if state.values[key] == "" {
		t.Fatalf("expected the row under %q, have %v", key, state.values)
	}
	if state.ttls[key] != subscriptionCacheTTL {
		t.Fatalf("ttl = %s, want %s", state.ttls[key], subscriptionCacheTTL)
	}
	if err := repo.Create(activeSubscription("ws-1")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if state.values["ws:subscription:version:ws-1"] != "1" {
		t.Fatalf("version key = %v", state.values)
	}
}
