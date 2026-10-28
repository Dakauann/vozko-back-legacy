package workspace_plan_usecase

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	workspace_plan "vozko/domain/workspace/workspace_plan"
)

type recordingAccess struct {
	cut      []string
	restored []string
	err      error
}

func (r *recordingAccess) OnAccessCut(_ context.Context, workspaceID string) error {
	r.cut = append(r.cut, workspaceID)
	return r.err
}

func (r *recordingAccess) OnAccessRestored(_ context.Context, workspaceID string) error {
	r.restored = append(r.restored, workspaceID)
	return r.err
}

func TestSubscribingRestoresTheWorkspaceAccess(t *testing.T) {
	plans := newTestPlanRepo()
	plans.Create(&workspace_plan.PlanDefinition{ID: "plan-1", Name: "Starter", BasePriceBRLCents: 19900})
	access := &recordingAccess{err: errors.New("kv down")}
	uc := NewSubscribeWorkspaceUseCase(plans, newTestSubscriptionRepo(), access)
	if _, err := uc.Execute("ws-1", "plan-1", workspace_plan.BillingCycleMonthly); err != nil {
		t.Fatalf("a failed hook never fails the payment: %v", err)
	}
	if !slices.Equal(access.restored, []string{"ws-1"}) {
		t.Fatalf("got %v", access.restored)
	}
}

func TestRenewingRestoresTheWorkspaceAccess(t *testing.T) {
	plans := newTestPlanRepo()
	plans.Create(&workspace_plan.PlanDefinition{ID: "plan-1", Name: "Starter", BasePriceBRLCents: 19900})
	subscriptions := newTestSubscriptionRepo()
	now := time.Now().UTC()
	subscriptions.subscriptions["sub-1"] = &workspace_plan.WorkspaceSubscription{
		ID: "sub-1", WorkspaceID: "ws-1", PlanDefinitionID: "plan-1", PlanName: "Starter", Status: workspace_plan.SubscriptionStatusExpired,
		CurrentPeriodStart: now.AddDate(0, -2, 0), CurrentPeriodEnd: now.AddDate(0, -1, 0),
	}
	access := &recordingAccess{}
	uc := NewRenewWorkspaceSubscriptionUseCase(plans, subscriptions, access)
	if _, err := uc.Execute("ws-1"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(access.restored, []string{"ws-1"}) {
		t.Fatalf("got %v", access.restored)
	}
}

func TestExpiringCutsTheAccessOfEveryExpiredWorkspace(t *testing.T) {
	repo := &countingExpireRepo{returns: []expireReturn{{wsIDs: []string{"ws-a", "ws-b"}}}}
	access := &recordingAccess{err: errors.New("kv down")}
	uc := NewExpireSubscriptionsUseCase(repo, nil, access)
	if _, err := uc.Execute(); err != nil {
		t.Fatalf("a failed hook is repaired by the sites sweep: %v", err)
	}
	slices.Sort(access.cut)
	if !slices.Equal(access.cut, []string{"ws-a", "ws-b"}) {
		t.Fatalf("got %v", access.cut)
	}
}
