package billing_usecase

import (
	"context"
	"fmt"
	"log"
	"time"

	billing "vozko/domain/billing"
	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

type confirmMonthlyBillingUseCase struct {
	subs     workspace_plan.SubscriptionRepository
	addons   workspace_addon.AddonSubscriptionRepository
	onChange workspace_addon.EntitlementChangeHandler
	access   AccessHooks
	dueDay   int
	now      clockFn
}

var (
	_ billing.ConfirmMonthlyBillingUseCase = (*confirmMonthlyBillingUseCase)(nil)
	_ billing.ConfirmBilledCycleUseCase    = (*confirmMonthlyBillingUseCase)(nil)
)

func NewConfirmMonthlyBillingUseCase(
	subs workspace_plan.SubscriptionRepository,
	addons workspace_addon.AddonSubscriptionRepository,
) *confirmMonthlyBillingUseCase {
	return &confirmMonthlyBillingUseCase{
		subs:   subs,
		addons: addons,
		dueDay: billing.DefaultDueDay,
		now:    brtNow,
	}
}

func (uc *confirmMonthlyBillingUseCase) WithReactivation(handler workspace_addon.EntitlementChangeHandler) *confirmMonthlyBillingUseCase {
	uc.onChange = handler
	return uc
}

func (uc *confirmMonthlyBillingUseCase) WithAccessHooks(hooks AccessHooks) *confirmMonthlyBillingUseCase {
	uc.access = hooks
	return uc
}

func (uc *confirmMonthlyBillingUseCase) Execute(workspaceID string) error {
	return uc.ExecuteForCycle(workspaceID, billing.LatestIssuedDue(uc.now()))
}

func (uc *confirmMonthlyBillingUseCase) ExecuteForCycle(workspaceID string, billedDue time.Time) error {
	now := uc.now()
	revived := false

	if sub, err := uc.subs.GetLatestByWorkspaceID(workspaceID); err != nil {
		log.Printf("[billing-confirm] workspace %s: get plan subscription failed: %v", workspaceID, err)
	} else if sub != nil {
		wasExpired := sub.Status == workspace_plan.SubscriptionStatusExpired
		if renewPlan(sub, billedDue, now, uc.dueDay) {
			revived = revived || wasExpired
			if err := uc.subs.Update(sub); err != nil {
				return fmt.Errorf("update plan subscription: %w", err)
			}
		}
	}

	addons, err := uc.addons.ListReactivatableByWorkspace(workspaceID, now.AddDate(0, -1, 0))
	if err != nil {
		return fmt.Errorf("list reactivatable addons: %w", err)
	}
	for _, a := range addons {
		wasExpired := a.Status == workspace_plan.SubscriptionStatusExpired
		if !a.RenewForInvoice(billedDue, now, uc.dueDay) {
			continue
		}
		revived = revived || (wasExpired && a.Status == workspace_plan.SubscriptionStatusActive)
		if err := uc.addons.Update(a); err != nil {
			return fmt.Errorf("update addon %s: %w", a.ID, err)
		}
	}

	if revived && uc.onChange != nil {
		if err := uc.onChange.OnEntitlementIncreased(workspaceID, workspace_addon.EntitlementWhatsAppBusinessPhones); err != nil {
			log.Printf("[billing-confirm] workspace %s: channel reactivation after late payment failed: %v", workspaceID, err)
		}
	}
	if revived && uc.access != nil {
		if err := uc.access.OnAccessRestored(context.Background(), workspaceID); err != nil {
			log.Printf("[billing-confirm] workspace %s: access restore after late payment failed: %v", workspaceID, err)
		}
	}
	return nil
}

func renewPlan(sub *workspace_plan.WorkspaceSubscription, billedDue, now time.Time, dueDay int) bool {
	if billing.DueOn(sub.CurrentPeriodEnd, billedDue) {
		sub.Extend(now, dueDay)
		return true
	}
	if sub.Status == workspace_plan.SubscriptionStatusExpired && sub.CurrentPeriodEnd.After(now) {
		sub.Status = workspace_plan.SubscriptionStatusActive
		sub.UpdatedAt = now
		return true
	}
	return false
}
