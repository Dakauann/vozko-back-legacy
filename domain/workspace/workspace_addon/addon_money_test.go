package workspace_addon

import (
	"errors"
	"math"
	"testing"
	"time"

	billing "vozko/domain/billing"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

func slotDefinition() *AddonDefinition {
	return &AddonDefinition{
		Key: "dominios", Name: "Domínios", EntitlementKind: EntitlementSiteDomains, UnitsPerQuantity: 1,
		MonthlyPriceMicros: 1_000_000, MaxUnits: 10, IsActive: true,
	}
}

func TestSlotDefinitionsNeedAMonthlyPrice(t *testing.T) {
	def := slotDefinition()
	def.MonthlyPriceMicros = 0
	def.AnnualPriceMicros = 9_000_000
	if err := def.Validate(); !errors.Is(err, ErrMonthlyPriceRequired) {
		t.Fatalf("a slot add-on without a monthly price must be refused, got %v", err)
	}
	other := &AddonDefinition{Key: "filial", Name: "Filial", EntitlementKind: EntitlementBranches, UnitsPerQuantity: 1}
	if err := other.Validate(); err != nil {
		t.Fatalf("free add-ons of other kinds stay valid, got %v", err)
	}
}

func TestSlotsAreSoldMonthlyOnly(t *testing.T) {
	def := slotDefinition()
	for _, requested := range []workspace_plan.BillingCycle{"", workspace_plan.BillingCycleMonthly} {
		if cycle, err := def.PurchaseCycle(requested); err != nil || cycle != workspace_plan.BillingCycleMonthly {
			t.Fatalf("%q must buy monthly slots, got %q %v", requested, cycle, err)
		}
	}
	for _, requested := range []workspace_plan.BillingCycle{workspace_plan.BillingCycleAnnual, "weekly"} {
		if _, err := def.PurchaseCycle(requested); !errors.Is(err, ErrMonthlyOnly) {
			t.Fatalf("%q must be refused for slots, got %v", requested, err)
		}
	}
	channels := &AddonDefinition{EntitlementKind: EntitlementCallChannels}
	if cycle, err := channels.PurchaseCycle(workspace_plan.BillingCycleAnnual); err != nil || cycle != workspace_plan.BillingCycleAnnual {
		t.Fatalf("other kinds keep the annual cycle, got %q %v", cycle, err)
	}
}

func TestQuantityIsCheckedBeforeAnyMultiplication(t *testing.T) {
	def := slotDefinition()
	for _, quantity := range []int{0, -1} {
		if _, err := def.UnitsFor(quantity); !errors.Is(err, ErrInvalidQuantity) {
			t.Fatalf("quantity %d must be refused, got %v", quantity, err)
		}
	}
	if _, err := def.UnitsFor(11); !errors.Is(err, ErrAddonUnitCapReached) {
		t.Fatalf("a quantity above the cap is refused before multiplying, got %v", err)
	}
	if units, err := def.UnitsFor(3); err != nil || units != 3 {
		t.Fatalf("3 slots are 3 units, got %d %v", units, err)
	}
	wide := &AddonDefinition{EntitlementKind: EntitlementCallChannels, UnitsPerQuantity: 1 << 20}
	if _, err := wide.UnitsFor(math.MaxInt / 2); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("a quantity whose units overflow must be refused, got %v", err)
	}
	if _, err := TotalUnits(math.MaxInt, []*AddonSubscription{{Quantity: 1, UnitsPerQuantity: 1}}); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("an owned total that overflows must be refused, got %v", err)
	}
}

func TestAPricedPurchaseNeverQuotesAFreeCharge(t *testing.T) {
	at := brt(2026, time.June, 10)
	free := slotDefinition()
	free.MonthlyPriceMicros = 0
	if _, err := QuotePurchase(free, workspace_plan.BillingCycleMonthly, 1, at); !errors.Is(err, ErrPurchaseChargeInvalid) {
		t.Fatalf("slots never activate without a charge, got %v", err)
	}
	pricey := &AddonDefinition{EntitlementKind: EntitlementCallChannels, MonthlyPriceMicros: math.MaxInt64 / 2}
	if _, err := QuotePurchase(pricey, workspace_plan.BillingCycleMonthly, 3, at); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("a price times quantity that overflows must be refused, got %v", err)
	}
	tiny := &AddonDefinition{EntitlementKind: EntitlementCallChannels, MonthlyPriceMicros: 1}
	if _, err := QuotePurchase(tiny, workspace_plan.BillingCycleMonthly, 1, at); !errors.Is(err, ErrPurchaseChargeInvalid) {
		t.Fatalf("a priced add-on whose pro rata rounds to zero must be refused, got %v", err)
	}
	gratis := &AddonDefinition{EntitlementKind: EntitlementCallChannels}
	if quote, err := QuotePurchase(gratis, workspace_plan.BillingCycleMonthly, 1, at); err != nil || quote.ChargeMicros != 0 {
		t.Fatalf("a free add-on of another kind still quotes nothing, got %+v %v", quote, err)
	}
}

