package googlesheet_repository

import (
	"context"

	"gorm.io/gorm"

	"vozko/domain/campaign"
)

type CampaignUsage struct {
	db *gorm.DB
}

func NewCampaignUsage(db *gorm.DB) *CampaignUsage {
	return &CampaignUsage{db: db}
}

const campaignsUsingFile = `SELECT DISTINCT c.name FROM whatsapp_campaigns c
JOIN whatsapp_campaign_entries e ON e.campaign_id = c.id AND e.deleted_at IS NULL
WHERE c.workspace_id = ? AND c.deleted_at IS NULL AND c.status IN ?
AND EXISTS (SELECT 1 FROM jsonb_each_text(e.metadata) AS kv WHERE kv.value = ?)
ORDER BY c.name`

func (u *CampaignUsage) CampaignsUsing(ctx context.Context, workspaceID, spreadsheetID string) ([]string, error) {
	var names []string
	running := []string{string(campaign.StatusRunning), string(campaign.StatusPaused)}
	err := u.db.WithContext(ctx).Raw(campaignsUsingFile, workspaceID, running, spreadsheetID).Scan(&names).Error
	return names, err
}
