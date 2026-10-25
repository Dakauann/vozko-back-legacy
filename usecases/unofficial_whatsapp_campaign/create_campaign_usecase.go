package unofficial_whatsapp_campaign

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"vozko/domain/campaign"
	"vozko/domain/lead"
	"vozko/domain/shared/deps"
	uw "vozko/domain/unofficial_whatsapp"
	uwc "vozko/domain/unofficial_whatsapp_campaign"
	workspace_department "vozko/domain/workspace/workspace_department"
	"vozko/usecases/campaignautomation"
	"vozko/usecases/campaigncreate"
	"vozko/usecases/campaignguard"
)

type createCampaignUseCase struct {
	repos              campaignRepos
	leads              LeadResolver
	instances          InstanceGateway
	eligibility        campaignguard.Screener
	departmentResolver workspace_department.CreationDepartmentResolver
	leadsByID          LeadsByID
	automation         AutomationCheck
	keyed              IdempotentCampaigns
	groupSenders       GroupSenderDirectory
}

type LeadsByID interface {
	FindByIDs(workspaceID string, ids []string) ([]*lead.Lead, error)
}

type AutomationCheck = campaignautomation.Checker

type IdempotentCampaigns interface {
	FindByIdempotencyKey(workspaceID, key string) (*uwc.Campaign, error)
	CreateWithEntries(c *uwc.Campaign, entries []uwc.Entry) error
}

type CreateCampaignDeps struct {
	Campaigns    uwc.Repository
	Entries      uwc.EntryRepository
	Leads        LeadResolver
	Instances    InstanceGateway
	Eligibility  campaignguard.Screener
	Departments  workspace_department.CreationDepartmentResolver
	LeadsByID    LeadsByID
	Automation   AutomationCheck
	Keyed        IdempotentCampaigns
	GroupSenders GroupSenderDirectory
}

func NewCreateCampaignUseCase(d CreateCampaignDeps) (uwc.CreateCampaignUseCase, error) {
	if err := deps.Require("unofficial campaign creation", map[string]any{
		"campaigns":           d.Campaigns,
		"entries":             d.Entries,
		"leads":               d.Leads,
		"instances":           d.Instances,
		"eligibility":         d.Eligibility,
		"department resolver": d.Departments,
		"leads by id":         d.LeadsByID,
		"automation":          d.Automation,
		"idempotency keys":    d.Keyed,
		"group senders":       d.GroupSenders,
	}); err != nil {
		return nil, err
	}
	return &createCampaignUseCase{
		repos:              campaignRepos{campaigns: d.Campaigns, entries: d.Entries},
		leads:              d.Leads,
		instances:          d.Instances,
		eligibility:        d.Eligibility,
		departmentResolver: d.Departments,
		leadsByID:          d.LeadsByID,
		automation:         d.Automation,
		keyed:              d.Keyed,
		groupSenders:       d.GroupSenders,
	}, nil
}

func (uc *createCampaignUseCase) Execute(
	ctx context.Context,
	in *uwc.Campaign,
	scope uw.DepartmentScope,
) (*uwc.Campaign, error) {
	if in == nil {
		return nil, uwc.ErrCampaignNameRequired
	}

	pacingUnset := in.SendDelayMinMS <= 0 && in.SendDelayMaxMS <= 0

	in.Normalize()
	if err := in.Validate(); err != nil {
		return nil, err
	}

	var instance *uw.Instance
	if !in.TargetsGroups() {
		usable, err := usableInstance(ctx, uc.instances, in.WorkspaceID, scope, in.InstanceID)
		if err != nil {
			return nil, err
		}
		instance = usable
	}

	if uc.departmentResolver != nil {
		departmentID, err := uc.departmentResolver.Resolve(ctx, in.WorkspaceID)
		if err != nil {
			return nil, err
		}
		in.DepartmentID = departmentID
	}

	if pacingUnset && instance != nil {
		in.SendDelayMinMS, in.SendDelayMaxMS = instance.SendDelayRange()
		in.Normalize()
	}

	existing, err := uc.existing(ctx, in)
	if err != nil || existing != nil {
		return existing, err
	}

	if err := in.ValidateTargetVariables(in.Message.ParameterCount()); err != nil {
		return nil, err
	}

	if err := uc.automation.Check(in.WorkspaceID, in.Automation(), in.EntryMetadata()); err != nil {
		return nil, err
	}

	if in.ID == "" {
		in.ID = uuid.New().String()
	}
	entries, err := uc.entriesFor(ctx, in, instance, scope)
	if err != nil {
		return nil, err
	}
	if err := uc.save(in, entries); err != nil {
		if errors.Is(err, campaign.ErrIdempotencyKeyTaken) {
			return uc.winnerOf(ctx, in)
		}
		return nil, err
	}

	return uc.hydrate(ctx, in.ID)
}

func (uc *createCampaignUseCase) save(in *uwc.Campaign, entries []uwc.Entry) error {
	if in.IdempotencyKey != "" {
		return uc.keyed.CreateWithEntries(in, entries)
	}
	if err := uc.repos.campaigns.Create(in); err != nil {
		return err
	}
	_, err := uc.repos.entries.CreateMany(entries)
	return err
}

