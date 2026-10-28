package googlesheet_repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/infra/crypto/piigorm"
	"vozko/infra/database/schema"
)

type rowRemovalRepository struct {
	db *gorm.DB
}

func NewRowRemovalRepository(db *gorm.DB) googlesheet.RowRemovalRepository {
	if db == nil {
		panic("googlesheet: the row removal repository requires the database")
	}
	return &rowRemovalRepository{db: db}
}

func (r *rowRemovalRepository) FindRow(ctx context.Context, workspaceID, sheetID, rowID string) (*googlesheet.Row, error) {
	var record schema.GoogleSheetRow
	err := r.db.WithContext(ctx).Where("id = ? AND workspace_id = ? AND sheet_id = ?", rowID, workspaceID, sheetID).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, googlesheet.ErrRowNotFound
	}
	if err != nil {
		return nil, err
	}
	return toRowDomain(&record)
}

func (r *rowRemovalRepository) CancelQueued(ctx context.Context, removal *googlesheet.RowRemoval) error {
	queued := make([]string, 0, len(googlesheet.QueuedStatuses()))
	for _, status := range googlesheet.QueuedStatuses() {
		queued = append(queued, string(status))
	}
	return r.removeAndRecord(ctx, removal, queued, true)
}

func (r *rowRemovalRepository) RecordDeleted(ctx context.Context, removal *googlesheet.RowRemoval) error {
	return r.removeAndRecord(ctx, removal, []string{string(googlesheet.RowWritten)}, false)
}

func (r *rowRemovalRepository) removeAndRecord(ctx context.Context, removal *googlesheet.RowRemoval, statuses []string, mustRemove bool) error {
	record, err := toRemovalSchema(removal)
	if err != nil {
		return err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		removed := tx.Where("id = ? AND workspace_id = ? AND sheet_id = ? AND status IN ?", removal.RowID, removal.WorkspaceID, removal.SheetID, statuses).
			Delete(&schema.GoogleSheetRow{})
		if removed.Error != nil {
			return removed.Error
		}
		if mustRemove && removed.RowsAffected != 1 {
			return googlesheet.ErrRowStatusChanged
		}
		if err := releaseClaims(tx, removal.SheetID, []string{removal.RowID}); err != nil {
			return err
		}
		return tx.Create(record).Error
	})
	if err != nil {
		return err
	}
	removal.ID = record.ID
	return nil
}

func (r *rowRemovalRepository) ListRemovals(ctx context.Context, workspaceID, sheetID string, p shared.Pagination) (*shared.PaginatedResult[*googlesheet.RowRemoval], error) {
	query := r.db.WithContext(ctx).Model(&schema.GoogleSheetRowRemoval{}).Where("workspace_id = ? AND sheet_id = ?", workspaceID, sheetID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var records []schema.GoogleSheetRowRemoval
	if err := query.Order("removed_at DESC, id DESC").Offset(p.Offset()).Limit(p.PageSize).Find(&records).Error; err != nil {
		return nil, err
	}
	removals := make([]*googlesheet.RowRemoval, 0, len(records))
	for i := range records {
		removal, err := toRemovalDomain(&records[i])
		if err != nil {
			return nil, err
		}
		removals = append(removals, removal)
	}
	return shared.NewPaginatedResult(removals, p, total), nil
}

func toRemovalSchema(removal *googlesheet.RowRemoval) (*schema.GoogleSheetRowRemoval, error) {
	cells, err := json.Marshal(removal.Cells)
	if err != nil {
		return nil, fmt.Errorf("googlesheet: encode removed cells: %w", err)
	}
	record := &schema.GoogleSheetRowRemoval{
		ID:          removal.ID,
		WorkspaceID: removal.WorkspaceID,
		SheetID:     removal.SheetID,
		RowID:       removal.RowID,
		Kind:        string(removal.Kind),
		Reason:      piigorm.NewEncrypted(removal.Reason),
		RemovedBy:   removal.RemovedBy,
		Cells:       piigorm.NewEncrypted(string(cells)),
		PeriodKey:   removal.PeriodKey,
		OriginKind:  string(removal.Origin.Kind),
		AgentID:     removal.Origin.AgentID,
		WorkflowID:  removal.Origin.WorkflowID,
		RunID:       removal.Origin.RunID,
		ActorID:     removal.Origin.ActorID,
		EntryID:     removal.Origin.EntryID,
		EntryType:   removal.Origin.EntryType,
		RemovedAt:   removal.RemovedAt,
	}
	if removal.RowNumber > 0 {
		number := removal.RowNumber
		record.RowNumber = &number
	}
	if removal.Origin.LeadID != "" {
		lead := removal.Origin.LeadID
		record.LeadID = &lead
	}
	return record, nil
}

func toRemovalDomain(record *schema.GoogleSheetRowRemoval) (*googlesheet.RowRemoval, error) {
	cells := googlesheet.Cells{}
	if record.Cells.Valid && record.Cells.Plain != "" {
		if err := json.Unmarshal([]byte(record.Cells.Plain), &cells); err != nil {
			return nil, fmt.Errorf("googlesheet: decode removed cells of %s: %w", record.ID, err)
		}
	}
	removal := &googlesheet.RowRemoval{
		ID:          record.ID,
		WorkspaceID: record.WorkspaceID,
		SheetID:     record.SheetID,
		RowID:       record.RowID,
		Kind:        googlesheet.RemovalKind(record.Kind),
		Reason:      record.Reason.Plain,
		RemovedBy:   record.RemovedBy,
		Cells:       cells,
		PeriodKey:   record.PeriodKey,
		Origin: googlesheet.Origin{
			Kind:       googlesheet.OriginKind(record.OriginKind),
			AgentID:    record.AgentID,
			WorkflowID: record.WorkflowID,
			RunID:      record.RunID,
			ActorID:    record.ActorID,
			EntryID:    record.EntryID,
			EntryType:  record.EntryType,
		},
		RemovedAt: record.RemovedAt,
	}
	if record.RowNumber != nil {
		removal.RowNumber = *record.RowNumber
	}
	if record.LeadID != nil {
		removal.Origin.LeadID = *record.LeadID
	}
	return removal, nil
}
