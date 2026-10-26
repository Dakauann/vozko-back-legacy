package conversation_repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/conversation"
	"vozko/infra/database/schema"
)

const mergeMetadataSQL = "CASE WHEN jsonb_typeof(metadata) = 'object' THEN metadata || ?::jsonb ELSE ?::jsonb END"

func mergedMetadata(patch []byte) clause.Expr {
	return gorm.Expr(mergeMetadataSQL, string(patch), string(patch))
}

func NewInteractiveBackfill(db *gorm.DB) conversation.InteractiveBackfill {
	return &repository{db: db}
}

func (r *repository) AttachMissingInteractive(ctx context.Context, ref conversation.SentMessageRef, content conversation.InteractiveContent) (bool, error) {
	carrier := &conversation.Message{}
	if err := carrier.AttachInteractive(&content); err != nil {
		return false, err
	}
	patch, ok := carrier.MetadataObject()
	if !ok {
		return false, nil
	}
	result := r.db.WithContext(ctx).Model(&schema.ConversationMessage{}).
		Where("entry_type = ? AND entry_id = ? AND external_message_id = ? AND sender_kind = ? AND sender_id = ?",
			string(ref.EntryType), ref.EntryID, ref.ProviderMessageID, string(ref.SentBy.Kind()), ref.SentBy.ID()).
		Where("metadata IS NULL OR (jsonb_typeof(metadata) = 'object' AND metadata->'interactive' IS NULL)").
		Update("metadata", mergedMetadata(patch))
	return result.RowsAffected > 0, result.Error
}
