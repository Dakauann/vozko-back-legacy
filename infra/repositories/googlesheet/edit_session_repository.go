package googlesheet_repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/infra/database"
	"vozko/infra/database/schema"
)

const (
	openEditSessionIndex = "ux_google_sheet_edit_sessions_open"
	startAttempts        = 2
)

type editSessionRepository struct {
	db *gorm.DB
}

func NewEditSessionRepository(db *gorm.DB) googlesheet.EditSessionRepository {
	if db == nil {
		panic("googlesheet: the edit session repository requires the database")
	}
	return &editSessionRepository{db: db}
}

func (r *editSessionRepository) Start(ctx context.Context, session *googlesheet.EditSession) error {
	for range startAttempts {
		record, err := toEditSchema(session)
		if err != nil {
			return err
		}
		record.ID = uuid.NewString()
		err = r.db.WithContext(ctx).Create(record).Error
		if err == nil {
			session.ID = record.ID
			return nil
		}
		if !database.IsUniqueViolationOf(err, openEditSessionIndex) {
			return err
		}
		holder, found, err := r.Current(ctx, session.WorkspaceID, session.SpreadsheetID)
		if err != nil {
			return err
		}
		if found {
			return &googlesheet.EditTakenError{Session: holder}
		}
	}
	return fmt.Errorf("%w: the open session changed while starting", googlesheet.ErrFileBeingEdited)
}

func (r *editSessionRepository) Current(ctx context.Context, workspaceID, spreadsheetID string) (*googlesheet.EditSession, bool, error) {
	var records []schema.GoogleSheetEditSession
	err := r.db.WithContext(ctx).Where("workspace_id = ? AND spreadsheet_id = ? AND ended_at IS NULL", workspaceID, spreadsheetID).Limit(1).Find(&records).Error
	if err != nil || len(records) == 0 {
		return nil, false, err
	}
	session, err := toEditDomain(&records[0])
	return session, err == nil, err
}

func (r *editSessionRepository) Find(ctx context.Context, workspaceID, id string) (*googlesheet.EditSession, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, googlesheet.ErrEditSessionNotFound
	}
	var record schema.GoogleSheetEditSession
	err := r.db.WithContext(ctx).Where("id = ? AND workspace_id = ?", id, workspaceID).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, googlesheet.ErrEditSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	return toEditDomain(&record)
}

func (r *editSessionRepository) Extend(ctx context.Context, session *googlesheet.EditSession, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&schema.GoogleSheetEditSession{}).
		Where("id = ? AND ended_at IS NULL AND expires_at > ?", session.ID, now).
		Update("expires_at", session.ExpiresAt)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return googlesheet.ErrEditSessionEnded
	}
	return nil
}

func (r *editSessionRepository) Settle(ctx context.Context, session *googlesheet.EditSession) error {
	return r.db.WithContext(ctx).Model(&schema.GoogleSheetEditSession{}).
		Where("id = ? AND settled_at IS NULL", session.ID).
		Update("settled_at", session.SettledAt).Error
}

const queuedRowsOfFile = `(SELECT count(*) FROM google_sheet_rows r
	JOIN google_sheets s ON s.id = r.sheet_id
	WHERE s.workspace_id = google_sheet_edit_sessions.workspace_id
	AND s.spreadsheet_id = google_sheet_edit_sessions.spreadsheet_id
	AND s.deleted_at IS NULL
	AND r.status IN ('pending', 'in_flight'))`

func (r *editSessionRepository) End(ctx context.Context, session *googlesheet.EditSession) (bool, error) {
	if !session.Ended() {
		return false, fmt.Errorf("googlesheet: edit session %s has no end to store", session.ID)
	}
	ended := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&schema.GoogleSheetEditSession{}).
			Where("id = ? AND ended_at IS NULL", session.ID).
			Updates(map[string]any{
				"ended_at":    session.EndedAt,
				"ended_by":    session.EndedBy,
				"end_reason":  string(session.EndReason),
				"waited_rows": gorm.Expr(queuedRowsOfFile),
			})
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		ended = true
		var waited int
		if err := tx.Model(&schema.GoogleSheetEditSession{}).Where("id = ?", session.ID).Select("waited_rows").Scan(&waited).Error; err != nil {
			return err
		}
		session.WaitedRows = waited
		return nil
	})
	return ended, err
}

