package googlesheet

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"vozko/domain/shared"
)

const (
	EditSessionTTL       = 2 * time.Minute
	EditHeartbeatEvery   = 30 * time.Second
	EditSessionMaxLength = 4 * time.Hour
	EmbeddedEditorMode   = "rm=minimal"
)

var (
	ErrFileBeingEdited      = errors.New("googlesheet: someone is editing the google file by hand")
	ErrEditSessionNotFound  = errors.New("googlesheet: edit session not found")
	ErrEditSessionEnded     = errors.New("googlesheet: the edit session has ended")
	ErrEditSessionNotYours  = errors.New("googlesheet: the edit session belongs to another person")
	ErrEditEndReasonUnknown = errors.New("googlesheet: unknown reason to end an edit session")
	errEditOwnerRequired    = errors.New("googlesheet: an edit session needs the workspace, the file and the person")
)

const (
	CodeFileBeingEdited      = "google_sheet_file_being_edited"
	CodeEditSessionNotFound  = "google_sheet_edit_session_not_found"
	CodeEditSessionEnded     = "google_sheet_edit_session_ended"
	CodeEditSessionNotYours  = "google_sheet_edit_session_not_yours"
	CodeEditEndReasonUnknown = "google_sheet_edit_end_reason_unknown"
)

type EditEndReason string

const (
	EditFinished     EditEndReason = "finished"
	EditLeft         EditEndReason = "left"
	EditExpired      EditEndReason = "expired"
	EditEndedByAdmin EditEndReason = "ended_by_admin"
)

func ParseClientEndReason(raw string) (EditEndReason, error) {
	switch reason := EditEndReason(raw); reason {
	case EditFinished, EditLeft:
		return reason, nil
	}
	return "", fmt.Errorf("%w: %q", ErrEditEndReasonUnknown, raw)
}

type EditSession struct {
	ID            string
	WorkspaceID   string
	SpreadsheetID string
	StartedBy     string
	StartedAt     time.Time
	ExpiresAt     time.Time
	SettledAt     *time.Time
	EndedAt       *time.Time
	EndedBy       string
	EndReason     EditEndReason
	WaitedRows    int
	Check         EditCheck
}

func NewEditSession(workspaceID, spreadsheetID, userID string, now time.Time) (*EditSession, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(spreadsheetID) == "" || strings.TrimSpace(userID) == "" {
		return nil, errEditOwnerRequired
	}
	return &EditSession{
		WorkspaceID: workspaceID, SpreadsheetID: spreadsheetID, StartedBy: userID,
		StartedAt: now, ExpiresAt: now.Add(EditSessionTTL),
	}, nil
}

func (s *EditSession) Ended() bool { return s.EndedAt != nil }

func (s *EditSession) Settled() bool { return s.SettledAt != nil }

func (s *EditSession) Deadline() time.Time { return s.StartedAt.Add(EditSessionMaxLength) }

func (s *EditSession) HoldsAt(now time.Time) bool {
	return !s.Ended() && now.Before(s.ExpiresAt)
}

func (s *EditSession) Extend(now time.Time) error {
	if !s.HoldsAt(now) {
		return ErrEditSessionEnded
	}
	next := now.Add(EditSessionTTL)
	if deadline := s.Deadline(); next.After(deadline) {
		next = deadline
	}
	if next.After(s.ExpiresAt) {
		s.ExpiresAt = next
	}
	return nil
}

func (s *EditSession) Settle(at time.Time) {
	if s.SettledAt == nil {
		s.SettledAt = &at
	}
}

type EditEnd struct {
	Reason EditEndReason
	By     string
	At     time.Time
}

func (s *EditSession) End(end EditEnd) error {
	if s.Ended() {
		return ErrEditSessionEnded
	}
	switch end.Reason {
	case EditFinished, EditLeft:
		if end.By != s.StartedBy {
			return ErrEditSessionNotYours
		}
	case EditEndedByAdmin:
		if strings.TrimSpace(end.By) == "" {
			return ErrEditSessionNotYours
		}
	case EditExpired:
		end.By = ""
	default:
		return fmt.Errorf("%w: %q", ErrEditEndReasonUnknown, end.Reason)
	}
	at := end.At
	s.EndedAt, s.EndedBy, s.EndReason = &at, end.By, end.Reason
	return nil
}

type EditTakenError struct {
	Session    *EditSession
	HolderName string
}

func (e *EditTakenError) Error() string {
	return fmt.Sprintf("googlesheet: %s is editing the google file by hand since %s", e.Session.StartedBy, e.Session.StartedAt.Format(time.RFC3339))
}

func (e *EditTakenError) Unwrap() error { return ErrFileBeingEdited }

type CheckStatus string

const (
	CheckOK        CheckStatus = "ok"
	CheckBroken    CheckStatus = "broken"
	CheckUnchecked CheckStatus = "unchecked"
)

type TabCheck struct {
	SheetID     string          `json:"sheet_id"`
	TabID       int64           `json:"tab_id"`
	TabName     string          `json:"tab_name"`
	RenamedFrom string          `json:"renamed_from,omitempty"`
	Reason      AttentionReason `json:"reason,omitempty"`
	Detail      string          `json:"detail,omitempty"`
	Unchecked   string          `json:"unchecked,omitempty"`
	RefWarnings []int           `json:"ref_warnings,omitempty"`
}

func (c TabCheck) Repairable() bool { return c.Reason.Repairable() }

type EditCheck struct {
	Status CheckStatus `json:"status,omitempty"`
	Detail string      `json:"detail,omitempty"`
	Tabs   []TabCheck  `json:"tabs,omitempty"`
}

func NewEditCheck(tabs []TabCheck) EditCheck {
	status := CheckOK
	for _, tab := range tabs {
		switch {
		case tab.Reason != "":
			return EditCheck{Status: CheckBroken, Tabs: tabs}
		case tab.Unchecked != "":
			status = CheckUnchecked
		}
	}
	return EditCheck{Status: status, Tabs: tabs}
}

func UncheckedEdit(detail string) EditCheck {
	return EditCheck{Status: CheckUnchecked, Detail: detail}
}

func (c EditCheck) Done() bool { return c.Status != "" }

func (t TabSnapshot) RefWarnings() []int {
	var rows []int
	seen := map[string]bool{}
	for i := firstBodyRow - 1; i < len(t.Refs); i++ {
		if len(t.Refs[i]) == 0 {
			continue
		}
		text, isText := t.Refs[i][0].(string)
		text = strings.TrimSpace(text)
		if isText && text == "" {
			continue
		}
		if !isText || !IsRowRef(text) || seen[text] {
			rows = append(rows, i+1)
			continue
		}
		seen[text] = true
	}
	return rows
}

func (s *Sheet) EmbedURL() string {
	return fmt.Sprintf("%s?%s#gid=%d", SpreadsheetURL(s.SpreadsheetID), EmbeddedEditorMode, s.TabID)
}

type EditSessionRepository interface {
	Start(ctx context.Context, session *EditSession) error
	Current(ctx context.Context, workspaceID, spreadsheetID string) (*EditSession, bool, error)
	Find(ctx context.Context, workspaceID, id string) (*EditSession, error)
	Extend(ctx context.Context, session *EditSession, now time.Time) error
	Settle(ctx context.Context, session *EditSession) error
	End(ctx context.Context, session *EditSession) (bool, error)
	SaveCheck(ctx context.Context, session *EditSession) error
	Expired(ctx context.Context, now time.Time, limit int) ([]*EditSession, error)
	History(ctx context.Context, workspaceID, spreadsheetID string, pagination shared.Pagination) (*shared.PaginatedResult[*EditSession], error)
}
