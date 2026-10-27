package whatsapp_campaign_usecase

import (
	"vozko/domain/campaign"
	wc "vozko/domain/whatsapp_campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
)

type getSummaryUseCase struct {
	aggregator wce.SummaryAggregator
}

func NewGetSummaryUseCase(aggregator wce.SummaryAggregator) wc.GetSummaryUseCase {
	return &getSummaryUseCase{aggregator: aggregator}
}

func (uc *getSummaryUseCase) Execute(filter wce.WorkspaceSummaryFilter) (*campaign.SendsSummary, error) {
	tally, err := uc.aggregator.TallySends(filter)
	if err != nil {
		return nil, err
	}
	return campaign.SummarizeSends(tally)
}
