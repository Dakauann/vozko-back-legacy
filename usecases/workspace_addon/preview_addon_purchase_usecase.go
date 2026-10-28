package workspace_addon_usecase

import (
	workspace_addon "vozko/domain/workspace/workspace_addon"
)

type previewAddonPurchaseUseCase struct {
	defs workspace_addon.AddonDefinitionRepository
	subs workspace_addon.AddonSubscriptionRepository
	now  clockFn
}

func NewPreviewAddonPurchaseUseCase(
	defs workspace_addon.AddonDefinitionRepository,
	subs workspace_addon.AddonSubscriptionRepository,
) workspace_addon.PreviewAddonPurchaseUseCase {
	return &previewAddonPurchaseUseCase{defs: defs, subs: subs, now: utcNow}
}

func (uc *previewAddonPurchaseUseCase) Execute(workspaceID string, input workspace_addon.PurchaseAddonInput) (*workspace_addon.AddonPurchasePreview, error) {
	def, err := loadPurchasable(uc.defs, workspaceID, input)
	if err != nil {
		return nil, err
	}
	plan, err := planPurchase(def, uc.subs, workspaceID, input, uc.now())
	if err != nil {
		return nil, err
	}
	return &workspace_addon.AddonPurchasePreview{
		ChargeNowMicros: plan.quote.ChargeMicros,
		RecurringMicros: plan.quote.RecurringMicros,
		BillingCycle:    plan.cycle,
		Prorated:        plan.quote.Prorated,
		ProratedDays:    plan.quote.ProratedDays,
		PeriodEnd:       plan.quote.PeriodEnd,
		NextInvoiceDate: plan.quote.PeriodEnd,
	}, nil
}
