package billing_usecase

import (
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	billing "vozko/domain/billing"
	"vozko/domain/invoice"
	"vozko/domain/workspace"
	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
	workspace_pricing "vozko/domain/workspace/workspace_pricing"
)

const defaultEmitBatchSize = 500

type emitMonthlyInvoicesUseCase struct {
	subs          workspace_plan.SubscriptionRepository
	plans         workspace_plan.PlanReader
	addons        workspace_addon.AddonSubscriptionRepository
	workspaces    workspace.Repository
	pricing       workspace_pricing.Repository
	createInvoice invoice.CreateInvoiceUseCase
	notices       BillingNotices
	billingType   string
	dueDay        int
	batchSize     int
	now           clockFn
}

func NewEmitMonthlyInvoicesUseCase(
	subs workspace_plan.SubscriptionRepository,
	plans workspace_plan.PlanReader,
	addons workspace_addon.AddonSubscriptionRepository,
	workspaces workspace.Repository,
	pricing workspace_pricing.Repository,
	createInvoice invoice.CreateInvoiceUseCase,
	notices BillingNotices,
) *emitMonthlyInvoicesUseCase {
	return &emitMonthlyInvoicesUseCase{
		subs:          subs,
		plans:         plans,
		addons:        addons,
		workspaces:    workspaces,
		pricing:       pricing,
		createInvoice: createInvoice,
		notices:       notices,
		billingType:   "PIX",
		dueDay:        billing.DefaultDueDay,
		batchSize:     defaultEmitBatchSize,
		now:           brtNow,
	}
}

func (uc *emitMonthlyInvoicesUseCase) Execute() (int, error) {
	now := uc.now().In(billing.LocationBRT())
	if !billing.InEmitWindow(now) {
		return 0, nil
	}
	windowEnd := billing.NextAnchor(now, uc.dueDay)
	rate := uc.exchangeRate()

	emitted, err := uc.emitPlansDue(windowEnd, rate)
	if err != nil {
		return emitted, err
	}
	addonOnly, err := uc.emitAddonsDue(windowEnd, rate)
	return emitted + addonOnly, err
}

func (uc *emitMonthlyInvoicesUseCase) emitPlansDue(windowEnd time.Time, rate float64) (int, error) {
	emitted := 0
	afterID := ""
	for {
		page, err := uc.subs.ListActiveBillingDue(windowEnd, afterID, uc.batchSize)
		if err != nil {
			return emitted, err
		}
		if len(page) == 0 {
			break
		}
		for _, sub := range page {
			afterID = sub.ID
			uc.emitAndCount(&emitted, sub, true, windowEnd, rate)
		}
		if len(page) < uc.batchSize {
			break
		}
	}
	return emitted, nil
}

func (uc *emitMonthlyInvoicesUseCase) emitAddonsDue(windowEnd time.Time, rate float64) (int, error) {
	emitted := 0
	afterID := ""
	for {
		page, err := uc.addons.ListWorkspaceIDsWithAddonsDue(windowEnd, afterID, uc.batchSize)
		if err != nil {
			return emitted, err
		}
		if len(page) == 0 {
			break
		}
		for _, workspaceID := range page {
			afterID = workspaceID
			plan, billable := uc.addonOnlyPlan(workspaceID, windowEnd)
			if billable {
				uc.emitAndCount(&emitted, plan, false, windowEnd, rate)
			}
		}
		if len(page) < uc.batchSize {
			break
		}
	}
	return emitted, nil
}

func (uc *emitMonthlyInvoicesUseCase) addonOnlyPlan(workspaceID string, windowEnd time.Time) (*workspace_plan.WorkspaceSubscription, bool) {
	plan, err := uc.subs.GetLatestByWorkspaceID(workspaceID)
	if errors.Is(err, workspace_plan.ErrSubscriptionNotFound) {
		return nil, false
	}
	if err != nil {
		log.Printf("[billing-emit] workspace %s add-ons skipped, its plan could not be read: %v", workspaceID, err)
		return nil, false
	}
	if plan == nil || plan.Status != workspace_plan.SubscriptionStatusActive {
		return nil, false
	}
	if billing.DueOn(plan.CurrentPeriodEnd, windowEnd) {
		return nil, false
	}
	return plan, true
}

func (uc *emitMonthlyInvoicesUseCase) emitAndCount(emitted *int, sub *workspace_plan.WorkspaceSubscription, billPlan bool, windowEnd time.Time, rate float64) {
	created, err := uc.emitForWorkspace(sub, billPlan, windowEnd, rate)
	if err != nil {
		log.Printf("[billing-emit] workspace %s emit failed: %v", sub.WorkspaceID, err)
		return
	}
	if created {
		*emitted++
	}
}

