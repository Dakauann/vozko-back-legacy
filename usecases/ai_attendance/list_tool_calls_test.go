package ai_attendance_usecase

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	aa "vozko/domain/ai_attendance"
	"vozko/domain/conversation"
	"vozko/domain/shared"
)

type toolCallLogSpy struct {
	mu        sync.Mutex
	batches   [][]aa.ToolCall
	recordErr error
}

func (s *toolCallLogSpy) Record(calls []aa.ToolCall) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batches = append(s.batches, calls)
	return s.recordErr
}

func (s *toolCallLogSpy) ListByEntry(workspaceID, entryID, entryType string, p shared.Pagination) (*shared.PaginatedResult[*aa.ToolCall], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := []*aa.ToolCall{}
	for _, batch := range s.batches {
		for i := range batch {
			c := batch[i]
			if c.WorkspaceID == workspaceID && c.EntryID == entryID && c.EntryType == entryType {
				items = append(items, &c)
			}
		}
	}
	return shared.NewPaginatedResult(items, p, int64(len(items))), nil
}

func (s *toolCallLogSpy) DeleteCreatedBefore(time.Time, int) (int64, error) { return 0, nil }

func visibleConversationCalls() *toolCallLogSpy {
	return &toolCallLogSpy{batches: [][]aa.ToolCall{{
		{ID: "1", WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: "instagram", ToolName: "http_request"},
		{ID: "2", WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: "instagram", ToolName: "transfer_to_human"},
		{ID: "3", WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: "instagram", ToolName: "route_to_sales"},
	}}}
}

func TestToolCallsAreListedForAConversationTheCallerCanSee(t *testing.T) {
	log := visibleConversationCalls()
	person := shared.Person{UserID: "u-1"}
	cases := []struct {
		name    string
		rule    conversationRule
		visible bool
	}{
		{"visible", conversationRule{departments: []string{"dept-sales"}, department: "dept-sales"}, true},
		{"hidden by department", conversationRule{departments: []string{"dept-support"}, viewOthers: true, department: "dept-sales"}, false},
		{"hidden by ownership without view_others", conversationRule{department: "dept-sales", assignee: "u-2"}, false},
		{"hidden when the lookup fails", conversationRule{err: errors.New("department scope unreadable")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, err := NewListToolCallsUseCase(log, tc.rule).Execute(person, "ws-1", "instagram", "entry-1", shared.Pagination{Page: 1, PageSize: 20})
			if tc.visible && (err != nil || len(page.Items) != 3) {
				t.Fatalf("page = %+v err = %v", page, err)
			}
			if !tc.visible && (!errors.Is(err, ErrConversationNotVisible) || page != nil) {
				t.Fatalf("a conversation the caller cannot see must be refused, got %+v, %v", page, err)
			}
		})
	}
	if _, err := NewListToolCallsUseCase(log, conversationRule{}).Execute(person, "", "instagram", "entry-1", shared.Pagination{}); !errors.Is(err, ErrConversationNotVisible) {
		t.Fatalf("no workspace must be refused, got %v", err)
	}
}

type conversationRule struct {
	departments []string
	viewOthers  bool
	department  string
	assignee    string
	err         error
}

func (r conversationRule) CanView(viewer shared.Person, workspaceID, _, _ string) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	if workspaceID == "" {
		return false, nil
	}
	if r.assignee != "" && r.assignee != viewer.UserID && !r.viewOthers {
		return false, nil
	}
	return r.departments == nil || slices.Contains(r.departments, r.department) || r.assignee == viewer.UserID, nil
}

func (r conversationRule) Scope(shared.Person, string) (conversation.VisibilityScope, error) {
	return conversation.VisibilityScope{}, r.err
}

