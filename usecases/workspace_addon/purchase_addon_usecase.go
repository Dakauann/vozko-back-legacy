package workspace_addon_usecase

import (
	"errors"
	"log"

	"github.com/google/uuid"

	"vozko/domain/balance"
	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_pricing "vozko/domain/workspace/workspace_pricing"
	balance_usecase "vozko/usecases/balance"
)

type purchaseAddonUseCase struct {
	defs        workspace_addon.AddonDefinitionRepository
	subs        workspace_addon.AddonSubscriptionRepository
	balanceRepo balance.Repository
	onChange    workspace_addon.EntitlementChangeHandler
	now         clockFn
	newID       func() string
}

func NewPurchaseAddonUseCase(
	defs workspace_addon.AddonDefinitionRepository,
	subs workspace_addon.AddonSubscriptionRepository,
	balanceRepo balance.Repository,
	onChange workspace_addon.EntitlementChangeHandler,
) workspace_addon.PurchaseAddonUseCase {
	return &purchaseAddonUseCase{defs: defs, subs: subs, balanceRepo: balanceRepo, onChange: onChange, now: utcNow, newID: newPurchaseID}
}

func newPurchaseID() string { return uuid.NewString() }

func purchaseReference(subscriptionID, attemptID string) string {
	return "addon_purchase:" + subscriptionID + ":" + attemptID
}

func (uc *purchaseAddonUseCase) Execute(workspaceID string, input workspace_addon.PurchaseAddonInput) (*workspace_addon.AddonSubscription, error) {
	def, err := loadPurchasable(uc.defs, workspaceID, input)
	if err != nil {
		return nil, err
	}
	var bought *workspace_addon.AddonSubscription
	var charged *balance_usecase.ReferenceCharge
	err = uc.subs.WithUnitsLock(workspaceID, def.EntitlementKind, func(locked workspace_addon.AddonSubscriptionRepository) error {
		now := uc.now()
		plan, err := planPurchase(def, locked, workspaceID, input, now)
		if err != nil {
			return err
		}
		sub := plan.subscription(workspaceID, uc.newID(), now)
		if err := sub.Validate(); err != nil {
			return err
		}
		if plan.quote.ChargeMicros > 0 {
			charge := uc.charge(workspaceID, sub.ID, def, plan.quote)
			if err := uc.debit(charge); err != nil {
				return err
			}
			charged = &charge
		}
		if err := plan.save(locked, sub); err != nil {
			return err
		}
		bought = sub
		return nil
	})
	if err != nil {
		if charged != nil {
			return nil, uc.creditBack(*charged, err)
		}
		return nil, err
	}
	if uc.onChange != nil {
		_ = uc.onChange.OnEntitlementIncreased(workspaceID, def.EntitlementKind)
	}
	return bought, nil
}

func (uc *purchaseAddonUseCase) charge(workspaceID, subscriptionID string, def *workspace_addon.AddonDefinition, quote workspace_addon.PurchaseQuote) balance_usecase.ReferenceCharge {
	return balance_usecase.ReferenceCharge{
		WorkspaceID: workspaceID,
		ReferenceID: purchaseReference(subscriptionID, uc.newID()),
		ServiceType: balance.ServiceAddon,
		Price: workspace_pricing.PriceResult{
			PriceMicros:  quote.ChargeMicros,
			CostMicros:   quote.CostMicros,
			ProfitMicros: quote.ChargeMicros - quote.CostMicros,
		},
		Description:       "Addon: " + def.Name,
		RefundDescription: "Estorno addon: " + def.Name,
	}
}

func (uc *purchaseAddonUseCase) debit(charge balance_usecase.ReferenceCharge) error {
	if _, err := uc.balanceRepo.EnsureBalanceExists(charge.WorkspaceID, balanceCurrencyUSD); err != nil {
		return err
	}
	transaction, err := balance_usecase.DebitOnce(uc.balanceRepo, charge)
	if err != nil {
		return err
	}
	if transaction == nil {
		return workspace_addon.ErrPurchaseAlreadyRecorded
	}
	return nil
}

func (uc *purchaseAddonUseCase) creditBack(charge balance_usecase.ReferenceCharge, cause error) error {
	if err := balance_usecase.RefundOnce(uc.balanceRepo, charge); err != nil {
		log.Printf("[addon-purchase] workspace %s was charged under %s, the units were not saved (%v) and the credit back failed: %v", charge.WorkspaceID, charge.ReferenceID, cause, err)
		return errors.Join(cause, err)
	}
	return cause
}
