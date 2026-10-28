package workspace_addon_usecase

import (
	"errors"
	"math"
	"strings"
	"sync"
	"testing"

	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

func slotPurchase(quantity int) workspace_addon.PurchaseAddonInput {
	return workspace_addon.PurchaseAddonInput{AddonDefinitionID: "d1", Quantity: quantity, BillingCycle: workspace_plan.BillingCycleMonthly}
}

func TestRepeatPurchasesInTheSameSecondAreEachCharged(t *testing.T) {
	subs := newFakeSubRepo(slotSubscription(1))
	bal := newFakeBalanceRepo(100_000_000)
	uc := &purchaseAddonUseCase{defs: newFakeDefRepo(cappedSlots(10)), subs: subs, balanceRepo: bal, now: testClock, newID: newPurchaseID}

	for i := 0; i < 2; i++ {
		if _, err := uc.Execute("ws", slotPurchase(1)); err != nil {
			t.Fatalf("purchase %d: %v", i, err)
		}
	}
	if got := subs.subs["slots"].Quantity; got != 3 {
		t.Fatalf("two paid purchases on top of 1 slot give 3, got %d", got)
	}
	if len(bal.debits) != 2 || *bal.debits[0].ReferenceID == *bal.debits[1].ReferenceID {
		t.Fatalf("each purchase needs its own charge, got %+v", bal.debits)
	}
	if !bal.debits[0].OncePerReference {
		t.Fatal("the charge must be recorded once per reference")
	}
}

func TestAPurchaseReferenceAlreadyInTheLedgerGrantsNothing(t *testing.T) {
	subs := newFakeSubRepo(slotSubscription(1))
	bal := newFakeBalanceRepo(100_000_000)
	uc := &purchaseAddonUseCase{defs: newFakeDefRepo(cappedSlots(10)), subs: subs, balanceRepo: bal, now: testClock, newID: func() string { return "attempt" }}
	bal.byRef[purchaseReference("slots", "attempt")] = true

	if _, err := uc.Execute("ws", slotPurchase(2)); !errors.Is(err, workspace_addon.ErrPurchaseAlreadyRecorded) {
		t.Fatalf("an already recorded charge must refuse the purchase, got %v", err)
	}
	if got := subs.subs["slots"].Quantity; got != 1 || len(bal.debits) != 0 {
		t.Fatalf("nothing is granted without a new charge, quantity %d debits %d", got, len(bal.debits))
	}
}

func TestThePurchaseIsPlannedInsideTheUnitsLock(t *testing.T) {
	stale := newFakeSubRepo(slotSubscription(1))
	stale.lockedView = newFakeSubRepo(slotSubscription(4))
	bal := newFakeBalanceRepo(100_000_000)
	uc := &purchaseAddonUseCase{defs: newFakeDefRepo(cappedSlots(5)), subs: stale, balanceRepo: bal, now: testClock, newID: newPurchaseID}

	if _, err := uc.Execute("ws", slotPurchase(2)); !errors.Is(err, workspace_addon.ErrAddonUnitCapReached) {
		t.Fatalf("the cap must be checked against the locked rows, got %v", err)
	}
	got, err := uc.Execute("ws", slotPurchase(1))
	if err != nil {
		t.Fatal(err)
	}
	if got.Quantity != 5 || stale.lockedView.subs["slots"].Quantity != 5 || stale.subs["slots"].Quantity != 1 {
		t.Fatalf("the slot is added to the locked row, got %d", got.Quantity)
	}
	if len(stale.locks) != 2 || stale.locks[0] != "ws:site_domains" {
		t.Fatalf("each purchase takes the workspace and kind lock, got %v", stale.locks)
	}
}

func TestConcurrentPurchasesNeverPassTheCap(t *testing.T) {
	subs := newFakeSubRepo()
	bal := newFakeBalanceRepo(1_000_000_000)
	uc := &purchaseAddonUseCase{defs: newFakeDefRepo(cappedSlots(5)), subs: subs, balanceRepo: bal, now: testClock, newID: newPurchaseID}

	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := uc.Execute("ws", slotPurchase(1)); err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	total := 0
	for _, s := range subs.subs {
		total += s.Quantity
	}
	if succeeded != 5 || total != 5 || len(bal.debits) != 5 {
		t.Fatalf("exactly 5 paid slots fit the cap, succeeded %d owned %d debits %d", succeeded, total, len(bal.debits))
	}
}

func TestSlotsAreNeverSoldOnAnotherCycle(t *testing.T) {
	subs := newFakeSubRepo()
	defs := newFakeDefRepo(cappedSlots(10))
	bal := newFakeBalanceRepo(100_000_000)
	uc := &purchaseAddonUseCase{defs: defs, subs: subs, balanceRepo: bal, now: testClock, newID: newPurchaseID}
	annual := workspace_addon.PurchaseAddonInput{AddonDefinitionID: "d1", Quantity: 1, BillingCycle: workspace_plan.BillingCycleAnnual}

	if _, err := uc.Execute("ws", annual); !errors.Is(err, workspace_addon.ErrMonthlyOnly) {
		t.Fatalf("annual slots must be refused, got %v", err)
	}
	preview := &previewAddonPurchaseUseCase{defs: defs, subs: subs, now: testClock}
	if _, err := preview.Execute("ws", annual); !errors.Is(err, workspace_addon.ErrMonthlyOnly) {
		t.Fatalf("the preview refuses the same cycle, got %v", err)
	}
	if len(subs.subs) != 0 || len(bal.debits) != 0 {
		t.Fatal("a refused cycle grants and charges nothing")
	}
}

func TestSlotsWithoutAMonthlyPriceAreNeverGranted(t *testing.T) {
	def := cappedSlots(10)
	def.MonthlyPriceMicros = 0
	subs := newFakeSubRepo()
	bal := newFakeBalanceRepo(100_000_000)
	uc := &purchaseAddonUseCase{defs: newFakeDefRepo(def), subs: subs, balanceRepo: bal, now: testClock, newID: newPurchaseID}

	if _, err := uc.Execute("ws", slotPurchase(1)); !errors.Is(err, workspace_addon.ErrPurchaseChargeInvalid) {
		t.Fatalf("a slot without a charge must be refused, got %v", err)
	}
	if len(subs.subs) != 0 {
		t.Fatal("no free slots")
	}
}

func TestQuantitiesAreBoundedBeforeAnyMath(t *testing.T) {
	subs := newFakeSubRepo()
	bal := newFakeBalanceRepo(100_000_000)
	defs := newFakeDefRepo(cappedSlots(10), activeDef("channels", workspace_addon.EntitlementCallChannels, 5_000_000))
	uc := &purchaseAddonUseCase{defs: defs, subs: subs, balanceRepo: bal, now: testClock, newID: newPurchaseID}

	for _, quantity := range []int{0, -3} {
		if _, err := uc.Execute("ws", slotPurchase(quantity)); !errors.Is(err, workspace_addon.ErrInvalidQuantity) {
			t.Fatalf("quantity %d must be refused, got %v", quantity, err)
		}
	}
	if _, err := uc.Execute("ws", slotPurchase(11)); !errors.Is(err, workspace_addon.ErrAddonUnitCapReached) {
		t.Fatalf("a quantity above the cap must be refused, got %v", err)
	}
	huge := workspace_addon.PurchaseAddonInput{AddonDefinitionID: "channels", Quantity: math.MaxInt, BillingCycle: workspace_plan.BillingCycleMonthly}
	if _, err := uc.Execute("ws", huge); !errors.Is(err, workspace_addon.ErrInvalidQuantity) {
		t.Fatalf("a quantity whose price overflows must be refused, got %v", err)
	}
	if len(subs.subs) != 0 || len(bal.debits) != 0 {
		t.Fatal("refused quantities grant and charge nothing")
	}
}

func TestAFailedSaveCreditsTheChargeBackUnderItsReference(t *testing.T) {
	subs := newFakeSubRepo(slotSubscription(1))
	subs.updateErr = errors.New("write failed")
	bal := newFakeBalanceRepo(100_000_000)
	uc := &purchaseAddonUseCase{defs: newFakeDefRepo(cappedSlots(10)), subs: subs, balanceRepo: bal, now: testClock, newID: newPurchaseID}

	if _, err := uc.Execute("ws", slotPurchase(1)); err == nil {
		t.Fatal("a failed save must surface")
	}
	if bal.amount != 100_000_000 || len(bal.credits) != 1 {
		t.Fatalf("the charge must come back, wallet %d credits %d", bal.amount, len(bal.credits))
	}
	credit := bal.credits[0]
	if credit.RefundOf == nil || *credit.RefundOf != *bal.debits[0].ReferenceID || !credit.OncePerReference || !strings.HasPrefix(*credit.ReferenceID, "refund:") {
		t.Fatalf("the credit must point at the charge it reverses, got %+v", credit)
	}
}

func TestCancellingSlotsIsGuardedByTheDomainsInUse(t *testing.T) {
	usage := func(u fixedUsage) map[workspace_addon.EntitlementKind]workspace_addon.UnitUsage {
		return map[workspace_addon.EntitlementKind]workspace_addon.UnitUsage{workspace_addon.EntitlementSiteDomains: u}
	}
	inUse := &cancelAddonSubscriptionUseCase{subs: newFakeSubRepo(slotSubscription(2)), usage: usage(fixedUsage{units: 1}), now: testClock}
	if _, err := inUse.Execute("ws", "slots"); !errors.Is(err, workspace_addon.ErrUnitsInUse) {
		t.Fatalf("a connected domain blocks the cancellation, got %v", err)
	}
	unknown := &cancelAddonSubscriptionUseCase{subs: newFakeSubRepo(slotSubscription(2)), usage: usage(fixedUsage{err: errors.New("db down")}), now: testClock}
	if _, err := unknown.Execute("ws", "slots"); err == nil {
		t.Fatal("an unreadable usage must refuse the cancellation")
	}
	unwired := &cancelAddonSubscriptionUseCase{subs: newFakeSubRepo(slotSubscription(2)), now: testClock}
	if _, err := unwired.Execute("ws", "slots"); !errors.Is(err, workspace_addon.ErrReductionUnsupported) {
		t.Fatalf("slots without a usage reader cannot be cancelled, got %v", err)
	}
	phones := slotSubscription(2)
	phones.EntitlementKind = workspace_addon.EntitlementWhatsAppBusinessPhones
	other := &cancelAddonSubscriptionUseCase{subs: newFakeSubRepo(phones), now: testClock}
	if got, err := other.Execute("ws", "slots"); err != nil || got.CancelledAt == nil {
		t.Fatalf("other kinds keep cancelling as before, got %v", err)
	}
}

func TestReducingSlotsLimitsNewDomainsAtOnce(t *testing.T) {
	sub := slotSubscription(4)
	sub.CurrentPeriodEnd = fixedNow.AddDate(0, 1, 0)
	subs := newFakeSubRepo(sub)
	schedule := &scheduleAddonQuantityUseCase{subs: subs, usage: map[workspace_addon.EntitlementKind]workspace_addon.UnitUsage{workspace_addon.EntitlementSiteDomains: fixedUsage{units: 2}}, now: testClock}
	if _, err := schedule.Execute("ws", "slots", 2); err != nil {
		t.Fatal(err)
	}
	resolver := &entitlementResolver{subscriptions: activeSubscription(), plans: &fakePlanReader{plan: &workspace_plan.PlanDefinition{ID: "plan-1"}}, addons: subs, now: testClock}
	if total, err := resolver.Resolve("ws", workspace_addon.EntitlementSiteDomains); err != nil || total != 2 {
		t.Fatalf("after scheduling 2 slots only 2 domains fit, got %d %v", total, err)
	}
	if len(subs.locks) != 1 {
		t.Fatalf("a reduction takes the units lock, got %v", subs.locks)
	}
}
