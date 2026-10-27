package unofficial_whatsapp_campaign_repository

import (
	"strings"

	"gorm.io/gorm"

	"vozko/domain/campaign"
	uwc "vozko/domain/unofficial_whatsapp_campaign"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/campaignstamp"
)

type summaryRepository struct{ db *gorm.DB }

func NewSummaryRepository(db *gorm.DB) uwc.SummaryAggregator {
	return &summaryRepository{db: db}
}

func (r *summaryRepository) TallySends(filter uwc.WorkspaceSummaryFilter) (*campaign.SendsTally, error) {
	type row struct {
		Status  string
		Placed  int64
		Undated int64
	}

	placed, undated := "TRUE", "FALSE"
	var args []interface{}
	if filter.Window.Bounded() {
		moment := campaignstamp.OutcomeMoment("e")
		var bounds []string
		if filter.Window.From != nil {
			bounds = append(bounds, moment+" >= ?")
			args = append(args, filter.Window.From.UTC())
		}
		if filter.Window.To != nil {
			bounds = append(bounds, moment+" <= ?")
			args = append(args, filter.Window.To.UTC())
		}
		placed = strings.Join(bounds, " AND ")
		undated = "e.status <> ? AND " + moment + " IS NULL"
		args = append(args, string(campaign.SendStatusPending))
	}

	query := r.db.Model(&schema.UnofficialWhatsAppCampaignEntry{}).
		Table("unofficial_whatsapp_campaign_entries AS e").
		Select("e.status AS status, COUNT(*) FILTER (WHERE "+placed+") AS placed, COUNT(*) FILTER (WHERE "+undated+") AS undated", args...).
		Joins("JOIN unofficial_whatsapp_campaigns c ON c.id = e.campaign_id AND c.deleted_at IS NULL").
		Where("c.workspace_id = ?", filter.WorkspaceID).
		Where("e.deleted_at IS NULL")

	if len(filter.DepartmentIDs) > 0 {
		query = query.Where("c.department_id IN ?", filter.DepartmentIDs)
	}
	if len(filter.InstanceIDs) > 0 {
		query = query.Where("(c.instance_id IN ? OR e.sender_instance_id IN ?)", filter.InstanceIDs, filter.InstanceIDs)
	}

	var rows []row
	if err := query.Group("e.status").Scan(&rows).Error; err != nil {
		return nil, err
	}

	tally := &campaign.SendsTally{}
	for _, rec := range rows {
		tally.Add(campaign.SendSourceCampaign, campaign.SendStatus(rec.Status), "", rec.Placed)
		tally.Undated += rec.Undated
	}
	return tally, nil
}