func (uc *createCampaignUseCase) existing(ctx context.Context, in *uwc.Campaign) (*uwc.Campaign, error) {
	found, ok, err := campaigncreate.FindKeyed[*uwc.Campaign](uc.keyed, in.WorkspaceID, in.IdempotencyKey, uwc.ErrCampaignNotFound)
	if err != nil || !ok {
		return nil, err
	}
	return uc.hydrate(ctx, found.ID)
}

func (uc *createCampaignUseCase) winnerOf(ctx context.Context, in *uwc.Campaign) (*uwc.Campaign, error) {
	winner, err := uc.existing(ctx, in)
	if err != nil {
		return nil, err
	}
	if winner == nil {
		return nil, campaign.ErrIdempotencyKeyTaken
	}
	return winner, nil
}

func (uc *createCampaignUseCase) entriesFor(ctx context.Context, in *uwc.Campaign, instance *uw.Instance, scope uw.DepartmentScope) ([]uwc.Entry, error) {
	if in.TargetsGroups() {
		return uc.groupEntries(ctx, in, scope)
	}
	return uc.materializeTargets(ctx, in, instance)
}

func (uc *createCampaignUseCase) resolveTargets(in *uwc.Campaign) (campaigncreate.Resolved, error) {
	targets := make([]campaigncreate.Target, 0, len(in.Targets))
	for _, t := range in.Targets {
		targets = append(targets, campaigncreate.Target{LeadID: t.LeadID, Number: t.Number, Name: t.Name})
	}
	return campaigncreate.Resolve(uc.leadsByID, uc.leads, in.WorkspaceID, targets)
}

func (uc *createCampaignUseCase) materializeTargets(
	ctx context.Context,
	in *uwc.Campaign,
	instance *uw.Instance,
) ([]uwc.Entry, error) {
	targets, err := uc.resolveTargets(in)
	if err != nil {
		return nil, err
	}
	screening, err := uc.eligibility.Screen(ctx, in.WorkspaceID, targets.LeadIDs(), instance.ID)
	if err != nil {
		return nil, err
	}

	seeded := in.SeedOutcome.Statuses(len(in.Targets))
	now := time.Now().UTC()

	entries := make([]uwc.Entry, 0, len(in.Targets))
	seen := make(map[string]struct{}, len(in.Targets))
	for _, t := range in.Targets {
		number := t.Number
		l := targets.Of(campaigncreate.Target{LeadID: t.LeadID, Number: t.Number, Name: t.Name})
		if t.LeadID != "" {
			if l == nil {
				continue
			}
			number = l.Number
		}
		if l == nil {
			return nil, fmt.Errorf("%w: %q", uwc.ErrCampaignTargetInvalid, t.Number)
		}
		if _, dup := seen[l.ID]; dup {
			continue
		}
		seen[l.ID] = struct{}{}

		entry := uwc.Entry{
			ID:          uuid.New().String(),
			CampaignID:  in.ID,
			WorkspaceID: in.WorkspaceID,
			LeadID:      l.ID,
			Number:      number,
			Name:        t.Name,
			Status:      campaign.SendStatusPending,
			Variables:   t.Variables,
			Metadata:    t.Metadata,
		}
		if seeded != nil {
			entry.Status = seeded[len(entries)]
			if entry.Status != campaign.SendStatusPending {
				entry.SentAt = &now
			}
		}
		markIneligible(&entry, campaign.FirstSkip(screening.Skipped[l.ID], t.Skip), screening.Detail(t.Missing))
		entry.Normalize()
		entries = append(entries, entry)
	}
	return entries, nil
}

func markIneligible(entry *uwc.Entry, reason campaign.SkipReason, detail campaign.SkipDetail) {
	if reason == "" || entry.Status != campaign.SendStatusPending {
		return
	}
	entry.Status, entry.ErrorCode, entry.ErrorMessage = reason.OutcomeWith(detail)
}

func (uc *createCampaignUseCase) hydrate(ctx context.Context, campaignID string) (*uwc.Campaign, error) {
	saved, err := uc.repos.campaigns.FindByID(campaignID)
	if err != nil {
		return nil, err
	}
	if counts, err := uc.repos.entries.CountByStatus(campaignID); err == nil {
		saved.Metrics = campaign.NewMetrics(counts)
	}
	enrichInstance(ctx, uc.instances, saved)
	return saved, nil
}

func ensureInstanceCanCampaign(instance *uw.Instance) error {
	switch instance.Status {
	case uw.StatusBanned:
		return uwc.NewInstanceUnusableError(instance.Label(),
			"WhatsApp disabled this number; it cannot be recovered")
	case uw.StatusProvisionFailed:
		return uwc.NewInstanceUnusableError(instance.Label(),
			"this number was never finished connecting")
	}
	return nil
}

func enrichInstance(ctx context.Context, gateway InstanceGateway, campaigns ...*uwc.Campaign) {
	if gateway == nil {
		return
	}
	cache := map[string]*uw.Instance{}
	for _, c := range campaigns {
		if c == nil || c.InstanceID == "" {
			continue
		}
		instance, ok := cache[c.InstanceID]
		if !ok {
			var err error
			instance, err = gateway.Instance(ctx, c.InstanceID)
			if err != nil || instance == nil {
				continue
			}
			cache[c.InstanceID] = instance
		}
		c.InstanceLabel = instance.Label()
		c.InstanceStatus = string(instance.Status)
		c.InstanceSessionLive = instance.SessionLive()
	}
}
