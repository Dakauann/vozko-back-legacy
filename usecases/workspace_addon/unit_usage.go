package workspace_addon_usecase

import (
	"fmt"

	workspace_addon "vozko/domain/workspace/workspace_addon"
)

type unitUsages map[workspace_addon.EntitlementKind]workspace_addon.UnitUsage

func (u unitUsages) inUse(workspaceID string, kind workspace_addon.EntitlementKind, required bool) (int, error) {
	usage, known := u[kind]
	if !known || usage == nil {
		if required {
			return 0, workspace_addon.ErrReductionUnsupported
		}
		return 0, nil
	}
	units, err := usage.UnitsInUse(workspaceID)
	if err != nil {
		return 0, fmt.Errorf("read %s in use: %w", kind, err)
	}
	return units, nil
}

func lockedSubscription(
	subs workspace_addon.AddonSubscriptionRepository,
	workspaceID, subscriptionID string,
	change func(locked workspace_addon.AddonSubscriptionRepository, sub *workspace_addon.AddonSubscription) error,
) error {
	sub, err := subs.GetByID(subscriptionID)
	if err != nil {
		return err
	}
	if sub.WorkspaceID != workspaceID {
		return workspace_addon.ErrAddonSubscriptionNotFound
	}
	return subs.WithUnitsLock(workspaceID, sub.EntitlementKind, func(locked workspace_addon.AddonSubscriptionRepository) error {
		current, err := locked.GetByID(subscriptionID)
		if err != nil {
			return err
		}
		if current.WorkspaceID != workspaceID {
			return workspace_addon.ErrAddonSubscriptionNotFound
		}
		return change(locked, current)
	})
}
