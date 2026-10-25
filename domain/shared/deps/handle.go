package deps

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	ErrUnbound      = errors.New("dependency not bound yet")
	ErrNilBinding   = errors.New("a handle cannot be bound to nil")
	ErrAlreadyBound = errors.New("a handle can be bound only once")
)

type UnboundError struct {
	Names []string
}

func (e *UnboundError) Error() string {
	return ErrUnbound.Error() + ": " + strings.Join(e.Names, ", ")
}

func (e *UnboundError) Unwrap() error { return ErrUnbound }

type Handle[T any] struct {
	name   string
	mu     sync.RWMutex
	target T
	bound  bool
}

func NewHandle[T any](name string) *Handle[T] {
	return &Handle[T]{name: name}
}

func (h *Handle[T]) Name() string { return h.name }

func (h *Handle[T]) Bind(target T) error {
	if IsNil(target) {
		return fmt.Errorf("%s: %w", h.name, ErrNilBinding)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.bound {
		return fmt.Errorf("%s: %w", h.name, ErrAlreadyBound)
	}
	h.target = target
	h.bound = true
	return nil
}

func (h *Handle[T]) Get() (T, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.bound {
		var zero T
		return zero, &UnboundError{Names: []string{h.name}}
	}
	return h.target, nil
}

func (h *Handle[T]) Bound() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.bound
}

type Binding interface {
	Name() string
	Bound() bool
}

type Handles struct {
	mu       sync.Mutex
	bindings []Binding
}

func (s *Handles) Track(b Binding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bindings = append(s.bindings, b)
}

func Tracked[T any](s *Handles, name string) *Handle[T] {
	h := NewHandle[T](name)
	s.Track(h)
	return h
}

func (s *Handles) VerifyBound() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var names []string
	for _, b := range s.bindings {
		if !b.Bound() {
			names = append(names, b.Name())
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	return &UnboundError{Names: names}
}
