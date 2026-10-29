package tutorial_repository

import (
	"context"

	"gorm.io/gorm"

	"vozko/domain/tutorial"
)

type SiteFacts struct {
	db *gorm.DB
}

func NewSiteFacts(db *gorm.DB) *SiteFacts {
	return &SiteFacts{db: db}
}

const siteFactsSQL = `SELECT
	EXISTS (SELECT 1 FROM site_projects WHERE workspace_id = @ws) AS has_page,
	EXISTS (SELECT 1 FROM site_revisions WHERE workspace_id = @ws AND kind = 'autosave' AND author LIKE 'person:%') AS edited_by_hand,
	EXISTS (SELECT 1 FROM site_settings s, jsonb_each(CASE WHEN jsonb_typeof(s.settings->'forms') = 'object' THEN s.settings->'forms' ELSE '{}'::jsonb END) f
		WHERE s.workspace_id = @ws AND COALESCE(f.value->>'funnel_id', '') <> '') AS form_to_funnel,
	EXISTS (SELECT 1 FROM site_settings WHERE workspace_id = @ws AND COALESCE(settings->'site'->>'webchat_widget', '') <> '') AS site_chat,
	EXISTS (SELECT 1 FROM site_settings WHERE workspace_id = @ws AND jsonb_typeof(settings->'bookings') = 'object' AND settings->'bookings' <> '{}'::jsonb) AS booking,
	EXISTS (SELECT 1 FROM site_hostnames WHERE workspace_id = @ws AND kind = 'custom' AND status = 'active') AS custom_domain,
	EXISTS (SELECT 1 FROM site_deployments WHERE workspace_id = @ws AND status IN ('active', 'superseded')) AS published`

type siteFactsRow struct {
	HasPage      bool
	EditedByHand bool
	FormToFunnel bool
	SiteChat     bool
	Booking      bool
	CustomDomain bool
	Published    bool
}

func (r *SiteFacts) SiteFacts(ctx context.Context, workspaceID string) (tutorial.SiteFacts, error) {
	var row siteFactsRow
	if err := r.db.WithContext(ctx).Raw(siteFactsSQL, map[string]any{"ws": workspaceID}).Scan(&row).Error; err != nil {
		return tutorial.SiteFacts{}, err
	}
	return tutorial.SiteFacts(row), nil
}
