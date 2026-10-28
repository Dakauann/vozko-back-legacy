package googlesheet

import (
	"context"
	"errors"
	"maps"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"vozko/domain/shared"
)

const MaxRemovalReasonRunes = 500

type RemovalKind string

const (
	RemovalCancel RemovalKind = "cancel"
	RemovalDelete RemovalKind = "remove"
)

var (
	ErrRowBeingWritten       = errors.New("googlesheet: the row is being written to google right now")
	ErrRowStatusChanged      = errors.New("googlesheet: the row changed status since it was listed")
	ErrRowNotInSheet         = errors.New("googlesheet: the row reference is not in column A of the tab")
	ErrRowRefRepeated        = errors.New("googlesheet: the row reference appears more than once in column A")
	ErrRemovalReasonRequired = errors.New("googlesheet: a reason is required to remove a row")
	ErrRemovalReasonTooLong  = errors.New("googlesheet: the removal reason is too long")
	ErrRemovalActorRequired  = errors.New("googlesheet: a removal needs the person who asked for it")
	ErrSheetBusy             = errors.New("googlesheet: the sheet is writing rows right now")
)

const (
	CodeRowBeingWritten       = "google_sheet_row_being_written"
	CodeRowStatusChanged      = "google_sheet_row_status_changed"
	CodeRowNotInSheet         = "google_sheet_row_not_in_sheet"
	CodeRowRefRepeated        = "google_sheet_row_ref_repeated"
	CodeRemovalReasonRequired = "google_sheet_removal_reason_required"
	CodeRemovalReasonTooLong  = "google_sheet_removal_reason_too_long"
	CodeRemovalActorRequired  = "google_sheet_removal_actor_required"
	CodeSheetBusy             = "google_sheet_busy"
)

func QueuedStatuses() []RowStatus {
	return []RowStatus{RowPending, RowHeld, RowFailed}
}

func (r *Row) Removal() (RemovalKind, bool) {
	switch r.Status {
	case RowPending, RowHeld, RowFailed:
		return RemovalCancel, true
	case RowWritten:
		return RemovalDelete, true
	}
	return "", false
}

func (r *Row) CheckRemoval(kind RemovalKind) error {
	if offered, ok := r.Removal(); ok && offered == kind {
		return nil
	}
	if r.Status == RowInFlight {
		return ErrRowBeingWritten
	}
	return ErrRowStatusChanged
}

func CheckRemovalReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", ErrRemovalReasonRequired
	}
	if utf8.RuneCountInString(reason) > MaxRemovalReasonRunes {
		return "", ErrRemovalReasonTooLong
	}
	return reason, nil
}

func IsRowRef(text string) bool {
	parsed, err := uuid.Parse(text)
	return err == nil && parsed.String() == text
}

func (t TabSnapshot) Locate(rowID string) (int, error) {
	if !IsRowRef(rowID) {
		return 0, ErrRowNotInSheet
	}
	found := 0
	for i := firstBodyRow - 1; i < len(t.Refs); i++ {
		if len(t.Refs[i]) == 0 {
			continue
		}
		if ref, ok := t.Refs[i][0].(string); !ok || ref != rowID {
			continue
		}
		if found != 0 {
			return 0, ErrRowRefRepeated
		}
		found = i + 1
	}
	if found == 0 {
		return 0, ErrRowNotInSheet
	}
	return found, nil
}

type RowRemoval struct {
	ID          string
	WorkspaceID string
	SheetID     string
	RowID       string
	Kind        RemovalKind
	Reason      string
	RemovedBy   string
	Cells       Cells
	PeriodKey   string
	RowNumber   int
	Origin      Origin
	RemovedAt   time.Time
}

func CheckRemovalRequest(removedBy, reason string) (string, string, error) {
	reason, err := CheckRemovalReason(reason)
	if err != nil {
		return "", "", err
	}
	removedBy = strings.TrimSpace(removedBy)
	if removedBy == "" {
		return "", "", ErrRemovalActorRequired
	}
	return removedBy, reason, nil
}

func NewRowRemoval(row *Row, kind RemovalKind, removedBy, reason string, rowNumber int, now time.Time) (*RowRemoval, error) {
	if err := row.CheckRemoval(kind); err != nil {
		return nil, err
	}
	removedBy, reason, err := CheckRemovalRequest(removedBy, reason)
	if err != nil {
		return nil, err
	}
	if kind == RemovalDelete && rowNumber < firstBodyRow {
		return nil, ErrRowNotInSheet
	}
	if kind == RemovalCancel {
		rowNumber = 0
	}
	return &RowRemoval{
		WorkspaceID: row.WorkspaceID,
		SheetID:     row.SheetID,
		RowID:       row.ID,
		Kind:        kind,
		Reason:      reason,
		RemovedBy:   removedBy,
		Cells:       maps.Clone(row.Cells),
		PeriodKey:   row.PeriodKey,
		RowNumber:   rowNumber,
		Origin:      row.Origin,
		RemovedAt:   now,
	}, nil
}

type RowRemovalRepository interface {
	FindRow(ctx context.Context, workspaceID, sheetID, rowID string) (*Row, error)
	CancelQueued(ctx context.Context, removal *RowRemoval) error
	RecordDeleted(ctx context.Context, removal *RowRemoval) error
	ListRemovals(ctx context.Context, workspaceID, sheetID string, pagination shared.Pagination) (*shared.PaginatedResult[*RowRemoval], error)
}
