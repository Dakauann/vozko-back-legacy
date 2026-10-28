package workspace_addon_usecase

import (
	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

type scheduleAddonQuantityUseCase struct {
	subs  workspace_addon.AddonSubscriptionRepository
	usage unitUsages
	now   clockFn
}

func NewScheduleAddonQuantityUseCase(
	subs workspace_addon.AddonSubscriptionRepository,
	usage map[workspace_addon.EntitlementKind]workspace_addon.UnitUsage,
) workspace_addon.ScheduleAddonQuantityUseCase {
	return &scheduleAddonQuantityUseCase{subs: subs, usage: usage, now: utcNow}
}

func (uc *scheduleAddonQuantityUseCase) Execute(workspaceID, subscriptionID string, quantity int) (*workspace_addon.AddonSubscription, error) {
	var out *workspace_addon.AddonSubscription
	err := lockedSubscription(uc.subs, workspaceID, subscriptionID, func(locked workspace_addon.AddonSubscriptionRepository, sub *workspace_addon.AddonSubscription) error {
		if sub.Status != workspace_plan.SubscriptionStatusActive {
			return workspace_addon.ErrAddonSubscriptionNotFound
		}
		inUse, err := uc.usage.inUse(workspaceID, sub.EntitlementKind, true)
		if err != nil {
			return err
		}
		now := uc.now()
		if err := sub.ScheduleQuantity(quantity, sub.ChangeDate(now), inUse); err != nil {
			return err
		}
		sub.UpdatedAt = now
		if err := locked.Update(sub); err != nil {
			return err
		}
		out = sub
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
