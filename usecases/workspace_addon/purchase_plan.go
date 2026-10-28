package workspace_addon_usecase

import (
	"errors"
	"time"

	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

type purchasePlan struct {
	def      *workspace_addon.AddonDefinition
	existing *workspace_addon.AddonSubscription
	quantity int
	cycle    workspace_plan.BillingCycle
	quote    workspace_addon.PurchaseQuote
}

func loadPurchasable(defs workspace_addon.AddonDefinitionReader, workspaceID string, input workspace_addon.PurchaseAddonInput) (*workspace_addon.AddonDefinition, error) {
	if workspaceID == "" || input.AddonDefinitionID == "" {
		return nil, workspace_addon.ErrInvalidAddonSubscription
	}
	def, err := defs.GetByID(input.AddonDefinitionID)
	if err != nil {
		return nil, err
	}
	if def.IsArchived() || !def.IsActive {
		return nil, workspace_addon.ErrAddonInactive
	}
	return def, nil
}

func planPurchase(
	def *workspace_addon.AddonDefinition,
	subs workspace_addon.AddonSubscriptionRepository,
	workspaceID string,
	input workspace_addon.PurchaseAddonInput,
	now time.Time,
) (purchasePlan, error) {
	cycle, err := def.PurchaseCycle(input.BillingCycle)
	if err != nil {
		return purchasePlan{}, err
	}
	units, err := def.UnitsFor(input.Quantity)
	if err != nil {
		return purchasePlan{}, err
	}
	existing, err := subs.GetActiveByWorkspaceAndDefinition(workspaceID, def.ID)
	if err != nil && !errors.Is(err, workspace_addon.ErrAddonSubscriptionNotFound) {
		return purchasePlan{}, err
	}
	owned, err := subs.ListActiveByWorkspaceAndKind(workspaceID, def.EntitlementKind)
	if err != nil {
		return purchasePlan{}, err
	}
	total, err := workspace_addon.TotalUnits(units, owned)
	if err != nil {
		return purchasePlan{}, err
	}
	if err := def.AllowsUnits(total); err != nil {
		return purchasePlan{}, err
	}
	quote, err := workspace_addon.QuotePurchase(def, cycle, input.Quantity, now)
	if err != nil {
		return purchasePlan{}, err
	}
	return purchasePlan{def: def, existing: existing, quantity: input.Quantity, cycle: cycle, quote: quote}, nil
}

func (p purchasePlan) subscription(workspaceID, newID string, now time.Time) *workspace_addon.AddonSubscription {
	if p.existing == nil {
		return workspace_addon.NewAddonSubscription(newID, workspaceID, p.def, p.quantity, p.cycle, p.quote, now)
	}
	p.existing.TopUp(p.def, p.quantity, p.cycle, p.quote, now)
	return p.existing
}

func (p purchasePlan) save(subs workspace_addon.AddonSubscriptionRepository, sub *workspace_addon.AddonSubscription) error {
	if p.existing == nil {
		return subs.Create(sub)
	}
	return subs.Update(sub)
}
