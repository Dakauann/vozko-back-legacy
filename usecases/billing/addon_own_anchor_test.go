package billing_usecase

import (
	"errors"
	"testing"
	"time"

	billing "vozko/domain/billing"
	"vozko/domain/invoice"
	"vozko/domain/workspace"
	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

func brtDay(month time.Month, day, hour int) time.Time {
	return time.Date(2026, month, day, hour, 0, 0, 0, billing.LocationBRT())
}

func anchorOf(month time.Month) time.Time {
	return time.Date(2026, month, billing.DefaultDueDay, 0, 0, 0, 0, billing.LocationBRT())
}

func annualPlan(ws string) *workspace_plan.WorkspaceSubscription {
	return &workspace_plan.WorkspaceSubscription{
		ID: "sub-" + ws, WorkspaceID: ws, PlanDefinitionID: "plan-1",
		Status: workspace_plan.SubscriptionStatusActive, BillingCycle: workspace_plan.BillingCycleAnnual,
		CurrentPeriodStart: anchorOf(time.January), CurrentPeriodEnd: anchorOf(time.January).AddDate(1, 0, 0),
	}
}

func monthlySlots(ws string, quantity int, end time.Time) *workspace_addon.AddonSubscription {
	return &workspace_addon.AddonSubscription{
		ID: "slots-" + ws, WorkspaceID: ws, AddonDefinitionID: "d1", AddonKey: "site_domain",
		EntitlementKind: workspace_addon.EntitlementSiteDomains, Status: workspace_plan.SubscriptionStatusActive,
		BillingCycle: workspace_plan.BillingCycleMonthly, Quantity: quantity, UnitsPerQuantity: 1, UnitPriceMicros: 1_000_000,
		CurrentPeriodStart: end.AddDate(0, -1, 0), CurrentPeriodEnd: end,
	}
}

func slotDefinition() *workspace_addon.AddonDefinition {
	return &workspace_addon.AddonDefinition{
		ID: "d1", Key: "site_domain", Name: "Domínio", EntitlementKind: workspace_addon.EntitlementSiteDomains,
		UnitsPerQuantity: 1, MonthlyPriceMicros: 1_000_000, MaxUnits: 10, IsActive: true,
	}
}

func emitOn(subs *fakeSubs, addons *fakeAddons, inv invoiceCreator, at time.Time) *emitMonthlyInvoicesUseCase {
	plans := &fakePlans{plans: map[string]*workspace_plan.PlanDefinition{"plan-1": {ID: "plan-1", Name: "Pro", BasePriceBRLCents: 50_000}}}
	wss := &fakeWorkspaces{byWS: map[string]*workspace.Workspace{"ws-1": owner("ws-1", "user-1")}}
	uc := NewEmitMonthlyInvoicesUseCase(subs, plans, addons, wss, &fakePricing{rate: 6.0}, inv, &recordingNotices{})
	uc.now = func() time.Time { return at }
	return uc
}

type invoiceCreator interface {
	Execute(in invoice.CreateInvoiceInput) (*invoice.CreateInvoiceOutput, error)
}

func TestAMonthlyAddonOnAnAnnualPlanIsInvoicedOnItsOwnAnchor(t *testing.T) {
	subs := &fakeSubs{latest: annualPlan("ws-1")}
	addons := &fakeAddons{byWS: map[string][]*workspace_addon.AddonSubscription{"ws-1": {monthlySlots("ws-1", 2, anchorOf(time.March))}}}
	inv := &fakeCreateInvoice{}

	if n, err := emitOn(subs, addons, inv, brtDay(time.March, 18, 9)).Execute(); err != nil || n != 1 {
		t.Fatalf("emit: n=%d err=%v", n, err)
	}
	got := inv.calls[0]
	if got.AmountBRL != 12 || got.CreditableBRL != 0 || got.IdempotencyKey != "monthly:ws-1:2026-03-23" {
		t.Fatalf("the slots alone are billed on Mar 23, got %+v", got)
	}
	if len(got.LineItems) != 1 || got.LineItems[0].Kind != invoice.LineItemChannel || got.LineItems[0].Quantity != 2 {
		t.Fatalf("the annual plan is not billed again, lines = %+v", got.LineItems)
	}
	if got.BillingCycle != string(workspace_plan.BillingCycleMonthly) {
		t.Fatalf("the add-on invoice is monthly, got %q", got.BillingCycle)
	}
}

func TestAWorkspaceWithAPlanDueGetsOneInvoice(t *testing.T) {
	plan := activeSub("ws-1", "plan-1")
	subs := &fakeSubs{subs: []*workspace_plan.WorkspaceSubscription{plan}, latest: plan}
	addons := &fakeAddons{byWS: map[string][]*workspace_addon.AddonSubscription{"ws-1": {monthlySlots("ws-1", 1, anchorOf(time.March))}}}
	inv := &fakeCreateInvoice{}

	if n, err := emitOn(subs, addons, inv, brtDay(time.March, 18, 9)).Execute(); err != nil || n != 1 || len(inv.calls) != 1 {
		t.Fatalf("one invoice carries the plan and the slots, n=%d calls=%d err=%v", n, len(inv.calls), err)
	}
	if len(inv.calls[0].LineItems) != 2 {
		t.Fatalf("plan and slots on one invoice, got %+v", inv.calls[0].LineItems)
	}
}

func TestOnlyAddonsDueByTheAnchorAreInvoiced(t *testing.T) {
	plan := activeSub("ws-1", "plan-1")
	subs := &fakeSubs{subs: []*workspace_plan.WorkspaceSubscription{plan}, latest: plan}
	paidAhead := monthlySlots("ws-1", 1, anchorOf(time.April))
	yearly := channelAddon(25_000_000, 1)
	yearly.BillingCycle = workspace_plan.BillingCycleAnnual
	yearly.CurrentPeriodEnd = anchorOf(time.March).AddDate(1, 0, 0)
	addons := &fakeAddons{byWS: map[string][]*workspace_addon.AddonSubscription{"ws-1": {paidAhead, yearly}}}
	inv := &fakeCreateInvoice{}

	if _, err := emitOn(subs, addons, inv, brtDay(time.March, 18, 9)).Execute(); err != nil {
		t.Fatal(err)
	}
	if lines := inv.calls[0].LineItems; len(lines) != 1 || lines[0].Kind != invoice.LineItemPlan {
		t.Fatalf("a slot paid to Apr 23 and an annual add-on are not billed on Mar 23, lines = %+v", lines)
	}
}

func TestAddonsOfAWorkspaceWithoutAnActivePlanAreNotInvoiced(t *testing.T) {
	expired := annualPlan("ws-1")
	expired.Status = workspace_plan.SubscriptionStatusExpired
	addons := &fakeAddons{byWS: map[string][]*workspace_addon.AddonSubscription{"ws-1": {monthlySlots("ws-1", 2, anchorOf(time.March))}}}
	for name, subs := range map[string]*fakeSubs{
		"expired plan":    {latest: expired},
		"no plan":         {},
		"unreadable plan": {latestErr: errors.New("db blip")},
	} {
		inv := &fakeCreateInvoice{}
		if n, err := emitOn(subs, addons, inv, brtDay(time.March, 18, 9)).Execute(); err != nil || n != 0 || len(inv.calls) != 0 {
			t.Fatalf("%s: units that grant nothing are not billed, n=%d calls=%d err=%v", name, n, len(inv.calls), err)
		}
	}
}

func TestTheAddonDueListErrorPropagates(t *testing.T) {
	addons := &fakeAddons{dueErr: errors.New("db down")}
	if _, err := emitOn(&fakeSubs{}, addons, &fakeCreateInvoice{}, brtDay(time.March, 18, 9)).Execute(); err == nil {
		t.Fatal("the add-on due list error must propagate")
	}
}

func TestPayingAnAddonOnlyInvoiceNeverExtendsTheAnnualPlan(t *testing.T) {
	plan := annualPlan("ws-1")
	subs := &fakeSubs{latest: plan}
	addons := &fakeAddons{byWS: map[string][]*workspace_addon.AddonSubscription{"ws-1": {monthlySlots("ws-1", 2, anchorOf(time.March))}}}
	uc := NewConfirmMonthlyBillingUseCase(subs, addons)
	uc.now = func() time.Time { return brtDay(time.March, 20, 10) }

	if err := uc.ExecuteForCycle("ws-1", anchorOf(time.March)); err != nil {
		t.Fatal(err)
	}
	if len(subs.updated) != 0 {
		t.Fatalf("the annual plan was not billed, so it must not move, got %+v", subs.updated)
	}
	if len(addons.updated) != 1 || !addons.updated[0].CurrentPeriodEnd.Equal(anchorOf(time.April)) {
		t.Fatalf("the slots renew to Apr 23, got %+v", addons.updated)
	}
}

func TestASlotBoughtBetweenIssueAndDueIsNotPushedAheadByThePayment(t *testing.T) {
	slots := monthlySlots("ws-1", 1, anchorOf(time.March))
	addons := &fakeAddons{byWS: map[string][]*workspace_addon.AddonSubscription{"ws-1": {slots}}}
	plan := planSub("ws-1", anchorOf(time.March))
	subs := &fakeSubs{subs: []*workspace_plan.WorkspaceSubscription{plan}, latest: plan}
	store := newMemInvoiceStore()

	if _, err := emitOn(subs, addons, &storeBackedCreateInvoice{store: store}, brtDay(time.March, 18, 9)).Execute(); err != nil {
		t.Fatal(err)
	}
	buySlot(t, slots, brtDay(time.March, 20, 10))
	confirm := NewConfirmMonthlyBillingUseCase(subs, addons)
	confirm.now = func() time.Time { return brtDay(time.March, 21, 10) }
	for i := 0; i < 2; i++ {
		if err := confirm.ExecuteForCycle("ws-1", anchorOf(time.March)); err != nil {
			t.Fatal(err)
		}
	}
	if !slots.CurrentPeriodEnd.Equal(anchorOf(time.April)) || slots.Quantity != 2 {
		t.Fatalf("paying the Mar 23 invoice after buying keeps Apr 23, got %s qty %d", slots.CurrentPeriodEnd, slots.Quantity)
	}

	if err := slots.ScheduleCancellation(slots.ChangeDate(brtDay(time.March, 25, 10)), brtDay(time.March, 25, 10), 0); err != nil {
		t.Fatal(err)
	}
	plan.CurrentPeriodEnd = anchorOf(time.April)
	april := &fakeCreateInvoice{}
	if _, err := emitOn(subs, addons, april, brtDay(time.April, 18, 9)).Execute(); err != nil {
		t.Fatal(err)
	}
	if lines := april.calls[0].LineItems; len(lines) != 1 {
		t.Fatalf("a slot cancelled in March is not billed in April, lines = %+v", lines)
	}
}

func TestPayingBeforeBuyingEndsOnTheSameAnchor(t *testing.T) {
	slots := monthlySlots("ws-1", 1, anchorOf(time.March))
	addons := &fakeAddons{byWS: map[string][]*workspace_addon.AddonSubscription{"ws-1": {slots}}}
	confirm := NewConfirmMonthlyBillingUseCase(&fakeSubs{}, addons)
	confirm.now = func() time.Time { return brtDay(time.March, 19, 10) }
	if err := confirm.ExecuteForCycle("ws-1", anchorOf(time.March)); err != nil {
		t.Fatal(err)
	}
	buySlot(t, slots, brtDay(time.March, 20, 10))
	if !slots.CurrentPeriodEnd.Equal(anchorOf(time.April)) || slots.Quantity != 2 {
		t.Fatalf("both orders end on Apr 23 with 2 slots, got %s qty %d", slots.CurrentPeriodEnd, slots.Quantity)
	}
}

func TestConfirmWithoutTheDueDatePaysTheLatestIssuedCycle(t *testing.T) {
	slots := monthlySlots("ws-1", 1, anchorOf(time.March))
	slots.CurrentPeriodEnd = anchorOf(time.April)
	addons := &fakeAddons{byWS: map[string][]*workspace_addon.AddonSubscription{"ws-1": {slots}}}
	confirm := NewConfirmMonthlyBillingUseCase(&fakeSubs{}, addons)
	confirm.now = func() time.Time { return brtDay(time.March, 21, 10) }
	if err := confirm.Execute("ws-1"); err != nil {
		t.Fatal(err)
	}
	if len(addons.updated) != 0 || !slots.CurrentPeriodEnd.Equal(anchorOf(time.April)) {
		t.Fatalf("a payment on Mar 21 settles the Mar 23 cycle only, got %s", slots.CurrentPeriodEnd)
	}
}

func TestAnUnpaidAddonOnlyInvoiceLapsesOnlyTheAddonsItBilled(t *testing.T) {
	due := anchorOf(time.March)
	unpaid := dueOn("inv-1", "ws-1", due)
	unpaid.LineItems = []invoice.InvoiceLineItem{{Kind: invoice.LineItemChannel, Label: "site_domain", AmountBRL: 12, Quantity: 2}}
	inv := &fakeInvoiceRepo{unpaid: []invoice.Invoice{unpaid}}
	billed := monthlySlots("ws-1", 2, due)
	prepaid := channelAddon(25_000_000, 1)
	prepaid.CurrentPeriodEnd = anchorOf(time.April)
	addons := &fakeAddons{byWS: map[string][]*workspace_addon.AddonSubscription{"ws-1": {billed, prepaid}}}
	subs := &fakeSubs{latest: annualPlan("ws-1")}
	onReduced := &fakeEntitlementHandler{}
	hooks := &recordingAccessHooks{}
	notices := &recordingNotices{}
	uc := NewCancelSweepUseCase(inv, subs, addons, onReduced, &fakeAlerter{}, notices).WithAccessHooks(hooks)
	uc.now = func() time.Time { return brtDay(time.March, 28, 1) }

	if n, err := uc.Execute(); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(addons.updated) != 1 || addons.updated[0] != billed || billed.Status != workspace_plan.SubscriptionStatusExpired {
		t.Fatalf("only the unpaid slots lapse, got %+v", addons.updated)
	}
	if len(subs.updated) != 0 || len(hooks.cut) != 0 || len(notices.cut) != 0 {
		t.Fatalf("the paid annual plan keeps running, plan updates %d access cuts %v notices %v", len(subs.updated), hooks.cut, notices.cut)
	}
	if len(onReduced.reduced) != 1 || inv.statuses["inv-1"] != invoice.StatusExpired {
		t.Fatalf("entitlements are reconciled and the invoice closed, reduced %v status %q", onReduced.reduced, inv.statuses["inv-1"])
	}
}

func buySlot(t *testing.T, slots *workspace_addon.AddonSubscription, at time.Time) {
	t.Helper()
	quote, err := workspace_addon.QuotePurchase(slotDefinition(), workspace_plan.BillingCycleMonthly, 1, at)
	if err != nil {
		t.Fatal(err)
	}
	slots.TopUp(slotDefinition(), 1, workspace_plan.BillingCycleMonthly, quote, at)
}
