package workspace_addon

import (
	"errors"
	"testing"
	"time"

	billing "vozko/domain/billing"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

func brt(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, billing.LocationBRT())
}

func anchor(y int, m time.Month) time.Time {
	return time.Date(y, m, billing.DefaultDueDay, 0, 0, 0, 0, billing.LocationBRT())
}

func slots(quantity int) *AddonSubscription {
	return &AddonSubscription{
		ID: "s1", WorkspaceID: "ws", AddonDefinitionID: "d1", EntitlementKind: EntitlementSiteDomains,
		Quantity: quantity, UnitsPerQuantity: 1, BillingCycle: workspace_plan.BillingCycleMonthly,
		Status: workspace_plan.SubscriptionStatusActive, CurrentPeriodStart: anchor(2026, time.May), CurrentPeriodEnd: anchor(2026, time.June),
	}
}

func TestSiteDomainDefinitionsNeedAUnitCap(t *testing.T) {
	def := &AddonDefinition{Key: "dominios", Name: "Domínios", EntitlementKind: EntitlementSiteDomains, UnitsPerQuantity: 1, MonthlyPriceMicros: 1_000_000}
	if err := def.Validate(); !errors.Is(err, ErrMaxUnitsRequired) {
		t.Fatalf("a domain slot add-on without a cap must be refused, got %v", err)
	}
	def.MaxUnits = -1
	if err := def.Validate(); !errors.Is(err, ErrInvalidMaxUnits) {
		t.Fatalf("a negative cap must be refused, got %v", err)
	}
	def.MaxUnits = 20
	if err := def.Validate(); err != nil {
		t.Fatalf("a capped domain slot add-on is valid, got %v", err)
	}
	other := &AddonDefinition{Key: "filial", Name: "Filial", EntitlementKind: EntitlementBranches, UnitsPerQuantity: 1}
	if err := other.Validate(); err != nil {
		t.Fatalf("other kinds keep working without a cap, got %v", err)
	}
}

func TestUnitCapRefusesMoreThanTheMaximum(t *testing.T) {
	def := &AddonDefinition{MaxUnits: 5}
	if err := def.AllowsUnits(5); err != nil {
		t.Fatalf("exactly the cap is allowed, got %v", err)
	}
	if err := def.AllowsUnits(6); !errors.Is(err, ErrAddonUnitCapReached) {
		t.Fatalf("one past the cap must be refused, got %v", err)
	}
	if err := (&AddonDefinition{}).AllowsUnits(1000); err != nil {
		t.Fatalf("no cap means no limit, got %v", err)
	}
	if err := (&AddonDefinition{EntitlementKind: EntitlementSiteDomains}).AllowsUnits(1); !errors.Is(err, ErrMaxUnitsRequired) {
		t.Fatalf("a domain slot add-on saved without a cap sells nothing, got %v", err)
	}
}

func TestReductionStartsAtTheNextCycleAndNeverBelowUse(t *testing.T) {
	sub := slots(5)
	from := sub.ChangeDate(brt(2026, time.June, 10))
	if !from.Equal(anchor(2026, time.June)) {
		t.Fatalf("a reduction on Jun 10 starts at %s, want the Jun 23 renewal", from)
	}
	if err := sub.ScheduleQuantity(2, from, 3); !errors.Is(err, ErrUnitsInUse) {
		t.Fatalf("going below the 3 domains in use must be refused, got %v", err)
	}
	if err := sub.ScheduleQuantity(6, from, 0); !errors.Is(err, ErrQuantityIncreaseNeedsPurchase) {
		t.Fatalf("more slots are bought, never scheduled, got %v", err)
	}
	if err := sub.ScheduleQuantity(3, from, 3); err != nil {
		t.Fatalf("reducing to the slots in use is allowed, got %v", err)
	}
	if got := sub.QuantityAt(brt(2026, time.June, 22)); got != 5 {
		t.Fatalf("the paid cycle keeps 5 slots, got %d", got)
	}
	if got := sub.QuantityAt(anchor(2026, time.June)); got != 3 {
		t.Fatalf("the next cycle has 3 slots, got %d", got)
	}
	if err := sub.ScheduleQuantity(5, from, 0); err != nil || sub.ScheduledQuantity != nil {
		t.Fatalf("scheduling the current quantity undoes the reduction, got %v %v", err, sub.ScheduledQuantity)
	}
}

func TestChangesAfterTheInvoiceWasIssuedWaitOneMoreCycle(t *testing.T) {
	sub := slots(4)
	if from := sub.ChangeDate(brt(2026, time.June, 19)); !from.Equal(anchor(2026, time.July)) {
		t.Fatalf("on Jun 19 the Jun 23 invoice already bills 4 slots, so a change starts Jul 23, got %s", from)
	}
	paidAhead := slots(4)
	paidAhead.CurrentPeriodEnd = anchor(2026, time.July)
	if from := paidAhead.ChangeDate(brt(2026, time.June, 10)); !from.Equal(anchor(2026, time.July)) {
		t.Fatalf("a cycle already paid is never cut short, got %s", from)
	}
}

