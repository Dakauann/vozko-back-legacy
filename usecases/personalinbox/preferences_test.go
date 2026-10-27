package personalinbox_usecase

import (
	"context"
	"errors"
	"sync"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/personalinbox"
	"vozko/domain/shared"
)

type preferencesKey struct {
	workspace string
	user      string
}

type memoryPreferences struct {
	mu     sync.Mutex
	stored map[preferencesKey]personalinbox.Preferences
	broken error
}

func newMemoryPreferences() *memoryPreferences {
	return &memoryPreferences{stored: map[preferencesKey]personalinbox.Preferences{}}
}

func (m *memoryPreferences) FindPreferences(_ context.Context, workspaceID, userID string) (personalinbox.Preferences, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.broken != nil {
		return personalinbox.Preferences{HideGroups: true}, m.broken
	}
	if prefs, ok := m.stored[preferencesKey{workspaceID, userID}]; ok {
		return prefs, nil
	}
	return personalinbox.DefaultPreferences(), nil
}

func (m *memoryPreferences) SavePreferences(_ context.Context, workspaceID, userID string, prefs personalinbox.Preferences) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.broken != nil {
		return m.broken
	}
	m.stored[preferencesKey{workspaceID, userID}] = prefs
	return nil
}

func TestTheInboxPreferencesStartAtTheDefault(t *testing.T) {
	f := newFixture(t)
	if got := f.service.InboxPreferences(context.Background(), homeWorkspace, "ana"); got != personalinbox.DefaultPreferences() {
		t.Fatalf("unsaved preferences = %+v, want the default", got)
	}
}

func TestAnUnreadablePreferenceFallsBackToTheDefault(t *testing.T) {
	f := newFixture(t)
	f.preferences.broken = errors.New("db down")
	if got := f.service.InboxPreferences(context.Background(), homeWorkspace, "ana"); got != personalinbox.DefaultPreferences() {
		t.Fatalf("an unreadable preference must read as the default, got %+v", got)
	}
}

func TestHidingGroupsIsPersonalAndAnnouncedToThatPersonOnly(t *testing.T) {
	f := newFixture(t)
	saved, err := f.service.SaveInboxPreferences(context.Background(), ana, homeWorkspace, personalinbox.Preferences{HideGroups: true})
	if err != nil {
		t.Fatalf("SaveInboxPreferences: %v", err)
	}
	if !saved.HideGroups {
		t.Fatalf("saved = %+v", saved)
	}
	if !f.service.InboxPreferences(context.Background(), homeWorkspace, "ana").HideGroups {
		t.Fatal("ana's choice was not stored")
	}
	if f.service.InboxPreferences(context.Background(), homeWorkspace, "bia").HideGroups {
		t.Fatal("ana's choice leaked to bia")
	}
	if f.service.InboxPreferences(context.Background(), "ws-2", "ana").HideGroups {
		t.Fatal("ana's choice leaked to another workspace")
	}
	sent := f.announcer.preferencesTo("ana")
	if len(sent) != 1 || sent[0].workspaceID != homeWorkspace || !sent[0].prefs.HideGroups {
		t.Fatalf("announced = %+v", sent)
	}
}

func TestSavingPreferencesNeedsInboxAccess(t *testing.T) {
	f := newFixture(t)
	outsider := shared.Person{UserID: "eve"}
	if _, err := f.service.SaveInboxPreferences(context.Background(), outsider, homeWorkspace, personalinbox.Preferences{HideGroups: true}); !errors.Is(err, conversation.ErrUnauthorized) {
		t.Fatalf("err = %v, want unauthorized", err)
	}
	if _, err := f.service.SaveInboxPreferences(context.Background(), ana, "", personalinbox.Preferences{}); !errors.Is(err, personalinbox.ErrWorkspaceRequired) {
		t.Fatalf("err = %v, want workspace required", err)
	}
	if len(f.announcer.preferencesTo("eve")) != 0 || len(f.announcer.preferencesTo("ana")) != 0 {
		t.Fatal("a refused save must not be announced")
	}
}

func TestAFailedSaveIsReportedAndNotAnnounced(t *testing.T) {
	f := newFixture(t)
	f.preferences.broken = errors.New("db down")
	if _, err := f.service.SaveInboxPreferences(context.Background(), ana, homeWorkspace, personalinbox.Preferences{HideGroups: true}); err == nil {
		t.Fatal("a failed save must be reported")
	}
	if len(f.announcer.preferencesTo("ana")) != 0 {
		t.Fatal("a failed save must not be announced")
	}
}
