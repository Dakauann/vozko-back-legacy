package workspace_addon_usecase

import (
	"errors"
	"testing"
	"time"

	billing "vozko/domain/billing"
	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

type fixedUsage struct {
	units int
	err   error
}

func (u fixedUsage) UnitsInUse(string) (int, error) { return u.units, u.err }

func slotSubscription(quantity int) *workspace_addon.AddonSubscription {
	end := time.Date(2026, time.July, billing.DefaultDueDay, 0, 0, 0, 0, billing.LocationBRT())
	return &workspace_addon.AddonSubscription{
		ID: "slots", WorkspaceID: "ws", AddonDefinitionID: "d1", AddonKey: "d1", EntitlementKind: workspace_addon.EntitlementSiteDomains,
		Quantity: quantity, UnitsPerQuantity: 1, BillingCycle: workspace_plan.BillingCycleMonthly, Status: workspace_plan.SubscriptionStatusActive,
		UnitPriceMicros: 1_000_000, CurrentPeriodStart: end.AddDate(0, -1, 0), CurrentPeriodEnd: end,
	}
}

func cappedSlots(maxUnits int) *workspace_addon.AddonDefinition {
	def := activeDef("d1", workspace_addon.EntitlementSiteDomains, 1_000_000)
	def.MaxUnits = maxUnits
	return def
}

func TestReducingSlotsStartsAtTheNextCycleAndKeepsTheDomainsInUse(t *testing.T) {
	sub := slotSubscription(4)
	subs := newFakeSubRepo(sub)
	uc := &scheduleAddonQuantityUseCase{subs: subs, usage: map[workspace_addon.EntitlementKind]workspace_addon.UnitUsage{workspace_addon.EntitlementSiteDomains: fixedUsage{units: 2}}, now: testClock}

	if _, err := uc.Execute("ws", "slots", 1); !errors.Is(err, workspace_addon.ErrUnitsInUse) {
		t.Fatalf("2 domains are connected, so 1 slot must be refused, got %v", err)
	}
	got, err := uc.Execute("ws", "slots", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Quantity != 4 || got.ScheduledQuantity == nil || *got.ScheduledQuantity != 2 {
		t.Fatalf("the paid cycle keeps 4 slots and the next has 2, got %d %v", got.Quantity, got.ScheduledQuantity)
	}
	if !got.ScheduledFrom.Equal(sub.CurrentPeriodEnd) {
		t.Fatalf("the reduction starts when the paid period ends, got %s", got.ScheduledFrom)
	}
}

func TestReducingRefusesOtherWorkspacesAndUnknownUsage(t *testing.T) {
	subs := newFakeSubRepo(slotSubscription(3))
	uc := &scheduleAddonQuantityUseCase{subs: subs, usage: map[workspace_addon.EntitlementKind]workspace_addon.UnitUsage{workspace_addon.EntitlementSiteDomains: fixedUsage{err: errors.New("db down")}}, now: testClock}
	if _, err := uc.Execute("other", "slots", 1); !errors.Is(err, workspace_addon.ErrAddonSubscriptionNotFound) {
		t.Fatalf("another workspace must not see the add-on, got %v", err)
	}
	if _, err := uc.Execute("ws", "slots", 1); err == nil {
		t.Fatal("an unreadable usage must refuse the reduction")
	}
	none := &scheduleAddonQuantityUseCase{subs: subs, now: testClock}
	if _, err := none.Execute("ws", "slots", 1); !errors.Is(err, workspace_addon.ErrReductionUnsupported) {
		t.Fatalf("a kind without a usage reader cannot be reduced, got %v", err)
	}
}

func TestCancellingRunsTheSlotsOutWithoutTakingNewDomains(t *testing.T) {
	sub := slotSubscription(2)
	subs := newFakeSubRepo(sub)
	uc := &cancelAddonSubscriptionUseCase{subs: subs, usage: unitUsages{workspace_addon.EntitlementSiteDomains: fixedUsage{}}, now: testClock}
	got, err := uc.Execute("ws", "slots")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != workspace_plan.SubscriptionStatusActive || got.CancelledAt == nil {
		t.Fatalf("a cancelled add-on stays active until its period ends, got %s %v", got.Status, got.CancelledAt)
	}
	if !got.ScheduledFrom.Equal(sub.CurrentPeriodEnd) || got.QuantityAt(sub.CurrentPeriodEnd.Add(-time.Hour)) != 2 || got.QuantityAt(sub.CurrentPeriodEnd) != 0 {
		t.Fatal("the paid slots run until the paid period ends and are never renewed")
	}
	if got.GrantedUnitsAt(sub.CurrentPeriodEnd.Add(-time.Hour)) != 0 {
		t.Fatal("a cancelled slot add-on takes no new domains")
	}
}

func TestPurchaseRefusesSlotsPastTheCap(t *testing.T) {
	subs := newFakeSubRepo(slotSubscription(4))
	defs := newFakeDefRepo(cappedSlots(5))
	bal := newFakeBalanceRepo(100_000_000)
	uc := &purchaseAddonUseCase{defs: defs, subs: subs, balanceRepo: bal, now: testClock, newID: newPurchaseID}
	if _, err := uc.Execute("ws", workspace_addon.PurchaseAddonInput{AddonDefinitionID: "d1", Quantity: 2}); !errors.Is(err, workspace_addon.ErrAddonUnitCapReached) {
		t.Fatalf("4 owned plus 2 passes the cap of 5, got %v", err)
	}
	if bal.amount != 100_000_000 {
		t.Fatalf("a refused purchase charges nothing, balance %d", bal.amount)
	}
	preview := &previewAddonPurchaseUseCase{defs: defs, subs: subs, now: testClock}
	if _, err := preview.Execute("ws", workspace_addon.PurchaseAddonInput{AddonDefinitionID: "d1", Quantity: 2}); !errors.Is(err, workspace_addon.ErrAddonUnitCapReached) {
		t.Fatalf("the preview refuses the same purchase, got %v", err)
	}
}

func TestSlotsExistOnlyAfterTheWalletPaidForThem(t *testing.T) {
	subs := newFakeSubRepo()
	defs := newFakeDefRepo(cappedSlots(10))
	bal := newFakeBalanceRepo(100)
	uc := &purchaseAddonUseCase{defs: defs, subs: subs, balanceRepo: bal, now: testClock, newID: newPurchaseID}
	if _, err := uc.Execute("ws", workspace_addon.PurchaseAddonInput{AddonDefinitionID: "d1", Quantity: 1}); err == nil {
		t.Fatal("a wallet that cannot pay the pro rata must refuse the purchase")
	}
	if len(subs.subs) != 0 {
		t.Fatalf("no slot exists without payment, got %d subscriptions", len(subs.subs))
	}
}
