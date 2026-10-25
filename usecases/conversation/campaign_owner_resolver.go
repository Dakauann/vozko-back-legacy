package conversation_usecase

import (
	"context"
	"errors"
	"fmt"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	wc_domain "vozko/domain/whatsapp_campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
)

type EntryOwnerResolver interface {
	WorkspaceIDForEntry(ctx context.Context, entryID string) (string, error)
	DepartmentIDForEntry(ctx context.Context, entryID string) (string, error)
}

type EntryCampaignResolver interface {
	CampaignIDForEntry(ctx context.Context, entryID string) (string, error)
}

type campaignWorkspaceResolver struct {
	wcCampaignRepo wc_domain.Repository
	waEntryRepo    wce.Repository
	routes         conversation.DepartmentRoutes
	entryResolvers map[shared.EntryType]EntryOwnerResolver
}

func NewCampaignWorkspaceResolver(
	wcCampaignRepo wc_domain.Repository,
	waEntryRepo wce.Repository,
	routes conversation.DepartmentRoutes,
) conversation.CampaignWorkspaceResolver {
	return &campaignWorkspaceResolver{
		wcCampaignRepo: wcCampaignRepo,
		waEntryRepo:    waEntryRepo,
		routes:         routes,
		entryResolvers: make(map[shared.EntryType]EntryOwnerResolver, 2),
	}
}

func (r *campaignWorkspaceResolver) SetEntryOwnerResolver(entryType shared.EntryType, repo EntryOwnerResolver) {
	if r == nil || repo == nil || entryType == "" {
		return
	}
	if r.entryResolvers == nil {
		r.entryResolvers = make(map[shared.EntryType]EntryOwnerResolver, 2)
	}
	r.entryResolvers[entryType] = repo
}

var ErrChannelNotRegistered = errors.New("conversation: no channel is registered for this entry type")

func (r *campaignWorkspaceResolver) resolverFor(entryType string) (EntryOwnerResolver, bool) {
	if r == nil || r.entryResolvers == nil {
		return nil, false
	}
	resolver, ok := r.entryResolvers[shared.EntryType(entryType)]
	return resolver, ok && resolver != nil
}

func (r *campaignWorkspaceResolver) GetCampaignWorkspaceID(campaignID, campaignType string) (string, error) {
	if campaignType != string(shared.EntryTypeWhatsApp) {
		return "", fmt.Errorf("unknown campaign type: %s", campaignType)
	}
	c, err := r.wcCampaignRepo.FindByID(campaignID)
	if err != nil {
		return "", fmt.Errorf("find whatsapp campaign: %w", err)
	}
	return c.WorkspaceID, nil
}

func (r *campaignWorkspaceResolver) GetCampaignDepartmentID(campaignID, campaignType string) (string, error) {
	if campaignType == string(shared.EntryTypeWhatsApp) {
		c, err := r.wcCampaignRepo.FindByID(campaignID)
		if err != nil {
			return "", fmt.Errorf("find whatsapp campaign: %w", err)
		}
		return c.DepartmentID, nil
	}
	if shared.EntryType(campaignType).IsKnown() {
		return "", nil
	}
	return "", fmt.Errorf("unknown campaign type: %s", campaignType)
}

func (r *campaignWorkspaceResolver) GetEntryWorkspaceID(entryID, entryType string) (string, error) {
	if resolver, ok := r.resolverFor(entryType); ok {
		return resolver.WorkspaceIDForEntry(context.Background(), entryID)
	}
	if entryType != string(shared.EntryTypeWhatsApp) {
		return "", fmt.Errorf("%w: %s", ErrChannelNotRegistered, entryType)
	}
	info, err := r.waEntryRepo.GetCampaignForEntry(entryID)
	if err != nil {
		return "", fmt.Errorf("get campaign for whatsapp entry: %w", err)
	}
	return r.GetCampaignWorkspaceID(info.CampaignID, string(shared.EntryTypeWhatsApp))
}

func (r *campaignWorkspaceResolver) GetEntryDepartmentID(entryID, entryType string) (string, error) {
	if r.routes == nil {
		return "", fmt.Errorf("department of %s (%s): %w", entryID, entryType, conversation.ErrDepartmentRoutesUnavailable)
	}
	routed, found, err := r.routes.RoutedDepartment(entryID, entryType)
	if err != nil {
		return "", fmt.Errorf("department of %s (%s): route: %w", entryID, entryType, err)
	}
	if found {
		return routed, nil
	}
	return r.defaultDepartmentID(entryID, entryType)
}

func (r *campaignWorkspaceResolver) defaultDepartmentID(entryID, entryType string) (string, error) {
	if entryType == string(shared.EntryTypeWhatsApp) {
		campaignID, err := r.GetEntryCampaignID(entryID, entryType)
		if err != nil {
			return "", err
		}
		return r.GetCampaignDepartmentID(campaignID, string(shared.EntryTypeWhatsApp))
	}
	resolver, ok := r.resolverFor(entryType)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrChannelNotRegistered, entryType)
	}
	return resolver.DepartmentIDForEntry(context.Background(), entryID)
}

func (r *campaignWorkspaceResolver) GetEntryCampaignID(entryID, entryType string) (string, error) {
	if entryType == string(shared.EntryTypeWhatsApp) {
		info, err := r.waEntryRepo.GetCampaignForEntry(entryID)
		if err != nil {
			return "", fmt.Errorf("get campaign for whatsapp entry: %w", err)
		}
		return info.CampaignID, nil
	}
	resolver, ok := r.resolverFor(entryType)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrChannelNotRegistered, entryType)
	}
	if withCampaign, ok := resolver.(EntryCampaignResolver); ok {
		return withCampaign.CampaignIDForEntry(context.Background(), entryID)
	}
	return "", nil
}
