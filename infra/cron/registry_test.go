package cron

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeLease struct {
	mu      sync.Mutex
	now     time.Time
	expires map[string]time.Time
	failSet error
}

func newFakeLease() *fakeLease {
	return &fakeLease{now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), expires: map[string]time.Time{}}
}

func (l *fakeLease) SetNX(key, _ string, ttl time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failSet != nil {
		return false, l.failSet
	}
	if until, held := l.expires[key]; held && l.now.Before(until) {
		return false, nil
	}
	l.expires[key] = l.now.Add(ttl)
	return true, nil
}

func (l *fakeLease) Del(keys ...string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, key := range keys {
		delete(l.expires, key)
	}
	return nil
}

func (l *fakeLease) advance(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = l.now.Add(d)
}

func (l *fakeLease) hold(key string, d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.expires[key] = l.now.Add(d)
}

func (l *fakeLease) held(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	until, ok := l.expires[key]
	return ok && l.now.Before(until)
}

type counter struct {
	mu   sync.Mutex
	runs int
	err  error
}

func (c *counter) run(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runs++
	return c.err
}

func (c *counter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runs
}

func oneJob(t *testing.T, lease Lease, job Job, run Run) *Registry {
	t.Helper()
	r, err := NewRegistry(lease, []Job{job}, map[string]Run{job.Name: run})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return r
}

type executable struct{ ran bool }

func (e *executable) Execute(context.Context) error {
	e.ran = true
	return nil
}

func TestNewRegistryRefusesEveryWiringGap(t *testing.T) {
	noop := Run(func(context.Context) error { return nil })
	lease := newFakeLease()
	daily := Job{Name: "daily", Period: 24 * time.Hour}
	cases := map[string]struct {
		lease    Lease
		catalog  []Job
		bindings map[string]Run
		want     error
	}{
		"no lease":           {nil, []Job{daily}, map[string]Run{"daily": noop}, ErrNoLease},
		"job without a run":  {lease, []Job{daily}, map[string]Run{}, ErrUnboundJob},
		"run without a job":  {lease, []Job{daily}, map[string]Run{"daily": noop, "ghost": noop}, ErrUnknownJob},
		"nil run":            {lease, []Job{daily}, map[string]Run{"daily": nil}, ErrNilRun},
		"duplicate job":      {lease, []Job{daily, daily}, map[string]Run{"daily": noop}, ErrDuplicateJob},
		"zero period":        {lease, []Job{{Name: "daily"}}, map[string]Run{"daily": noop}, ErrInvalidPeriod},
		"negative period":    {lease, []Job{{Name: "daily", Period: -time.Second}}, map[string]Run{"daily": noop}, ErrInvalidPeriod},
		"typed nil executes": {lease, []Job{daily}, map[string]Run{"daily": Executes((*executable)(nil))}, ErrNilRun},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r, err := NewRegistry(tc.lease, tc.catalog, tc.bindings)
			if r != nil || !errors.Is(err, tc.want) {
				t.Fatalf("registry = %v err = %v, want %v", r, err, tc.want)
			}
		})
	}
}

func TestARegistryErrorNamesEveryProblemAtOnce(t *testing.T) {
	_, err := NewRegistry(newFakeLease(), []Job{{Name: "a", Period: time.Minute}, {Name: "b", Period: time.Minute}}, map[string]Run{"c": func(context.Context) error { return nil }})
	for _, want := range []error{ErrUnboundJob, ErrUnknownJob} {
		if !errors.Is(err, want) {
			t.Errorf("err = %v, want it to include %v", err, want)
		}
	}
}

func TestAnOverdueJobRunsAtBootAndOncePerPeriod(t *testing.T) {
	lease := newFakeLease()
	c := &counter{}
	r := oneJob(t, lease, Job{Name: "daily", Period: 24 * time.Hour}, c.run)
	job := r.jobs[0]

	if !r.runIfDue(context.Background(), job) || c.count() != 1 {
		t.Fatalf("runs = %d, want the overdue job run at boot", c.count())
	}
	lease.advance(23 * time.Hour)
	if r.runIfDue(context.Background(), job) || c.count() != 1 {
		t.Fatalf("runs = %d, want no second run inside the period, even after a restart", c.count())
	}
	lease.advance(time.Hour)
	if !r.runIfDue(context.Background(), job) || c.count() != 2 {
		t.Fatalf("runs = %d, want the next run once the period is over", c.count())
	}
}

func TestTwoReplicasShareOneRunPerPeriod(t *testing.T) {
	lease := newFakeLease()
	a, b := &counter{}, &counter{}
	ra := oneJob(t, lease, Job{Name: "hourly", Period: time.Hour}, a.run)
	rb := oneJob(t, lease, Job{Name: "hourly", Period: time.Hour}, b.run)

	ra.runIfDue(context.Background(), ra.jobs[0])
	rb.runIfDue(context.Background(), rb.jobs[0])
	if a.count()+b.count() != 1 {
		t.Fatalf("runs = %d + %d, want one run across replicas", a.count(), b.count())
	}
}

