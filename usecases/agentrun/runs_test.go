package agentrun_usecase

import (
	"errors"
	"testing"
	"time"

	"vozko/domain/agentrun"
	aa "vozko/domain/ai_attendance"
	"vozko/domain/conversation"
	"vozko/domain/shared"
)

type runStore struct {
	agentrun.Repository
	detail    *agentrun.Detail
	listed    conversation.VisibilityScope
	listCalls int
}

func (s *runStore) List(_, _ string, _ agentrun.Filter, visibility conversation.VisibilityScope, p shared.Pagination) (*shared.PaginatedResult[*agentrun.Run], error) {
	s.listCalls++
	s.listed = visibility
	return shared.NewPaginatedResult([]*agentrun.Run{}, p, 0), nil
}

func (s *runStore) Find(_, _, _ string) (*agentrun.Detail, error) {
	if s.detail == nil {
		return nil, agentrun.ErrRunNotFound
	}
	return s.detail, nil
}

type visibilityRule struct {
	visible  bool
	scope    conversation.VisibilityScope
	scopeErr error
	viewErr  error
	asked    int
}

func (v *visibilityRule) CanView(shared.Person, string, string, string) (bool, error) {
	v.asked++
	return v.visible, v.viewErr
}

func (v *visibilityRule) Scope(shared.Person, string) (conversation.VisibilityScope, error) {
	return v.scope, v.scopeErr
}

var viewer = shared.Person{UserID: "u-1"}

func TestTheRunListUsesTheViewersConversationScope(t *testing.T) {
	store := &runStore{}
	scope := conversation.VisibilityScope{ReadScope: conversation.ReadScope{DepartmentIDs: []string{"d-1"}, RestrictDepartments: true}}
	runs := NewRuns(store, &visibilityRule{scope: scope})
	if _, err := runs.List(viewer, "ws-1", "ag-1", agentrun.Filter{}, shared.Pagination{}); err != nil {
		t.Fatal(err)
	}
	if !store.listed.RestrictDepartments || store.listed.DepartmentIDs[0] != "d-1" {
		t.Fatalf("scope %+v", store.listed)
	}
}

func TestAnUnreadableScopeOrABadFilterListsNothing(t *testing.T) {
	store := &runStore{}
	if _, err := NewRuns(store, &visibilityRule{scopeErr: errors.New("db down")}).List(viewer, "ws-1", "ag-1", agentrun.Filter{}, shared.Pagination{}); err == nil {
		t.Fatal("an unreadable scope must fail closed")
	}
	if _, err := NewRuns(store, &visibilityRule{}).List(viewer, "ws-1", "ag-1", agentrun.Filter{Source: "x"}, shared.Pagination{}); !errors.Is(err, agentrun.ErrFilterInvalid) {
		t.Fatalf("err %v", err)
	}
	if store.listCalls != 0 {
		t.Fatalf("listed %d times", store.listCalls)
	}
}

func TestARunOfAConversationTheViewerCannotSeeIsNotFound(t *testing.T) {
	detail := &agentrun.Detail{Run: agentrun.Run{ID: "r-1", EntryID: "e-1", EntryType: "whatsapp"}, Spans: []aa.ToolCall{}}
	if _, err := NewRuns(&runStore{detail: detail}, &visibilityRule{visible: false}).Get(viewer, "ws-1", "ag-1", "r-1"); !errors.Is(err, agentrun.ErrRunNotFound) {
		t.Fatalf("err %v", err)
	}
	if _, err := NewRuns(&runStore{detail: detail}, &visibilityRule{viewErr: errors.New("down")}).Get(viewer, "ws-1", "ag-1", "r-1"); !errors.Is(err, agentrun.ErrRunNotFound) {
		t.Fatalf("an unreadable rule must fail closed: %v", err)
	}
	got, err := NewRuns(&runStore{detail: detail}, &visibilityRule{visible: true}).Get(viewer, "ws-1", "ag-1", "r-1")
	if err != nil || got.Run.ID != "r-1" {
		t.Fatalf("got %+v %v", got, err)
	}
}

func TestARunWithoutAConversationNeedsNoConversationCheck(t *testing.T) {
	rule := &visibilityRule{}
	simulated := &agentrun.Detail{Run: agentrun.Run{ID: "r-2", Source: agentrun.SourceSimulator, StartedAt: time.Now()}}
	if _, err := NewRuns(&runStore{detail: simulated}, rule).Get(viewer, "ws-1", "ag-1", "r-2"); err != nil || rule.asked != 0 {
		t.Fatalf("err %v asked %d", err, rule.asked)
	}
}
