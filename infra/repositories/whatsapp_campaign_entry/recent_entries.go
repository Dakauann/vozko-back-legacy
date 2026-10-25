package whatsapp_campaign_entry

import (
	"github.com/lib/pq"

	wce "vozko/domain/whatsapp_campaign_entry"
	"vozko/infra/database/schema"
)

const recentEntriesOrder = "updated_at DESC, id DESC"

const recentlyUpdatedForCampaignsSQL = `SELECT e.* FROM unnest(?::uuid[]) AS c(id) ` +
	`JOIN LATERAL (SELECT * FROM whatsapp_campaign_entries WHERE campaign_id = c.id AND deleted_at IS NULL ORDER BY ` + recentEntriesOrder + ` LIMIT ?) e ON true ` +
	`ORDER BY e.campaign_id, e.updated_at DESC, e.id DESC`

func (r *repository) ListRecentlyUpdatedForCampaigns(campaignIDs []string, limit int) (map[string][]wce.WhatsAppCampaignEntry, error) {
	ids := dedupeNonEmpty(campaignIDs)
	out := make(map[string][]wce.WhatsAppCampaignEntry, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	var rows []schema.WhatsAppCampaignEntry
	if err := r.db.Raw(recentlyUpdatedForCampaignsSQL, pq.Array(ids), limit).Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, id := range ids {
		out[id] = []wce.WhatsAppCampaignEntry{}
	}
	for i := range rows {
		out[rows[i].CampaignID] = append(out[rows[i].CampaignID], *toDomain(&rows[i]))
	}
	return out, nil
}
