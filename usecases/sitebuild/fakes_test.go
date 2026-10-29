package sitebuild

import (
	"context"
	"errors"
	"sync"
	"time"

	"vozko/domain/site"
	"vozko/domain/webchat"
)

type memoryCache struct {
	mu      sync.Mutex
	items   map[string]any
	ttls    map[string]time.Duration
	failSet error
	failGet error
}

func newMemoryCache() *memoryCache {
	return &memoryCache{items: map[string]any{}, ttls: map[string]time.Duration{}}
}

func (m *memoryCache) Set(key string, value any, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failSet != nil {
		return m.failSet
	}
	if b, ok := value.([]byte); ok {
		value = string(b)
	}
	m.items[key] = value
	m.ttls[key] = ttl
	return nil
}

func (m *memoryCache) Get(key string) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failGet != nil {
		return nil, m.failGet
	}
	value, ok := m.items[key]
	if !ok {
		return nil, nil
	}
	return value, nil
}

func (m *memoryCache) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
	return nil
}

type fakeBuilder struct {
	mu       sync.Mutex
	builds   []BuildRequest
	executes []ExecuteRequest
	extracts []ExtractRequest
	result   BuildResult
	execute  ExecuteResult
	extract  ExtractResult
	err      error
}

func (f *fakeBuilder) Extract(_ context.Context, req ExtractRequest) (ExtractResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.extracts = append(f.extracts, req)
	return f.extract, f.err
}

func (f *fakeBuilder) Execute(_ context.Context, req ExecuteRequest) (ExecuteResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.executes = append(f.executes, req)
	return f.execute, f.err
}

func (f *fakeBuilder) Build(_ context.Context, req BuildRequest) (BuildResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.builds = append(f.builds, req)
	if f.err != nil {
		return BuildResult{}, f.err
	}
	return f.result, nil
}

type fakeMedia struct {
	urls  map[string]string
	err   error
	calls [][]string
}

func (f *fakeMedia) Locate(_ context.Context, workspaceID string, ids []string) (map[string]string, error) {
	f.calls = append(f.calls, append([]string{workspaceID}, ids...))
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]string{}
	for _, id := range ids {
		if url, ok := f.urls[id]; ok {
			out[id] = url
		}
	}
	return out, nil
}

type fakeSections map[string]string

func (f fakeSections) Section(_ context.Context, _ string, stack site.Stack, key string) (Section, bool, error) {
	html, ok := f[string(stack)+"/"+key]
	return Section{Stack: site.StackHTML, Source: html}, ok, nil
}

type fakeWhatsApp struct {
	numbers map[string]string
	calls   []string
}

func (f *fakeWhatsApp) DirectLink(_ context.Context, workspaceID, numberRef, text string) (string, error) {
	f.calls = append(f.calls, workspaceID+"|"+numberRef+"|"+text)
	digits, ok := f.numbers[numberRef]
	if !ok {
		return "", errors.New("unknown number")
	}
	return "https://wa.me/" + digits + "?text=" + text, nil
}

type fakeWorkspaceNames map[string]string

func (f fakeWorkspaceNames) WorkspaceName(_ context.Context, workspaceID string) (string, error) {
	name, ok := f[workspaceID]
	if !ok {
		return "", errors.New("no workspace")
	}
	return name, nil
}

var errBoom = errors.New("boom")

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

type fakeWidgets struct {
	rows []webchat.Widget
	err  error
}

func (f *fakeWidgets) FindByPublicKey(_ context.Context, key string) (*webchat.Widget, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, w := range f.rows {
		if w.PublicKey == key {
			return &w, nil
		}
	}
	return nil, webchat.ErrWidgetNotFound
}

func (f *fakeWidgets) FindByID(_ context.Context, workspaceID, id string) (*webchat.Widget, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, w := range f.rows {
		if w.ID == id && w.WorkspaceID == workspaceID {
			return &w, nil
		}
	}
	return nil, webchat.ErrWidgetNotFound
}