func TestSlotsPastTheAccessEndOfTheirPeriodGrantNothing(t *testing.T) {
	sub := slots(3)
	if got := sub.GrantedUnitsAt(brt(2026, time.June, 25)); got != 3 {
		t.Fatalf("the invoice that renews Jun 23 is payable until the cutoff, got %d", got)
	}
	if got := sub.GrantedUnitsAt(brt(2026, time.June, 28)); got != 0 {
		t.Fatalf("a period nobody renewed grants nothing after the cutoff, got %d", got)
	}
	unset := slots(3)
	unset.CurrentPeriodEnd = time.Time{}
	if got := unset.GrantedUnitsAt(brt(2026, time.June, 1)); got != 0 {
		t.Fatalf("a subscription without a period end grants nothing, got %d", got)
	}
}

func TestAScheduledReductionCapsWhatCanBeAddedNow(t *testing.T) {
	sub := slots(4)
	if err := sub.ScheduleQuantity(2, anchor(2026, time.June), 2); err != nil {
		t.Fatal(err)
	}
	if got := sub.GrantedUnitsAt(brt(2026, time.June, 10)); got != 2 {
		t.Fatalf("after scheduling 2 slots no third domain can be added, got %d", got)
	}
	cancelled := slots(4)
	now := brt(2026, time.June, 10)
	if err := cancelled.ScheduleCancellation(anchor(2026, time.June), now, 0); err != nil {
		t.Fatal(err)
	}
	if got := cancelled.GrantedUnitsAt(now); got != 0 {
		t.Fatalf("a cancelled slot add-on takes no new domains, got %d", got)
	}
	phones := slots(4)
	phones.EntitlementKind = EntitlementWhatsAppBusinessPhones
	if err := phones.ScheduleQuantity(2, anchor(2026, time.June), 0); err != nil {
		t.Fatal(err)
	}
	if got := phones.GrantedUnitsAt(now); got != 4 {
		t.Fatalf("other kinds keep the paid units until the change date, got %d", got)
	}
}

func TestCancellingIsAReductionToZero(t *testing.T) {
	sub := slots(2)
	now := brt(2026, time.June, 10)
	if err := sub.ScheduleCancellation(anchor(2026, time.June), now, 1); !errors.Is(err, ErrUnitsInUse) {
		t.Fatalf("a domain in use blocks the cancellation, got %v", err)
	}
	if sub.CancelledAt != nil || sub.ScheduledQuantity != nil {
		t.Fatal("a refused cancellation changes nothing")
	}
}

func TestResendingTheCurrentQuantityUndoesTheCancellation(t *testing.T) {
	sub := slots(2)
	now := brt(2026, time.June, 10)
	if err := sub.ScheduleCancellation(anchor(2026, time.June), now, 0); err != nil {
		t.Fatal(err)
	}
	if err := sub.ScheduleQuantity(2, anchor(2026, time.June), 0); err != nil {
		t.Fatal(err)
	}
	if sub.CancelledAt != nil || sub.ScheduledQuantity != nil {
		t.Fatalf("keeping the quantity must clear the cancellation, got %v %v", sub.CancelledAt, sub.ScheduledQuantity)
	}
	reduced := slots(3)
	if err := reduced.ScheduleCancellation(anchor(2026, time.June), now, 0); err != nil {
		t.Fatal(err)
	}
	if err := reduced.ScheduleQuantity(1, anchor(2026, time.June), 0); err != nil {
		t.Fatal(err)
	}
	if reduced.CancelledAt != nil {
		t.Fatal("a reduction replaces the cancellation")
	}
}

func TestAPaidInvoiceRenewsOnlyTheCycleItBilled(t *testing.T) {
	due := anchor(2026, time.June)
	billed := slots(2)
	if !billed.RenewForInvoice(due, brt(2026, time.June, 21), billing.DefaultDueDay) {
		t.Fatal("the add-on billed on Jun 23 must renew")
	}
	if !billed.CurrentPeriodStart.Equal(due) || !billed.CurrentPeriodEnd.Equal(anchor(2026, time.July)) {
		t.Fatalf("the paid cycle is Jun 23 to Jul 23, got %s to %s", billed.CurrentPeriodStart, billed.CurrentPeriodEnd)
	}
	if billed.RenewForInvoice(due, brt(2026, time.June, 22), billing.DefaultDueDay) {
		t.Fatal("paying the same invoice twice must not renew again")
	}
	if !billed.CurrentPeriodEnd.Equal(anchor(2026, time.July)) {
		t.Fatalf("a repeated payment leaves Jul 23, got %s", billed.CurrentPeriodEnd)
	}
}