func TestCancellationTakesNoNewDomainsAndIsNeverRenewed(t *testing.T) {
	sub := slots(2)
	now := brt(2026, time.June, 10)
	if err := sub.ScheduleCancellation(sub.ChangeDate(now), now, 0); err != nil {
		t.Fatal(err)
	}
	if sub.CancelledAt == nil {
		t.Fatal("the cancellation must be recorded")
	}
	if got := sub.GrantedUnitsAt(brt(2026, time.June, 22)); got != 0 {
		t.Fatalf("a cancelled slot add-on takes no new domains while it runs out, got %d", got)
	}
	sub.RenewForInvoice(anchor(2026, time.June), anchor(2026, time.June), billing.DefaultDueDay)
	if sub.Status != workspace_plan.SubscriptionStatusExpired {
		t.Fatalf("a cancelled add-on is never renewed, got %s", sub.Status)
	}
}

func TestRenewalAppliesTheScheduledQuantity(t *testing.T) {
	sub := slots(5)
	from := sub.ChangeDate(brt(2026, time.June, 10))
	if err := sub.ScheduleQuantity(3, from, 1); err != nil {
		t.Fatal(err)
	}
	sub.RenewForInvoice(anchor(2026, time.June), anchor(2026, time.June), billing.DefaultDueDay)
	if sub.Quantity != 3 || sub.ScheduledQuantity != nil || sub.ScheduledFrom != nil {
		t.Fatalf("renewal must apply the reduction once, got quantity %d schedule %v %v", sub.Quantity, sub.ScheduledQuantity, sub.ScheduledFrom)
	}
	if sub.Status != workspace_plan.SubscriptionStatusActive || !sub.CurrentPeriodEnd.Equal(anchor(2026, time.July)) {
		t.Fatalf("the renewed add-on runs until Jul 23, got %s %s", sub.Status, sub.CurrentPeriodEnd)
	}
}

func TestRenewalKeepsAScheduleMeantForALaterCycle(t *testing.T) {
	sub := slots(5)
	later := anchor(2026, time.July)
	if err := sub.ScheduleQuantity(2, later, 0); err != nil {
		t.Fatal(err)
	}
	sub.RenewForInvoice(anchor(2026, time.June), anchor(2026, time.June), billing.DefaultDueDay)
	if sub.Quantity != 5 || sub.ScheduledQuantity == nil || *sub.ScheduledQuantity != 2 {
		t.Fatalf("the Jun 23 renewal keeps 5 slots and the Jul 23 reduction, got %d %v", sub.Quantity, sub.ScheduledQuantity)
	}
}

func TestBuyingMoreKeepsAReductionAndUndoesACancellation(t *testing.T) {
	sub := slots(5)
	if err := sub.ScheduleQuantity(3, anchor(2026, time.June), 0); err != nil {
		t.Fatal(err)
	}
	sub.AddQuantity(2)
	if sub.Quantity != 7 || *sub.ScheduledQuantity != 5 {
		t.Fatalf("2 more slots on top of a reduction to 3 give 7 now and 5 next cycle, got %d %d", sub.Quantity, *sub.ScheduledQuantity)
	}
	cancelled := slots(1)
	now := brt(2026, time.June, 10)
	if err := cancelled.ScheduleCancellation(anchor(2026, time.June), now, 0); err != nil {
		t.Fatal(err)
	}
	cancelled.AddQuantity(1)
	if cancelled.CancelledAt != nil || cancelled.ScheduledQuantity != nil || cancelled.Quantity != 2 {
		t.Fatalf("buying again undoes the cancellation, got %v %v %d", cancelled.CancelledAt, cancelled.ScheduledQuantity, cancelled.Quantity)
	}
}

func TestPurchaseQuoteChargesOnlyTheDaysLeft(t *testing.T) {
	def := &AddonDefinition{MonthlyPriceMicros: 1_000_000, MonthlyCostMicros: 100_000}
	quote, err := QuotePurchase(def, workspace_plan.BillingCycleMonthly, 3, brt(2026, time.June, 10))
	if err != nil {
		t.Fatal(err)
	}
	if quote.RecurringMicros != 3_000_000 {
		t.Fatalf("recurring = %d, want 3 slots a month", quote.RecurringMicros)
	}
	if quote.ChargeMicros != 1_258_065 || quote.CostMicros != 125_806 {
		t.Fatalf("charge now = %d cost %d, want 13 of 31 days", quote.ChargeMicros, quote.CostMicros)
	}
	if !quote.Prorated || quote.ProratedDays != 13 || !quote.PeriodEnd.Equal(anchor(2026, time.June)) {
		t.Fatalf("quote = %+v, want 13 prorated days until Jun 23", quote)
	}
}
