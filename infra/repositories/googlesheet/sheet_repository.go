package googlesheet_repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
)

type sheetRepository struct {
	db *gorm.DB
}

func NewSheetRepository(db *gorm.DB) googlesheet.SheetRepository {
	return &sheetRepository{db: db}
}

func (r *sheetRepository) Create(ctx context.Context, s *googlesheet.Sheet) error {
	record, err := toSheetSchema(s)
	if err != nil {
		return err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		if !s.Periodic() {
			return nil
		}
		return saveTab(tx, record.ID, record.WorkspaceID, googlesheet.Tab{Period: s.TabPeriodKey, ID: s.TabID, Title: s.TabName})
	})
	if err != nil {
		return err
	}
	s.ID, s.CreatedAt, s.UpdatedAt = record.ID, record.CreatedAt, record.UpdatedAt
	return nil
}

func saveTab(tx *gorm.DB, sheetID, workspaceID string, tab googlesheet.Tab) error {
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&schema.GoogleSheetTab{
		SheetID: sheetID, PeriodKey: tab.Period, WorkspaceID: workspaceID, TabID: tab.ID, TabName: tab.Title, CreatedAt: time.Now().UTC(),
	}).Error
}

func (r *sheetRepository) OpenTab(ctx context.Context, id string, tab googlesheet.Tab) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record schema.GoogleSheet
		if err := tx.Where("id = ?", id).Take(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return googlesheet.ErrSheetNotFound
			}
			return err
		}
		if err := saveTab(tx, id, record.WorkspaceID, tab); err != nil {
			return err
		}
		return tx.Model(&schema.GoogleSheet{}).Where("id = ? AND tab_period_key < ?", id, tab.Period).
			Updates(map[string]any{"tab_id": tab.ID, "tab_name": tab.Title, "tab_period_key": tab.Period, "updated_at": time.Now().UTC()}).Error
	})
}

func (r *sheetRepository) FindTab(ctx context.Context, sheetID, period string) (googlesheet.Tab, bool, error) {
	var record schema.GoogleSheetTab
	err := r.db.WithContext(ctx).Where("sheet_id = ? AND period_key = ?", sheetID, period).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return googlesheet.Tab{}, false, nil
	}
	if err != nil {
		return googlesheet.Tab{}, false, err
	}
	return googlesheet.Tab{Period: record.PeriodKey, ID: record.TabID, Title: record.TabName}, true, nil
}

func (r *sheetRepository) Tabs(ctx context.Context, sheetID string) ([]googlesheet.Tab, error) {
	var records []schema.GoogleSheetTab
	if err := r.db.WithContext(ctx).Where("sheet_id = ?", sheetID).Order("period_key").Find(&records).Error; err != nil {
		return nil, err
	}
	out := make([]googlesheet.Tab, len(records))
	for i, record := range records {
		out[i] = googlesheet.Tab{Period: record.PeriodKey, ID: record.TabID, Title: record.TabName}
	}
	return out, nil
}

func (r *sheetRepository) InSpreadsheet(ctx context.Context, workspaceID, spreadsheetID string) ([]*googlesheet.Sheet, error) {
	return r.findAll(r.db.WithContext(ctx).Where("workspace_id = ? AND spreadsheet_id = ?", workspaceID, spreadsheetID))
}

const listAllCap = 1000

func (r *sheetRepository) ListAll(ctx context.Context, workspaceID string) ([]*googlesheet.Sheet, error) {
	return r.findAll(r.db.WithContext(ctx).Where("workspace_id = ?", workspaceID).Limit(listAllCap))
}

