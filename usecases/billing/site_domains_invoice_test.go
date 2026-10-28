package billing_usecase

import (
	"testing"
	"time"

	billing "vozko/domain/billing"

	"vozko/domain/invoice"
	"vozko/domain/workspace"
	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

func TestSiteDomainAddonIsChargedOnTheMonthlyInvoiceLikeEveryAddon(t *testing.T) {
	subs := &fakeSubs{subs: []*workspace_plan.WorkspaceSubscription{activeSub("ws-1", "plan-1")}}
	plans := &fakePlans{plans: map[string]*workspace_plan.PlanDefinition{"plan-1": {ID: "plan-1", Name: "Pro", BasePriceBRLCents: 50_000}}}
	domains := &workspace_addon.AddonSubscription{
		AddonKey:        "site_domain",
		EntitlementKind: workspace_addon.EntitlementSiteDomains,
		Status:          workspace_plan.SubscriptionStatusActive,
		Quantity:        2,
		UnitPriceMicros: 1_650_000,
	}
	addons := &fakeAddons{byWS: map[string][]*workspace_addon.AddonSubscription{"ws-1": {domains}}}
	wss := &fakeWorkspaces{byWS: map[string]*workspace.Workspace{"ws-1": owner("ws-1", "user-1")}}
	inv := &fakeCreateInvoice{}

	if n, err := emitFixture(subs, plans, addons, wss, inv).Execute(); err != nil || n != 1 {
		t.Fatalf("emit: n=%d err=%v", n, err)
	}
	got := inv.calls[0]
	if len(got.LineItems) != 2 {
		t.Fatalf("lines = %+v", got.LineItems)
	}
	line := got.LineItems[1]
	if line.Label != "site_domain" || line.Quantity != 2 || line.AmountBRL != 19.80 || line.Creditable || line.Kind != invoice.LineItemChannel {
		t.Fatalf("site domain line = %+v, want 2 x US$ 1,65 at 6.0 = R$ 19,80, not creditable", line)
	}
	if got.AmountBRL != 519.80 {
		t.Fatalf("total = %.2f, want plan 500 plus 19,80", got.AmountBRL)
	}
}

func slotLines(t *testing.T, domains *workspace_addon.AddonSubscription) []invoice.InvoiceLineItem {
	t.Helper()
	subs := &fakeSubs{subs: []*workspace_plan.WorkspaceSubscription{activeSub("ws-1", "plan-1")}}
	plans := &fakePlans{plans: map[string]*workspace_plan.PlanDefinition{"plan-1": {ID: "plan-1", Name: "Pro", BasePriceBRLCents: 50_000}}}
	addons := &fakeAddons{byWS: map[string][]*workspace_addon.AddonSubscription{"ws-1": {domains}}}
	wss := &fakeWorkspaces{byWS: map[string]*workspace.Workspace{"ws-1": owner("ws-1", "user-1")}}
	inv := &fakeCreateInvoice{}
	if _, err := emitFixture(subs, plans, addons, wss, inv).Execute(); err != nil {
		t.Fatalf("emit: %v", err)
	}
	return inv.calls[0].LineItems
}

func slotSubscription(quantity int) *workspace_addon.AddonSubscription {
	return &workspace_addon.AddonSubscription{
		AddonKey: "site_domain", EntitlementKind: workspace_addon.EntitlementSiteDomains, Status: workspace_plan.SubscriptionStatusActive,
		Quantity: quantity, UnitsPerQuantity: 1, UnitPriceMicros: 1_000_000,
	}
}

func march23() time.Time {
	return time.Date(2026, time.March, billing.DefaultDueDay, 0, 0, 0, 0, billing.LocationBRT())
}

func TestTheInvoiceBillsTheReducedSlotsOfTheCycleItCharges(t *testing.T) {
	reduced := slotSubscription(5)
	if err := reduced.ScheduleQuantity(2, march23(), 0); err != nil {
		t.Fatal(err)
	}
	lines := slotLines(t, reduced)
	if len(lines) != 2 || lines[1].Quantity != 2 || lines[1].AmountBRL != 12 {
		t.Fatalf("the Mar 23 cycle has 2 slots, lines = %+v", lines)
	}
	later := slotSubscription(5)
	if err := later.ScheduleQuantity(2, march23().AddDate(0, 1, 0), 0); err != nil {
		t.Fatal(err)
	}
	if lines := slotLines(t, later); lines[1].Quantity != 5 {
		t.Fatalf("a reduction for Apr 23 still bills 5 slots on Mar 23, lines = %+v", lines)
	}
}

func TestTheInvoiceDropsCancelledSlots(t *testing.T) {
	cancelled := slotSubscription(3)
	if err := cancelled.ScheduleCancellation(march23(), march23().AddDate(0, 0, -10), 0); err != nil {
		t.Fatal(err)
	}
	if lines := slotLines(t, cancelled); len(lines) != 1 || lines[0].Kind != invoice.LineItemPlan {
		t.Fatalf("a cancelled add-on is not renewed, lines = %+v", lines)
	}
}
