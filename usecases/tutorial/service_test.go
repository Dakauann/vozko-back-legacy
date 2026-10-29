package tutorial_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/tutorial"
)

var now = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

type memoryProgress struct {
	rows    map[string]map[string]tutorial.Progress
	failing error
}

func newMemory() *memoryProgress {
	return &memoryProgress{rows: map[string]map[string]tutorial.Progress{}}
}

func (m *memoryProgress) List(_ context.Context, userID string) ([]tutorial.Progress, error) {
	if m.failing != nil {
		return nil, m.failing
	}
	out := []tutorial.Progress{}
	for _, p := range m.rows[userID] {
		out = append(out, p)
	}
	return out, nil
}

func (m *memoryProgress) Find(_ context.Context, userID, id string) (tutorial.Progress, bool, error) {
	if m.failing != nil {
		return tutorial.Progress{}, false, m.failing
	}
	p, ok := m.rows[userID][id]
	return p, ok, nil
}

func (m *memoryProgress) Save(_ context.Context, userID string, p tutorial.Progress) error {
	if m.failing != nil {
		return m.failing
	}
	if m.rows[userID] == nil {
		m.rows[userID] = map[string]tutorial.Progress{}
	}
	m.rows[userID][p.TutorialID] = p
	return nil
}

func (m *memoryProgress) Delete(_ context.Context, userID, id string) error {
	if m.failing != nil {
		return m.failing
	}
	delete(m.rows[userID], id)
	return nil
}

type factsStub struct {
	facts tutorial.SiteFacts
	err   error
	asked string
}

func (f *factsStub) SiteFacts(_ context.Context, workspaceID string) (tutorial.SiteFacts, error) {
	f.asked = workspaceID
	return f.facts, f.err
}

func service(t *testing.T, progress *memoryProgress, facts *factsStub) *Service {
	t.Helper()
	s, err := New(Deps{Progress: progress, SiteFacts: facts, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewRequiresEveryDependency(t *testing.T) {
	if _, err := New(Deps{SiteFacts: &factsStub{}}); err == nil {
		t.Fatal("missing progress must fail")
	}
	if _, err := New(Deps{Progress: newMemory()}); err == nil {
		t.Fatal("missing site facts must fail")
	}
}

func TestSavingProgressKeepsCompletionUntilReset(t *testing.T) {
	ctx := context.Background()
	s := service(t, newMemory(), &factsStub{})

	done, err := s.SaveProgress(ctx, "u1", Change{TutorialID: "sites.editor", Status: tutorial.StatusCompleted, Step: 7})
	if err != nil || done.Status != tutorial.StatusCompleted || !done.UpdatedAt.Equal(now) {
		t.Fatalf("done = %+v, %v", done, err)
	}
	again, err := s.SaveProgress(ctx, "u1", Change{TutorialID: "sites.editor", Status: tutorial.StatusInProgress, Step: 1})
	if err != nil || again.Status != tutorial.StatusCompleted || again.Step != 7 {
		t.Fatalf("again = %+v, %v; completion must stay", again, err)
	}
	if err := s.ResetProgress(ctx, "u1", "sites.editor"); err != nil {
		t.Fatal(err)
	}
	listed, err := s.Progress(ctx, "u1")
	if err != nil || len(listed) != 0 {
		t.Fatalf("after reset = %+v, %v", listed, err)
	}
	restarted, _ := s.SaveProgress(ctx, "u1", Change{TutorialID: "sites.editor", Status: tutorial.StatusInProgress, Step: 0})
	if restarted.Status != tutorial.StatusInProgress {
		t.Fatalf("restarted = %+v", restarted)
	}
}

func TestProgressBelongsToAPerson(t *testing.T) {
	ctx := context.Background()
	s := service(t, newMemory(), &factsStub{})
	if _, err := s.Progress(ctx, " "); !errors.Is(err, tutorial.ErrUserRequired) {
		t.Fatalf("list without person = %v", err)
	}
	if _, err := s.SaveProgress(ctx, "", Change{TutorialID: "sites.editor", Status: tutorial.StatusCompleted}); !errors.Is(err, tutorial.ErrUserRequired) {
		t.Fatalf("save without person = %v", err)
	}
	if err := s.ResetProgress(ctx, "", "sites.editor"); !errors.Is(err, tutorial.ErrUserRequired) {
		t.Fatalf("reset without person = %v", err)
	}
	if err := s.ResetProgress(ctx, "u1", "nope"); !errors.Is(err, tutorial.ErrInvalidID) {
		t.Fatalf("reset with bad id = %v", err)
	}
	if _, err := s.SaveProgress(ctx, "u1", Change{TutorialID: "sites.editor", Status: "maybe"}); !errors.Is(err, tutorial.ErrInvalidProgress) {
		t.Fatalf("bad status = %v", err)
	}
}

func TestStorageFailuresReachTheCaller(t *testing.T) {
	ctx := context.Background()
	store := newMemory()
	store.failing = errors.New("db down")
	s := service(t, store, &factsStub{})
	if _, err := s.Progress(ctx, "u1"); err == nil {
		t.Fatal("an unreadable progress must not look empty")
	}
	if _, err := s.SaveProgress(ctx, "u1", Change{TutorialID: "sites.editor", Status: tutorial.StatusCompleted}); err == nil {
		t.Fatal("a failed save must be reported")
	}
}

func TestTheSitesChecklistComesFromTheWorkspaceFacts(t *testing.T) {
	ctx := context.Background()
	facts := &factsStub{facts: tutorial.SiteFacts{HasPage: true, Published: true}}
	s := service(t, newMemory(), facts)
	list, err := s.SitesChecklist(ctx, "ws-1")
	if err != nil || list.DoneCount() != 2 || facts.asked != "ws-1" {
		t.Fatalf("list = %+v, %v, asked %q", list, err, facts.asked)
	}
	if _, err := s.SitesChecklist(ctx, ""); !errors.Is(err, tutorial.ErrWorkspaceRequired) {
		t.Fatalf("no workspace = %v", err)
	}
	facts.err = errors.New("db down")
	if _, err := s.SitesChecklist(ctx, "ws-1"); err == nil {
		t.Fatal("unreadable facts must not look like an empty workspace")
	}
}
