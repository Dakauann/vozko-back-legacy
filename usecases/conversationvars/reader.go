package conversationvars

import (
	"context"
	"fmt"

	"vozko/domain/conversation"
	domain "vozko/domain/conversationvars"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
)

type EntryFacts interface {
	GetEntryInfo(entryID, entryType string) (conversation.EntryInfo, error)
}

type ReaderDeps struct {
	Facts     EntryFacts
	Campaigns domain.CampaignSource
	Store     domain.Store
}

type Reader struct {
	facts     EntryFacts
	campaigns domain.CampaignSource
	store     domain.Store
}

func NewReader(d ReaderDeps) (*Reader, error) {
	if err := deps.Require("conversation variables reader", map[string]any{
		"entry facts":     d.Facts,
		"campaign values": d.Campaigns,
		"stored values":   d.Store,
	}); err != nil {
		return nil, err
	}
	return &Reader{facts: d.Facts, campaigns: d.Campaigns, store: d.Store}, nil
}

func (r *Reader) Values(ctx context.Context, entryID string, entryType shared.EntryType) (map[string]string, error) {
	info, err := r.facts.GetEntryInfo(entryID, string(entryType))
	if err != nil {
		return nil, fmt.Errorf("%w: lead of %s %s: %w", domain.ErrUnavailable, entryType, entryID, err)
	}
	campaign, err := r.campaigns.CampaignOf(ctx, entryID, entryType)
	if err != nil {
		return nil, fmt.Errorf("%w: campaign of %s %s: %w", domain.ErrUnavailable, entryType, entryID, err)
	}
	stored, err := r.store.Values(ctx, entryID, entryType)
	if err != nil {
		return nil, fmt.Errorf("%w: stored values of %s %s: %w", domain.ErrUnavailable, entryType, entryID, err)
	}
	return domain.Merge(domain.Lead{Name: info.LeadName, Number: info.LeadNumber}, campaign, stored), nil
}
