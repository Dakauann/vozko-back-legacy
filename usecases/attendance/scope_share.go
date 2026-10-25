package attendance_usecase

import (
	"context"
	"sync"

	"vozko/domain/cache"
)

type scopeShareKey struct{}

type scopeShare struct {
	mu      sync.Mutex
	gate    cache.Gate
	release func()
	ended   bool
}

func (uc *getOverviewUseCase) shareScope(ctx context.Context, fn func(context.Context) error) error {
	if _, shared := ctx.Value(scopeShareKey{}).(*scopeShare); shared {
		return fn(ctx)
	}
	share := &scopeShare{gate: uc.gate}
	defer share.end()
	return uc.repo.ShareScope(context.WithValue(ctx, scopeShareKey{}, share), fn)
}

func (uc *getOverviewUseCase) gated(ctx context.Context, fn func(context.Context) error) error {
	share, shared := ctx.Value(scopeShareKey{}).(*scopeShare)
	if !shared {
		return cache.Gated(ctx, uc.gate, fn)
	}
	held, err := share.hold(ctx)
	if err != nil {
		return err
	}
	if !held {
		return cache.Gated(ctx, uc.gate, fn)
	}
	return fn(ctx)
}

func (s *scopeShare) hold(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return false, nil
	}
	if s.release != nil || s.gate == nil {
		return true, nil
	}
	release, err := s.gate.Acquire(ctx)
	if err != nil {
		return false, err
	}
	s.release = release
	return true, nil
}

func (s *scopeShare) end() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended = true
	if s.release != nil {
		s.release()
		s.release = nil
	}
}