func (r *sheetRepository) findAll(query *gorm.DB) ([]*googlesheet.Sheet, error) {
	var records []schema.GoogleSheet
	if err := query.Order("created_at").Order("id").Find(&records).Error; err != nil {
		return nil, err
	}
	out := make([]*googlesheet.Sheet, 0, len(records))
	for i := range records {
		s, err := toSheetDomain(&records[i])
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *sheetRepository) Update(ctx context.Context, s *googlesheet.Sheet, expectedVersion int) error {
	columns, err := json.Marshal(s.Columns)
	if err != nil {
		return err
	}
	unique, err := json.Marshal(s.Unique.Normalized())
	if err != nil {
		return err
	}
	result := r.db.WithContext(ctx).Model(&schema.GoogleSheet{}).
		Where("id = ? AND workspace_id = ? AND version = ?", s.ID, s.WorkspaceID, expectedVersion).
		Updates(map[string]any{
			"title":         s.Title,
			"description":   s.Description,
			"style":         string(s.Style),
			"tab_pattern":   s.TabPattern,
			"columns":       columns,
			"unique_rule":   unique,
			"status":        string(s.Status),
			"reason":        string(s.Reason),
			"reason_detail": clip(s.ReasonDetail, 500),
			"version":       s.Version,
			"updated_at":    time.Now().UTC(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return googlesheet.ErrStaleSheet
	}
	return nil
}

func (r *sheetRepository) SaveStatus(ctx context.Context, s *googlesheet.Sheet) error {
	result := r.db.WithContext(ctx).Model(&schema.GoogleSheet{}).Where("id = ?", s.ID).Updates(map[string]any{
		"status":        string(s.Status),
		"reason":        string(s.Reason),
		"reason_detail": clip(s.ReasonDetail, 500),
		"updated_at":    time.Now().UTC(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return googlesheet.ErrSheetNotFound
	}
	return nil
}

func (r *sheetRepository) UpdateTabName(ctx context.Context, id, tabName string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&schema.GoogleSheet{}).Where("id = ?", id).
			Updates(map[string]any{"tab_name": tabName, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE google_sheet_tabs t SET tab_name = ? FROM google_sheets s
			WHERE s.id = ? AND t.sheet_id = s.id AND t.tab_id = s.tab_id`, tabName, id).Error
	})
}

func (r *sheetRepository) Get(ctx context.Context, id string) (*googlesheet.Sheet, error) {
	return r.find(r.db.WithContext(ctx).Where("id = ?", id))
}

func (r *sheetRepository) FindInWorkspace(ctx context.Context, workspaceID, id string) (*googlesheet.Sheet, error) {
	return r.find(r.db.WithContext(ctx).Where("id = ? AND workspace_id = ?", id, workspaceID))
}

func (r *sheetRepository) find(query *gorm.DB) (*googlesheet.Sheet, error) {
	var record schema.GoogleSheet
	if err := query.Limit(1).Find(&record).Error; err != nil {
		return nil, err
	}
	if record.ID == "" {
		return nil, googlesheet.ErrSheetNotFound
	}
	return toSheetDomain(&record)
}

func (r *sheetRepository) matching(ctx context.Context, in googlesheet.ListInput) *gorm.DB {
	query := r.db.WithContext(ctx).Model(&schema.GoogleSheet{}).Where("workspace_id = ?", in.WorkspaceID)
	if in.SpreadsheetID != "" {
		query = query.Where("spreadsheet_id = ?", in.SpreadsheetID)
	}
	if search := strings.TrimSpace(in.Search); search != "" {
		query = query.Where("title ILIKE ?", "%"+escapeLike(search)+"%")
	}
	if in.TakesInput {
		query = query.Where("columns @> ?::jsonb", `[{"source":"input","archived":false}]`)
	}
	if in.TakesMessages {
		clauses := make([]string, 0, len(googlesheet.PerMessageSources()))
		args := make([]any, 0, len(googlesheet.PerMessageSources()))
		for _, source := range googlesheet.PerMessageSources() {
			clauses = append(clauses, "columns @> ?::jsonb")
			args = append(args, `[{"source":"`+string(source)+`","archived":false}]`)
		}
		query = query.Where("("+strings.Join(clauses, " OR ")+")", args...)
	}
	return query
}

func (r *sheetRepository) ListSpreadsheets(ctx context.Context, in googlesheet.ListInput) (*shared.PaginatedResult[googlesheet.Spreadsheet], error) {
	pagination := shared.NormalizePagination(in.Pagination)
	var total int64
	if err := r.matching(ctx, in).Distinct("spreadsheet_id").Count(&total).Error; err != nil {
		return nil, err
	}
	var ids []string
	if err := r.matching(ctx, in).Group("spreadsheet_id").Order("MAX(created_at) DESC").Order("spreadsheet_id DESC").
		Offset(pagination.Offset()).Limit(pagination.PageSize).Pluck("spreadsheet_id", &ids).Error; err != nil {
		return nil, err
	}
	files := []googlesheet.Spreadsheet{}
	if len(ids) > 0 {
		sheets, err := r.findAll(r.db.WithContext(ctx).Where("workspace_id = ? AND spreadsheet_id IN ?", in.WorkspaceID, ids))
		if err != nil {
			return nil, err
		}
		titles, err := r.FileTitles(ctx, in.WorkspaceID, ids)
		if err != nil {
			return nil, err
		}
		grouped := map[string]googlesheet.Spreadsheet{}
		for _, file := range googlesheet.GroupBySpreadsheet(sheets) {
			file.Name = titles[file.ID]
			grouped[file.ID] = file
		}
		for _, id := range ids {
			if file, ok := grouped[id]; ok {
				files = append(files, file)
			}
		}
	}
	return shared.NewPaginatedResult(files, pagination, total), nil
}

func (r *sheetRepository) SaveFileTitle(ctx context.Context, workspaceID, spreadsheetID, title string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "spreadsheet_id"}},
		DoUpdates: clause.Assignments(map[string]any{"title": title, "updated_at": now}),
	}).Create(&schema.GoogleSheetFile{WorkspaceID: workspaceID, SpreadsheetID: spreadsheetID, Title: title, CreatedAt: now, UpdatedAt: now}).Error
}

func (r *sheetRepository) FileTitles(ctx context.Context, workspaceID string, spreadsheetIDs []string) (map[string]string, error) {
	titles := map[string]string{}
	if len(spreadsheetIDs) == 0 {
		return titles, nil
	}
	var records []schema.GoogleSheetFile
	if err := r.db.WithContext(ctx).Where("workspace_id = ? AND spreadsheet_id IN ?", workspaceID, spreadsheetIDs).Find(&records).Error; err != nil {
		return nil, err
	}
	for _, record := range records {
		titles[record.SpreadsheetID] = record.Title
	}
	return titles, nil
}

func (r *sheetRepository) List(ctx context.Context, in googlesheet.ListInput) (*shared.PaginatedResult[*googlesheet.Sheet], error) {
	pagination := shared.NormalizePagination(in.Pagination)
	query := r.matching(ctx, in)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var records []schema.GoogleSheet
	if err := query.Order("created_at DESC").Order("id DESC").Offset(pagination.Offset()).Limit(pagination.PageSize).Find(&records).Error; err != nil {
		return nil, err
	}
	sheets := make([]*googlesheet.Sheet, 0, len(records))
	for i := range records {
		s, err := toSheetDomain(&records[i])
		if err != nil {
			return nil, err
		}
		sheets = append(sheets, s)
	}
	return shared.NewPaginatedResult(sheets, pagination, total), nil
}

func (r *sheetRepository) Delete(ctx context.Context, workspaceID, id string) error {
	result := r.db.WithContext(ctx).Where("id = ? AND workspace_id = ?", id, workspaceID).Delete(&schema.GoogleSheet{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return googlesheet.ErrSheetNotFound
	}
	return nil
}

func (r *sheetRepository) TouchWritten(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&schema.GoogleSheet{}).Where("id = ?", id).
		UpdateColumn("last_written_at", at).Error
}

func toSheetSchema(s *googlesheet.Sheet) (*schema.GoogleSheet, error) {
	columns, err := json.Marshal(s.Columns)
	if err != nil {
		return nil, err
	}
	unique, err := json.Marshal(s.Unique.Normalized())
	if err != nil {
		return nil, err
	}
	record := &schema.GoogleSheet{
		ID:            s.ID,
		WorkspaceID:   s.WorkspaceID,
		AccountID:     s.AccountID,
		SpreadsheetID: s.SpreadsheetID,
		TabID:         s.TabID,
		TabName:       s.TabName,
		TabPeriod:     string(s.TabPeriod),
		TabPattern:    s.TabPattern,
		TabPeriodKey:  s.TabPeriodKey,
		Style:         string(s.Style),
		Title:         s.Title,
		Description:   s.Description,
		TimeZone:      s.TimeZone,
		Locale:        s.Locale,
		Columns:       columns,
		UniqueRule:    unique,
		Status:        string(s.Status),
		Reason:        string(s.Reason),
		ReasonDetail:  clip(s.ReasonDetail, 500),
		Version:       s.Version,
	}
	if s.CreatedBy != "" {
		record.CreatedBy = &s.CreatedBy
	}
	return record, nil
}

func toSheetDomain(record *schema.GoogleSheet) (*googlesheet.Sheet, error) {
	var columns []googlesheet.Column
	if err := json.Unmarshal(record.Columns, &columns); err != nil {
		return nil, err
	}
	var unique googlesheet.UniqueRule
	if len(record.UniqueRule) > 0 {
		if err := json.Unmarshal(record.UniqueRule, &unique); err != nil {
			return nil, err
		}
	}
	s := &googlesheet.Sheet{
		ID:            record.ID,
		WorkspaceID:   record.WorkspaceID,
		AccountID:     record.AccountID,
		SpreadsheetID: record.SpreadsheetID,
		TabID:         record.TabID,
		TabName:       record.TabName,
		TabPeriod:     googlesheet.TabPeriod(record.TabPeriod),
		TabPattern:    record.TabPattern,
		TabPeriodKey:  record.TabPeriodKey,
		Style:         googlesheet.StylePreset(record.Style),
		Title:         record.Title,
		Description:   record.Description,
		TimeZone:      record.TimeZone,
		Locale:        record.Locale,
		Columns:       columns,
		Unique:        unique.Normalized(),
		Status:        googlesheet.Status(record.Status),
		Reason:        googlesheet.AttentionReason(record.Reason),
		ReasonDetail:  record.ReasonDetail,
		Version:       record.Version,
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
		LastWrittenAt: record.LastWrittenAt,
	}
	if record.CreatedBy != nil {
		s.CreatedBy = *record.CreatedBy
	}
	return s, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func clip(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
