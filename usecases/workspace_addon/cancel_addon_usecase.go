package workspace_addon_usecase

import (
	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

type cancelAddonSubscriptionUseCase struct {
	subs  workspace_addon.AddonSubscriptionRepository
	usage unitUsages
	now   clockFn
}

func NewCancelAddonSubscriptionUseCase(
	subs workspace_addon.AddonSubscriptionRepository,
	usage map[workspace_addon.EntitlementKind]workspace_addon.UnitUsage,
) workspace_addon.CancelAddonSubscriptionUseCase {
	return &cancelAddonSubscriptionUseCase{subs: subs, usage: usage, now: utcNow}
}

func (uc *cancelAddonSubscriptionUseCase) Execute(workspaceID, subscriptionID string) (*workspace_addon.AddonSubscription, error) {
	var out *workspace_addon.AddonSubscription
	err := lockedSubscription(uc.subs, workspaceID, subscriptionID, func(locked workspace_addon.AddonSubscriptionRepository, sub *workspace_addon.AddonSubscription) error {
		out = sub
		if sub.Status != workspace_plan.SubscriptionStatusActive {
			return nil
		}
		inUse, err := uc.usage.inUse(workspaceID, sub.EntitlementKind, sub.EntitlementKind.RequiresUnitCap())
		if err != nil {
			return err
		}
		now := uc.now()
		if err := sub.ScheduleCancellation(sub.ChangeDate(now), now, inUse); err != nil {
			return err
		}
		return locked.Update(sub)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