func (uc *emitMonthlyInvoicesUseCase) exchangeRate() float64 {
	items, err := uc.pricing.ListDefaultPricingItems()
	if err != nil {
		log.Printf("[billing-emit] WARNING: exchange rate fetch failed, using fallback %.1f: %v", workspace_pricing.DefaultUSDToBRL, err)
		return workspace_pricing.DefaultUSDToBRL
	}
	return workspace_pricing.USDToBRLRate(items)
}

func (uc *emitMonthlyInvoicesUseCase) emitForWorkspace(sub *workspace_plan.WorkspaceSubscription, billPlan bool, anchor time.Time, rate float64) (bool, error) {
	var plan *workspace_plan.PlanDefinition
	planBRLCents := int64(0)
	cycle := workspace_plan.BillingCycleMonthly
	if billPlan {
		found, err := uc.plans.GetByID(sub.PlanDefinitionID)
		if err != nil {
			return false, fmt.Errorf("get plan: %w", err)
		}
		plan = found
		planBRLCents = sub.BillingCycle.TotalPriceBRLCents(plan.BasePriceBRLCents)
		cycle = sub.BillingCycle
	}

	addons, err := uc.addons.ListActiveByWorkspace(sub.WorkspaceID)
	if err != nil {
		return false, fmt.Errorf("list addons: %w", err)
	}

	renewed := make([]*workspace_addon.AddonSubscription, 0, len(addons))
	addonUSD := make([]int64, 0, len(addons))
	for _, a := range addons {
		if !a.DueBy(anchor) || a.QuantityAt(anchor) <= 0 {
			continue
		}
		renewed = append(renewed, a)
		addonUSD = append(addonUSD, a.UnitPriceMicros*int64(a.QuantityAt(anchor)))
	}

	totalBRL, creditableBRL := billing.MonthlyChargeBRL(planBRLCents, addonUSD, rate)
	if totalBRL <= 0 {
		return false, nil
	}

	lineItems := make([]invoice.InvoiceLineItem, 0, len(renewed)+1)
	if plan != nil && creditableBRL > 0 {
		lineItems = append(lineItems, invoice.InvoiceLineItem{
			Kind:       invoice.LineItemPlan,
			Label:      plan.Name,
			AmountBRL:  creditableBRL,
			Creditable: true,
		})
	}
	for _, a := range renewed {
		quantity := a.QuantityAt(anchor)
		lineBRL := math.Round(float64(a.UnitPriceMicros*int64(quantity))/1_000_000*rate*100) / 100
		lineItems = append(lineItems, invoice.InvoiceLineItem{
			Kind:      invoice.LineItemChannel,
			Label:     a.AddonKey,
			AmountBRL: lineBRL,
			Quantity:  quantity,
		})
	}

	ws, err := uc.workspaces.GetWorkspaceByID(sub.WorkspaceID)
	if err != nil {
		return false, fmt.Errorf("get workspace: %w", err)
	}
	if ws == nil || ws.OwnerID == "" {
		return false, fmt.Errorf("workspace %s has no owner to bill", sub.WorkspaceID)
	}

	key := fmt.Sprintf("monthly:%s:%s", sub.WorkspaceID, anchor.In(billing.LocationBRT()).Format("2006-01-02"))

	out, err := uc.createInvoice.Execute(invoice.CreateInvoiceInput{
		WorkspaceID:      sub.WorkspaceID,
		UserID:           ws.OwnerID,
		Purpose:          invoice.PurposeMonthlyBilling,
		PlanDefinitionID: sub.PlanDefinitionID,
		AmountBRL:        totalBRL,
		CreditableBRL:    creditableBRL,
		BillingType:      uc.billingType,
		BillingCycle:     string(cycle),
		IdempotencyKey:   key,
		DueDate:          &anchor,
		LineItems:        lineItems,
	})
	if err != nil {
		return false, fmt.Errorf("create invoice: %w", err)
	}
	uc.announce(out)
	return true, nil
}

func (uc *emitMonthlyInvoicesUseCase) announce(out *invoice.CreateInvoiceOutput) {
	if out == nil || out.Invoice == nil {
		return
	}
	if err := uc.notices.InvoiceIssued(out.Invoice); err != nil {
		log.Printf("[billing-emit] invoice %s was issued but its notice failed, the next run retries it: %v", out.Invoice.ID, err)
	}
}
