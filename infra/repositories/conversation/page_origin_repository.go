package conversation_repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
)

var ErrPageOriginIncomplete = errors.New("conversation: a page origin needs its conversation, workspace, page and reference")

type pageOriginRepository struct {
	db *gorm.DB
}

func NewPageOriginRepository(db *gorm.DB) conversation.PageOriginRepository {
	return &pageOriginRepository{db: db}
}

func (r *pageOriginRepository) Claim(ctx context.Context, origin conversation.PageOrigin) (bool, error) {
	if !origin.Complete() {
		return false, ErrPageOriginIncomplete
	}
	row := schema.ConversationPageOrigin{
		EntryID: origin.EntryID, EntryType: string(origin.EntryType), WorkspaceID: origin.WorkspaceID,
		ProjectID: origin.ProjectID, Route: origin.Route, Reference: origin.Reference, ArrivedAt: origin.ArrivedAt,
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	return result.RowsAffected == 1, result.Error
}

func (r *pageOriginRepository) Get(ctx context.Context, entryID string, entryType shared.EntryType) (*conversation.PageOrigin, error) {
	var row schema.ConversationPageOrigin
	err := r.db.WithContext(ctx).Where("entry_id = ? AND entry_type = ?", entryID, string(entryType)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &conversation.PageOrigin{
		EntryID: row.EntryID, EntryType: shared.EntryType(row.EntryType), WorkspaceID: row.WorkspaceID,
		ProjectID: row.ProjectID, Route: row.Route, Reference: row.Reference, ArrivedAt: row.ArrivedAt,
	}, nil
}