func TestASlotBoughtAfterTheInvoiceWentOutIsNotPushedAhead(t *testing.T) {
	due := anchor(2026, time.June)
	sub := slots(1)
	quote, err := QuotePurchase(slotDefinition(), workspace_plan.BillingCycleMonthly, 1, brt(2026, time.June, 20))
	if err != nil {
		t.Fatal(err)
	}
	sub.TopUp(slotDefinition(), 1, workspace_plan.BillingCycleMonthly, quote, brt(2026, time.June, 20))
	if !sub.CurrentPeriodEnd.Equal(anchor(2026, time.July)) {
		t.Fatalf("a slot bought on Jun 20 is paid to Jul 23, got %s", sub.CurrentPeriodEnd)
	}
	sub.RenewForInvoice(due, brt(2026, time.June, 21), billing.DefaultDueDay)
	if !sub.CurrentPeriodEnd.Equal(anchor(2026, time.July)) {
		t.Fatalf("paying the Jun 23 invoice afterwards must not reach Aug 23, got %s", sub.CurrentPeriodEnd)
	}
	if from := sub.ChangeDate(brt(2026, time.June, 25)); !from.Equal(anchor(2026, time.July)) {
		t.Fatalf("a cancellation then stops at Jul 23, got %s", from)
	}
}

func TestPayingFirstThenBuyingEndsOnTheSameAnchor(t *testing.T) {
	due := anchor(2026, time.June)
	sub := slots(1)
	sub.RenewForInvoice(due, brt(2026, time.June, 19), billing.DefaultDueDay)
	quote, err := QuotePurchase(slotDefinition(), workspace_plan.BillingCycleMonthly, 1, brt(2026, time.June, 20))
	if err != nil {
		t.Fatal(err)
	}
	sub.TopUp(slotDefinition(), 1, workspace_plan.BillingCycleMonthly, quote, brt(2026, time.June, 20))
	if sub.Quantity != 2 || !sub.CurrentPeriodEnd.Equal(anchor(2026, time.July)) {
		t.Fatalf("both orders end on Jul 23 with 2 slots, got %d %s", sub.Quantity, sub.CurrentPeriodEnd)
	}
}

func TestAStalePeriodPaidEarlyGetsTheWholeBilledCycle(t *testing.T) {
	sub := slots(1)
	sub.CurrentPeriodEnd = anchor(2026, time.February)
	sub.RenewForInvoice(anchor(2026, time.June), brt(2026, time.June, 19), billing.DefaultDueDay)
	if !sub.CurrentPeriodEnd.Equal(anchor(2026, time.July)) {
		t.Fatalf("the June invoice pays Jun 23 to Jul 23, got %s", sub.CurrentPeriodEnd)
	}
}

func TestALatePaymentRevivesFromThePaymentDay(t *testing.T) {
	sub := slots(1)
	sub.Status = workspace_plan.SubscriptionStatusExpired
	sub.RenewForInvoice(anchor(2026, time.June), brt(2026, time.July, 5), billing.DefaultDueDay)
	if sub.Status != workspace_plan.SubscriptionStatusActive || !sub.CurrentPeriodEnd.Equal(anchor(2026, time.July)) {
		t.Fatalf("a late payment revives until the next anchor, got %s %s", sub.Status, sub.CurrentPeriodEnd)
	}
}

func TestAPrepaidAddonLapsedByTheCutComesBackWithoutMoreTime(t *testing.T) {
	sub := slots(1)
	sub.CurrentPeriodEnd = anchor(2026, time.July)
	sub.Status = workspace_plan.SubscriptionStatusExpired
	if !sub.RenewForInvoice(anchor(2026, time.June), brt(2026, time.June, 26), billing.DefaultDueDay) {
		t.Fatal("the prepaid add-on must come back")
	}
	if sub.Status != workspace_plan.SubscriptionStatusActive || !sub.CurrentPeriodEnd.Equal(anchor(2026, time.July)) {
		t.Fatalf("it keeps the Jul 23 end it paid for, got %s %s", sub.Status, sub.CurrentPeriodEnd)
	}
}

func TestOnlyPeriodsEndingByTheAnchorAreDue(t *testing.T) {
	sub := slots(1)
	if !sub.DueBy(anchor(2026, time.June)) || sub.DueBy(anchor(2026, time.May)) {
		t.Fatal("a period ending Jun 23 is due on the Jun 23 invoice and not before")
	}
}

func TestANewSubscriptionStartsNowAndEndsAtTheQuote(t *testing.T) {
	now := brt(2026, time.June, 10)
	def := slotDefinition()
	def.ID = "d1"
	quote, err := QuotePurchase(def, workspace_plan.BillingCycleMonthly, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	sub := NewAddonSubscription("s1", "ws", def, 2, workspace_plan.BillingCycleMonthly, quote, now)
	if err := sub.Validate(); err != nil {
		t.Fatal(err)
	}
	if sub.Quantity != 2 || sub.UnitPriceMicros != 1_000_000 || !sub.CurrentPeriodEnd.Equal(anchor(2026, time.June)) {
		t.Fatalf("got %+v", sub)
	}
}