func (r *editSessionRepository) SaveCheck(ctx context.Context, session *googlesheet.EditSession) error {
	check, err := json.Marshal(session.Check)
	if err != nil {
		return fmt.Errorf("googlesheet: encode the edit check: %w", err)
	}
	return r.db.WithContext(ctx).Model(&schema.GoogleSheetEditSession{}).Where("id = ?", session.ID).Update("check", check).Error
}

func (r *editSessionRepository) Expired(ctx context.Context, now time.Time, limit int) ([]*googlesheet.EditSession, error) {
	var records []schema.GoogleSheetEditSession
	if err := r.db.WithContext(ctx).Where("ended_at IS NULL AND expires_at <= ?", now).Order("expires_at").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return toEditDomains(records)
}

func (r *editSessionRepository) History(ctx context.Context, workspaceID, spreadsheetID string, p shared.Pagination) (*shared.PaginatedResult[*googlesheet.EditSession], error) {
	query := r.db.WithContext(ctx).Model(&schema.GoogleSheetEditSession{}).Where("workspace_id = ? AND spreadsheet_id = ?", workspaceID, spreadsheetID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var records []schema.GoogleSheetEditSession
	if err := query.Order("started_at DESC, id DESC").Offset(p.Offset()).Limit(p.PageSize).Find(&records).Error; err != nil {
		return nil, err
	}
	sessions, err := toEditDomains(records)
	if err != nil {
		return nil, err
	}
	return shared.NewPaginatedResult(sessions, p, total), nil
}

func toEditSchema(session *googlesheet.EditSession) (*schema.GoogleSheetEditSession, error) {
	check, err := json.Marshal(session.Check)
	if err != nil {
		return nil, fmt.Errorf("googlesheet: encode the edit check: %w", err)
	}
	return &schema.GoogleSheetEditSession{
		ID:            session.ID,
		WorkspaceID:   session.WorkspaceID,
		SpreadsheetID: session.SpreadsheetID,
		StartedBy:     session.StartedBy,
		StartedAt:     session.StartedAt,
		ExpiresAt:     session.ExpiresAt,
		SettledAt:     session.SettledAt,
		EndedAt:       session.EndedAt,
		EndedBy:       session.EndedBy,
		EndReason:     string(session.EndReason),
		WaitedRows:    session.WaitedRows,
		Check:         check,
	}, nil
}

func toEditDomain(record *schema.GoogleSheetEditSession) (*googlesheet.EditSession, error) {
	var check googlesheet.EditCheck
	if len(record.Check) > 0 {
		if err := json.Unmarshal(record.Check, &check); err != nil {
			return nil, fmt.Errorf("googlesheet: decode the edit check of %s: %w", record.ID, err)
		}
	}
	return &googlesheet.EditSession{
		ID:            record.ID,
		WorkspaceID:   record.WorkspaceID,
		SpreadsheetID: record.SpreadsheetID,
		StartedBy:     record.StartedBy,
		StartedAt:     record.StartedAt.UTC(),
		ExpiresAt:     record.ExpiresAt.UTC(),
		SettledAt:     utcPointer(record.SettledAt),
		EndedAt:       utcPointer(record.EndedAt),
		EndedBy:       record.EndedBy,
		EndReason:     googlesheet.EditEndReason(record.EndReason),
		WaitedRows:    record.WaitedRows,
		Check:         check,
	}, nil
}

func toEditDomains(records []schema.GoogleSheetEditSession) ([]*googlesheet.EditSession, error) {
	out := make([]*googlesheet.EditSession, 0, len(records))
	for i := range records {
		session, err := toEditDomain(&records[i])
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, nil
}

func utcPointer(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	utc := at.UTC()
	return &utc
}
