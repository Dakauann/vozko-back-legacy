package googlesheet

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
)

type memoryEdits struct {
	mu          sync.Mutex
	sheets      *memorySheets
	rows        *memoryRows
	sessions    map[string]*googlesheet.EditSession
	failCurrent error
	ends        int
}

func newMemoryEdits(sheets *memorySheets, rows *memoryRows) *memoryEdits {
	return &memoryEdits{sheets: sheets, rows: rows, sessions: map[string]*googlesheet.EditSession{}}
}

func copySession(s *googlesheet.EditSession) *googlesheet.EditSession {
	c := *s
	c.Check.Tabs = append([]googlesheet.TabCheck(nil), s.Check.Tabs...)
	return &c
}

func (m *memoryEdits) open(workspaceID, spreadsheetID string) *googlesheet.EditSession {
	for _, s := range m.sessions {
		if s.WorkspaceID == workspaceID && s.SpreadsheetID == spreadsheetID && !s.Ended() {
			return s
		}
	}
	return nil
}

func (m *memoryEdits) Start(_ context.Context, session *googlesheet.EditSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if holder := m.open(session.WorkspaceID, session.SpreadsheetID); holder != nil {
		return &googlesheet.EditTakenError{Session: copySession(holder)}
	}
	session.ID = uuid.NewString()
	m.sessions[session.ID] = copySession(session)
	return nil
}

func (m *memoryEdits) Current(_ context.Context, workspaceID, spreadsheetID string) (*googlesheet.EditSession, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failCurrent != nil {
		return nil, false, m.failCurrent
	}
	if holder := m.open(workspaceID, spreadsheetID); holder != nil {
		return copySession(holder), true, nil
	}
	return nil, false, nil
}

func (m *memoryEdits) Find(_ context.Context, workspaceID, id string) (*googlesheet.EditSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok || s.WorkspaceID != workspaceID {
		return nil, googlesheet.ErrEditSessionNotFound
	}
	return copySession(s), nil
}

func (m *memoryEdits) Extend(_ context.Context, session *googlesheet.EditSession, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored := m.sessions[session.ID]
	if stored.Ended() || !stored.ExpiresAt.After(now) {
		return googlesheet.ErrEditSessionEnded
	}
	stored.ExpiresAt = session.ExpiresAt
	return nil
}

func (m *memoryEdits) Settle(_ context.Context, session *googlesheet.EditSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if stored := m.sessions[session.ID]; stored.SettledAt == nil {
		stored.SettledAt = session.SettledAt
	}
	return nil
}

func (m *memoryEdits) End(_ context.Context, session *googlesheet.EditSession) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored := m.sessions[session.ID]
	if stored.Ended() {
		return false, nil
	}
	m.ends++
	waited := 0
	m.sheets.mu.Lock()
	m.rows.mu.Lock()
	for _, row := range m.rows.rows {
		sheet, ok := m.sheets.sheets[row.SheetID]
		if ok && sheet.WorkspaceID == stored.WorkspaceID && sheet.SpreadsheetID == stored.SpreadsheetID && (row.Status == googlesheet.RowPending || row.Status == googlesheet.RowInFlight) {
			waited++
		}
	}
	m.rows.mu.Unlock()
	m.sheets.mu.Unlock()
	stored.EndedAt, stored.EndedBy, stored.EndReason, stored.WaitedRows = session.EndedAt, session.EndedBy, session.EndReason, waited
	session.WaitedRows = waited
	return true, nil
}

func (m *memoryEdits) SaveCheck(_ context.Context, session *googlesheet.EditSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.ID].Check = session.Check
	return nil
}

func (m *memoryEdits) Expired(_ context.Context, now time.Time, limit int) ([]*googlesheet.EditSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*googlesheet.EditSession
	for _, s := range m.sessions {
		if !s.Ended() && !s.ExpiresAt.After(now) && len(out) < limit {
			out = append(out, copySession(s))
		}
	}
	return out, nil
}

func (m *memoryEdits) History(_ context.Context, workspaceID, spreadsheetID string, p shared.Pagination) (*shared.PaginatedResult[*googlesheet.EditSession], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*googlesheet.EditSession
	for _, s := range m.sessions {
		if s.WorkspaceID == workspaceID && s.SpreadsheetID == spreadsheetID {
			out = append(out, copySession(s))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	total := len(out)
	start := min(p.Offset(), total)
	end := min(start+p.PageSize, total)
	return shared.NewPaginatedResult(out[start:end], p, int64(total)), nil
}
