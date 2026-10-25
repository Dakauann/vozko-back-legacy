package attendance_repository

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"gorm.io/gorm"

	"vozko/infra/database"
)

type scopeSessionKey struct{}

type scopeSession struct {
	mu     sync.Mutex
	db     *gorm.DB
	tx     *gorm.DB
	scope  string
	closed bool
}

func (r *repository) ShareScope(ctx context.Context, fn func(context.Context) error) error {
	if _, shared := ctx.Value(scopeSessionKey{}).(*scopeSession); shared {
		return fn(ctx)
	}
	session := &scopeSession{db: r.db.WithContext(context.WithoutCancel(ctx))}
	err := fn(context.WithValue(ctx, scopeSessionKey{}, session))
	return errors.Join(err, session.end(err != nil))
}

func sessionOf(ctx context.Context) *scopeSession {
	session, _ := ctx.Value(scopeSessionKey{}).(*scopeSession)
	return session
}

func scopeKey(selectBody string, args []interface{}) string {
	return selectBody + "\x00" + fmt.Sprint(args...)
}

func (s *scopeSession) run(ctx context.Context, scope string, build, fn func(tx *gorm.DB) error) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || (scope != "" && s.scope != "" && s.scope != scope) {
		return false, nil
	}
	if s.tx == nil {
		tx := s.db.Begin()
		if tx.Error != nil {
			return true, tx.Error
		}
		s.tx = tx
	}
	if scope != "" && s.scope == "" {
		if err := database.InSession(ctx, s.tx, analyticsSettings(), build); err != nil {
			return true, err
		}
		s.scope = scope
	}
	return true, database.InSession(ctx, s.tx, analyticsSettings(), fn)
}

func (s *scopeSession) end(failed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.tx == nil {
		return nil
	}
	if failed {
		return s.tx.Rollback().Error
	}
	return s.tx.Commit().Error
}
