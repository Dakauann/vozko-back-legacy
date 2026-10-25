package cron

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

type Run func(ctx context.Context) error

type Job struct {
	Name         string
	Period       time.Duration
	EveryReplica bool
}

type Lease interface {
	SetNX(key, value string, ttl time.Duration) (bool, error)
	Del(keys ...string) error
}

var (
	ErrNoLease       = errors.New("cron: no lease to coordinate replicas")
	ErrUnboundJob    = errors.New("cron: a catalog job has no run")
	ErrUnknownJob    = errors.New("cron: a run has no catalog job")
	ErrNilRun        = errors.New("cron: a job is bound to nothing")
	ErrDuplicateJob  = errors.New("cron: a job is in the catalog twice")
	ErrInvalidPeriod = errors.New("cron: a job period must be positive")
)

const (
	maxLockTTL       = 2 * time.Hour
	maxCheckInterval = time.Minute
	minCheckInterval = time.Second
)

type boundJob struct {
	Job
	run Run
}

type Registry struct {
	lease Lease
	jobs  []boundJob
}

func NewRegistry(lease Lease, catalog []Job, bindings map[string]Run) (*Registry, error) {
	var problems []error
	if lease == nil {
		problems = append(problems, ErrNoLease)
	}
	seen := map[string]bool{}
	jobs := make([]boundJob, 0, len(catalog))
	for _, job := range catalog {
		if seen[job.Name] {
			problems = append(problems, fmt.Errorf("%w: %s", ErrDuplicateJob, job.Name))
			continue
		}
		seen[job.Name] = true
		if job.Period <= 0 {
			problems = append(problems, fmt.Errorf("%w: %s", ErrInvalidPeriod, job.Name))
		}
		run, bound := bindings[job.Name]
		switch {
		case !bound:
			problems = append(problems, fmt.Errorf("%w: %s", ErrUnboundJob, job.Name))
		case run == nil:
			problems = append(problems, fmt.Errorf("%w: %s", ErrNilRun, job.Name))
		default:
			jobs = append(jobs, boundJob{Job: job, run: run})
		}
	}
	var extra []string
	for name := range bindings {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		problems = append(problems, fmt.Errorf("%w: %s", ErrUnknownJob, name))
	}
	if len(problems) > 0 {
		return nil, errors.Join(problems...)
	}
	return &Registry{lease: lease, jobs: jobs}, nil
}

func (r *Registry) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, job := range r.jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.loop(ctx, job)
		}()
	}
	wg.Wait()
}

func (r *Registry) loop(ctx context.Context, job boundJob) {
	r.runIfDue(ctx, job)
	ticker := time.NewTicker(checkInterval(job.Job))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runIfDue(ctx, job)
		}
	}
}

func (r *Registry) runIfDue(ctx context.Context, job boundJob) bool {
	if job.EveryReplica {
		r.execute(ctx, job)
		return true
	}
	lockKey := "lock:cron:" + job.Name
	locked, err := r.lease.SetNX(lockKey, "1", lockTTL(job.Period))
	if err != nil {
		log.Printf("[cron] %s skipped, the run lock could not be taken: %v", job.Name, err)
		return false
	}
	if !locked {
		return false
	}
	defer r.release(job.Name, lockKey)
	claimed, err := r.lease.SetNX("cron:slot:"+job.Name, "1", job.Period)
	if err != nil {
		log.Printf("[cron] %s skipped, the period slot could not be claimed: %v", job.Name, err)
		return false
	}
	if !claimed {
		return false
	}
	r.execute(ctx, job)
	return true
}

func (r *Registry) execute(ctx context.Context, job boundJob) {
	if err := job.run(ctx); err != nil {
		log.Printf("[cron] %s error: %v", job.Name, err)
	}
}

func (r *Registry) release(name, lockKey string) {
	if err := r.lease.Del(lockKey); err != nil {
		log.Printf("[cron] %s run lock release error, it expires in %v: %v", name, maxLockTTL, err)
	}
}

func lockTTL(period time.Duration) time.Duration {
	return min(2*period, maxLockTTL)
}

func checkInterval(job Job) time.Duration {
	if job.EveryReplica {
		return job.Period
	}
	return max(min(job.Period/4, maxCheckInterval), minCheckInterval)
}