func TestTheRunLockIsReleasedAfterEveryRunEvenAFailedOne(t *testing.T) {
	lease := newFakeLease()
	c := &counter{err: errors.New("boom")}
	r := oneJob(t, lease, Job{Name: "minutely", Period: time.Minute}, c.run)

	r.runIfDue(context.Background(), r.jobs[0])
	if lease.held("lock:cron:minutely") {
		t.Fatal("the run lock must be released after the run")
	}
	if !lease.held("cron:slot:minutely") {
		t.Fatal("the period slot must stay claimed until it expires")
	}
}

func TestARunningJobIsNotStartedTwice(t *testing.T) {
	lease := newFakeLease()
	c := &counter{}
	r := oneJob(t, lease, Job{Name: "slow", Period: time.Minute}, c.run)
	lease.hold("lock:cron:slow", time.Minute)

	if r.runIfDue(context.Background(), r.jobs[0]) || c.count() != 0 {
		t.Fatal("a job whose run lock is held must not start again")
	}
	if lease.held("cron:slot:slow") {
		t.Fatal("a skipped run must leave the period slot free for the next check")
	}
}

func TestALeaseOutageSkipsTheRunInsteadOfRunningUnguarded(t *testing.T) {
	lease := newFakeLease()
	lease.failSet = errors.New("redis down")
	c := &counter{}
	r := oneJob(t, lease, Job{Name: "daily", Period: 24 * time.Hour}, c.run)

	if r.runIfDue(context.Background(), r.jobs[0]) || c.count() != 0 {
		t.Fatal("without the lease nothing may run")
	}
}

func TestAnEveryReplicaJobRunsOnEveryCheckWithoutTheLease(t *testing.T) {
	lease := newFakeLease()
	c := &counter{}
	r := oneJob(t, lease, Job{Name: "local", Period: time.Minute, EveryReplica: true}, c.run)

	r.runIfDue(context.Background(), r.jobs[0])
	r.runIfDue(context.Background(), r.jobs[0])
	if c.count() != 2 || len(lease.expires) != 0 {
		t.Fatalf("runs = %d keys = %v", c.count(), lease.expires)
	}
}

func TestLockTTLAndCheckIntervalFollowThePeriod(t *testing.T) {
	cases := []struct {
		period, lock, check time.Duration
	}{
		{10 * time.Second, 20 * time.Second, 2500 * time.Millisecond},
		{time.Minute, 2 * time.Minute, 15 * time.Second},
		{time.Hour, 2 * time.Hour, time.Minute},
		{24 * time.Hour, 2 * time.Hour, time.Minute},
		{2 * time.Second, 4 * time.Second, time.Second},
	}
	for _, tc := range cases {
		if got := lockTTL(tc.period); got != tc.lock {
			t.Errorf("lockTTL(%v) = %v, want %v", tc.period, got, tc.lock)
		}
		if got := checkInterval(Job{Period: tc.period}); got != tc.check {
			t.Errorf("checkInterval(%v) = %v, want %v", tc.period, got, tc.check)
		}
	}
	if got := checkInterval(Job{Period: time.Minute, EveryReplica: true}); got != time.Minute {
		t.Errorf("an every-replica job checks once per period, got %v", got)
	}
}

func TestRunStopsEveryLoopOnShutdown(t *testing.T) {
	lease := newFakeLease()
	c := &counter{}
	r := oneJob(t, lease, Job{Name: "minutely", Period: time.Minute}, c.run)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()
	deadline := time.After(2 * time.Second)
	for c.count() == 0 {
		select {
		case <-deadline:
			t.Fatal("the job never ran at boot")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after shutdown")
	}
}

func TestAdaptersRefuseMissingTargets(t *testing.T) {
	if Executes(nil) != nil || Executes((*executable)(nil)) != nil {
		t.Fatal("Executes must return nil for a missing job")
	}
	if Counted[int]("x %d", nil) != nil || Plain(nil) != nil {
		t.Fatal("Counted and Plain must return nil for a missing func")
	}
	var missing *executable
	if Method(missing, func(context.Context) error { return nil }) != nil {
		t.Fatal("Method must return nil for a missing receiver")
	}
}

func TestAdaptersRunTheirTarget(t *testing.T) {
	e := &executable{}
	if err := Executes(e)(context.Background()); err != nil || !e.ran {
		t.Fatal("Executes must run the job")
	}
	failure := errors.New("down")
	if err := Counted("expired %d", func() (int, error) { return 2, failure })(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("Counted err = %v", err)
	}
	if err := Plain(func() error { return failure })(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("Plain err = %v", err)
	}
}
