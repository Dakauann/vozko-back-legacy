package googlesheet_repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/googlesheet"
	"vozko/infra/database/schema"
)

var errExportIncomplete = errors.New("googlesheet: an export needs its workspace, sheet and conversation")

type exportRepository struct {
	db *gorm.DB
}

func NewExportRepository(db *gorm.DB) googlesheet.ExportRepository {
	return &exportRepository{db: db}
}

func (r *exportRepository) Position(ctx context.Context, sheetID, entryID, entryType string) (googlesheet.ExportPosition, error) {
	var cursor schema.GoogleSheetExportCursor
	err := r.db.WithContext(ctx).
		Where("sheet_id = ? AND entry_type = ? AND entry_id = ?", sheetID, entryType, entryID).
		Take(&cursor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return googlesheet.ExportPosition{}, nil
	}
	if err != nil {
		return googlesheet.ExportPosition{}, err
	}
	return googlesheet.ExportPosition{At: cursor.LastMessageAt.UTC(), MessageID: cursor.LastMessageID}, nil
}

func (r *exportRepository) Record(ctx context.Context, export googlesheet.MessageExport) (int, error) {
	if strings.TrimSpace(export.WorkspaceID) == "" || strings.TrimSpace(export.SheetID) == "" || strings.TrimSpace(export.EntryID) == "" || strings.TrimSpace(export.EntryType) == "" {
		return 0, errExportIncomplete
	}
	records := make([]*schema.GoogleSheetRow, 0, len(export.Rows))
	for _, row := range export.Rows {
		record, err := toRowSchema(row)
		if err != nil {
			return 0, err
		}
		records = append(records, record)
	}
	inserted := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(records) > 0 {
			result := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "sheet_id"}, {Name: "idempotency_key"}},
				DoNothing: true,
			}).Create(&records)
			if result.Error != nil {
				return fmt.Errorf("googlesheet: record exported rows: %w", result.Error)
			}
			inserted = int(result.RowsAffected)
		}
		if export.Through.IsZero() {
			return nil
		}
		return tx.Exec(`
			INSERT INTO google_sheet_export_cursors (sheet_id, entry_type, entry_id, workspace_id, last_message_at, last_message_id, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (sheet_id, entry_type, entry_id) DO UPDATE
			SET last_message_at = EXCLUDED.last_message_at, last_message_id = EXCLUDED.last_message_id, updated_at = EXCLUDED.updated_at
			WHERE (google_sheet_export_cursors.last_message_at, google_sheet_export_cursors.last_message_id COLLATE "C") < (EXCLUDED.last_message_at, EXCLUDED.last_message_id COLLATE "C")`,
			export.SheetID, export.EntryType, export.EntryID, export.WorkspaceID, export.Through.At, export.Through.MessageID, time.Now().UTC()).Error
	})
	if err != nil {
		return 0, err
	}
	return inserted, nil
}
