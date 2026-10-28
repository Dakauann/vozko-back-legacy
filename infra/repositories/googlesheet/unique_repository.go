package googlesheet_repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/googlesheet"
	"vozko/infra/database/schema"
)

const holderSQL = "SELECT row_id FROM google_sheet_unique_keys WHERE sheet_id = ? AND period_key = ? AND key_hash = ?"

var errRepeated = errors.New("googlesheet: the unique key is held by another row")

var claimConflict = clause.OnConflict{
	Columns:   []clause.Column{{Name: "sheet_id"}, {Name: "period_key"}, {Name: "key_hash"}},
	DoNothing: true,
}

func (r *rowRepository) InsertUnique(ctx context.Context, row *googlesheet.Row, unique googlesheet.UniqueKey) (googlesheet.Inserted, error) {
	record, err := toRowSchema(row)
	if err != nil {
		return googlesheet.Inserted{}, err
	}
	var out googlesheet.Inserted
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "sheet_id"}, {Name: "idempotency_key"}},
			DoNothing: true,
		}).Create(record)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			stored, err := storedByKey(tx, row.SheetID, row.IdempotencyKey)
			out = googlesheet.Inserted{Row: stored, Duplicate: true}
			return err
		}
		if !unique.Empty() {
			claimed := tx.Clauses(claimConflict).Create(claimRecord(row, record.ID, unique))
			if claimed.Error != nil {
				return claimed.Error
			}
			if claimed.RowsAffected == 0 {
				holder, err := holderOf(tx, row.SheetID, unique)
				if err != nil {
					return err
				}
				out = googlesheet.Inserted{RepeatOf: holder}
				return errRepeated
			}
		}
		out = googlesheet.Inserted{Row: row}
		return nil
	})
	switch {
	case errors.Is(err, errRepeated):
		return out, nil
	case err != nil:
		return googlesheet.Inserted{}, err
	}
	if !out.Duplicate {
		row.ID = record.ID
	}
	return out, nil
}

func storedByKey(tx *gorm.DB, sheetID, idempotencyKey string) (*googlesheet.Row, error) {
	var existing schema.GoogleSheetRow
	if err := tx.Where("sheet_id = ? AND idempotency_key = ?", sheetID, idempotencyKey).Take(&existing).Error; err != nil {
		return nil, err
	}
	return toRowDomain(&existing)
}

func holderOf(tx *gorm.DB, sheetID string, unique googlesheet.UniqueKey) (string, error) {
	var holder string
	if err := tx.Raw(holderSQL, sheetID, unique.Period, unique.Hash).Scan(&holder).Error; err != nil {
		return "", err
	}
	if holder == "" {
		return "", fmt.Errorf("googlesheet: the unique key of sheet %s was released while this row was claiming it", sheetID)
	}
	return holder, nil
}

func claimRecord(row *googlesheet.Row, rowID string, unique googlesheet.UniqueKey) *schema.GoogleSheetUniqueKey {
	record := &schema.GoogleSheetUniqueKey{
		SheetID:     row.SheetID,
		PeriodKey:   unique.Period,
		KeyHash:     unique.Hash,
		WorkspaceID: row.WorkspaceID,
		RowID:       rowID,
		EntryID:     row.Origin.EntryID,
		EntryType:   row.Origin.EntryType,
		CreatedAt:   row.CreatedAt,
	}
	if row.Origin.LeadID != "" {
		lead := row.Origin.LeadID
		record.LeadID = &lead
	}
	return record
}

func (r *rowRepository) LedgerPage(ctx context.Context, sheetID string, after googlesheet.LedgerCursor, limit int) ([]*googlesheet.Row, error) {
	query := r.db.WithContext(ctx).Where("sheet_id = ?", sheetID)
	if after.ID != "" {
		query = query.Where("(created_at, id) > (?, ?::uuid)", after.CreatedAt, after.ID)
	}
	var records []schema.GoogleSheetRow
	if err := query.Order("created_at").Order("id").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return toRowDomains(records)
}

func (r *rowRepository) ClaimUnique(ctx context.Context, claims []googlesheet.UniqueClaim) (int, error) {
	if len(claims) == 0 {
		return 0, nil
	}
	records := make([]*schema.GoogleSheetUniqueKey, len(claims))
	for i, claim := range claims {
		records[i] = claimRecord(claim.Row, claim.Row.ID, claim.Key)
	}
	result := r.db.WithContext(ctx).Clauses(claimConflict).Create(records)
	return int(result.RowsAffected), result.Error
}

func releaseClaims(tx *gorm.DB, sheetID string, rowIDs any) error {
	return tx.Where("sheet_id = ? AND row_id IN (?)", sheetID, rowIDs).Delete(&schema.GoogleSheetUniqueKey{}).Error
}
