package whatsapp_campaign_entry

import "vozko/domain/campaign"

type WorkspaceSummaryFilter struct {
	WorkspaceID   string
	DepartmentIDs []string
	Window        campaign.SendWindow
}

type SummaryAggregator interface {
	TallySends(filter WorkspaceSummaryFilter) (*campaign.SendsTally, error)
}
