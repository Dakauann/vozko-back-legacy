package automationneeds_repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/conversationvars"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
)

type ConversationVariables struct {
	db *gorm.DB
}

func NewConversationVariables(db *gorm.DB) *ConversationVariables {
	return &ConversationVariables{db: db}
}

func (r *ConversationVariables) Values(ctx context.Context, entryID string, entryType shared.EntryType) (map[string]string, error) {
	var rows []schema.ConversationVariable
	if err := r.db.WithContext(ctx).Where("entry_id = ? AND entry_type = ?", entryID, string(entryType)).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Name] = row.Value
	}
	return out, nil
}

func (r *ConversationVariables) Detailed(ctx context.Context, entryID string, entryType shared.EntryType) ([]conversationvars.Stored, error) {
	var rows []schema.ConversationVariable
	if err := r.db.WithContext(ctx).Where("entry_id = ? AND entry_type = ?", entryID, string(entryType)).Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]conversationvars.Stored, 0, len(rows))
	for _, row := range rows {
		out = append(out, conversationvars.Stored{Name: row.Name, Value: row.Value, SetBy: row.SetBy, SetAt: row.UpdatedAt})
	}
	return out, nil
}

func (r *ConversationVariables) Save(ctx context.Context, ref conversationvars.Ref, values map[string]string, by string) error {
	if len(values) == 0 {
		return nil
	}
	now := time.Now().UTC()
	rows := make([]schema.ConversationVariable, 0, len(values))
	for name, value := range values {
		rows = append(rows, schema.ConversationVariable{
			EntryID: ref.EntryID, EntryType: string(ref.EntryType), Name: name, WorkspaceID: ref.WorkspaceID,
			Value: value, SetBy: by, CreatedAt: now, UpdatedAt: now,
		})
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "entry_id"}, {Name: "entry_type"}, {Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "set_by", "updated_at"}),
	}).Create(&rows).Error
}

const officialCampaignSQL = `SELECT COALESCE(CAST(wce.metadata AS text), '{}') AS metadata, wce.variables, wc.name
FROM whatsapp_campaign_entries wce
JOIN whatsapp_campaigns wc ON wc.id = wce.campaign_id
WHERE CAST(wce.id AS text) = ? AND wce.deleted_at IS NULL
LIMIT 1`

const unofficialCampaignSQL = `SELECT COALESCE(CAST(e.metadata AS text), '{}') AS metadata, e.variables, c.name
FROM unofficial_whatsapp_campaign_entries e
JOIN unofficial_whatsapp_campaigns c ON c.id = e.campaign_id
WHERE CAST(e.conversation_id AS text) = ? AND e.deleted_at IS NULL
ORDER BY e.created_at DESC
LIMIT 1`

type campaignRow struct {
	Metadata  string
	Variables pq.StringArray
	Name      string
}

func (r *ConversationVariables) CampaignOf(ctx context.Context, entryID string, entryType shared.EntryType) (conversationvars.Campaign, error) {
	query := ""
	switch entryType {
	case shared.EntryTypeWhatsApp:
		query = officialCampaignSQL
	case shared.EntryTypeUnofficialWhatsApp:
		query = unofficialCampaignSQL
	default:
		return conversationvars.Campaign{}, nil
	}
	var rows []campaignRow
	if err := r.db.WithContext(ctx).Raw(query, entryID).Scan(&rows).Error; err != nil {
		return conversationvars.Campaign{}, err
	}
	if len(rows) == 0 {
		return conversationvars.Campaign{}, nil
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(rows[0].Metadata), &metadata); err != nil {
		return conversationvars.Campaign{}, fmt.Errorf("campaign metadata of %s %s: %w", entryType, entryID, err)
	}
	return conversationvars.Campaign{Name: rows[0].Name, Values: conversationvars.CampaignValuesOf(metadata, rows[0].Variables)}, nil
}

func (r *ConversationVariables) Forget(ctx context.Context, ref conversationvars.Ref, names []string) error {
	if len(names) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("entry_id = ? AND entry_type = ? AND name IN ?", ref.EntryID, string(ref.EntryType), names).
		Delete(&schema.ConversationVariable{}).Error
}
