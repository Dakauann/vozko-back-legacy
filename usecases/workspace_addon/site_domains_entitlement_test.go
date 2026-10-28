package workspace_addon_usecase

import (
	"errors"
	"slices"
	"testing"
	"time"

	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

func TestSiteDomainsIsASellableKind(t *testing.T) {
	kind := workspace_addon.EntitlementSiteDomains
	if kind != "site_domains" || !kind.IsValid() || !slices.Contains(workspace_addon.AllEntitlementKinds(), kind) {
		t.Fatalf("site_domains must be a valid, listed kind, got %q", kind)
	}
}

func domainSlots(quantity int, end time.Time) *workspace_addon.AddonSubscription {
	return &workspace_addon.AddonSubscription{
		ID: "a1", WorkspaceID: "ws-1", EntitlementKind: workspace_addon.EntitlementSiteDomains,
		Quantity: quantity, UnitsPerQuantity: 1, Status: workspace_plan.SubscriptionStatusActive,
		CurrentPeriodStart: end.AddDate(0, -1, 0), CurrentPeriodEnd: end,
	}
}

func TestSiteDomainsAreOnlyThePurchasedSlots(t *testing.T) {
	plans := &fakePlanReader{plan: &workspace_plan.PlanDefinition{ID: "plan-1", MaxCallChannels: 9}}
	resolver := NewEntitlementResolver(activeSubscription(), plans, newFakeSubRepo(domainSlots(2, time.Now().Add(time.Hour))), &fakeConfigReader{})
	total, err := resolver.Resolve("ws-1", workspace_addon.EntitlementSiteDomains)
	if err != nil || total != 2 {
		t.Fatalf("total = %d, err = %v; want only the 2 bought slots", total, err)
	}
	empty := NewEntitlementResolver(activeSubscription(), plans, newFakeSubRepo(), &fakeConfigReader{})
	if total, err := empty.Resolve("ws-1", workspace_addon.EntitlementSiteDomains); err != nil || total != 0 {
		t.Fatalf("no plan includes a domain, got %d %v", total, err)
	}
}

func TestSiteDomainsNeedAnActivePlan(t *testing.T) {
	resolver := NewEntitlementResolver(&fakeCurrentSub{}, &fakePlanReader{}, newFakeSubRepo(), &fakeConfigReader{})
	if _, err := resolver.Resolve("ws-1", workspace_addon.EntitlementSiteDomains); !errors.Is(err, workspace_plan.ErrSubscriptionNotCurrent) {
		t.Fatalf("got %v", err)
	}
}

func TestCancelledSlotsStopCountingWhenThePaidPeriodEnds(t *testing.T) {
	end := time.Now().Add(-time.Minute)
	sub := domainSlots(3, end)
	if err := sub.ScheduleCancellation(end, end.AddDate(0, 0, -5), 0); err != nil {
		t.Fatal(err)
	}
	resolver := NewEntitlementResolver(activeSubscription(), &fakePlanReader{plan: &workspace_plan.PlanDefinition{ID: "plan-1"}}, newFakeSubRepo(sub), &fakeConfigReader{})
	if total, err := resolver.Resolve("ws-1", workspace_addon.EntitlementSiteDomains); err != nil || total != 0 {
		t.Fatalf("a cancelled add-on past its paid period grants nothing, got %d %v", total, err)
	}
}
