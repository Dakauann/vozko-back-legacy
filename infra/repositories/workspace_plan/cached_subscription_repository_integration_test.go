package workspace_plan_repository

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	workspace_plan "vozko/domain/workspace/workspace_plan"
	infracache "vozko/infra/cache"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func TestTheCachedSubscriptionMatchesPostgresAtEveryMoment(t *testing.T) {
	end := time.Date(2026, 11, 23, 23, 59, 59, 123456000, time.UTC)
	moments := map[string]time.Time{
		"an hour before":           end.Add(-time.Hour),
		"exactly at the end":       end,
		"400ns after the end":      end.Add(400 * time.Nanosecond),
		"999ns after the end":      end.Add(999 * time.Nanosecond),
		"one microsecond after":    end.Add(time.Microsecond),
		"an hour after":            end.Add(time.Hour),
		"a day before, other zone": end.Add(-24 * time.Hour).In(time.FixedZone("BRT", -3*3600)),
	}
	statuses := []workspace_plan.SubscriptionStatus{
		workspace_plan.SubscriptionStatusActive,
		workspace_plan.SubscriptionStatusCancelled,
		workspace_plan.SubscriptionStatusExpired,
	}
	for _, prepared := range []bool{false, true} {
		t.Run(fmt.Sprintf("prepared statements %v", prepared), func(t *testing.T) {
			db := repotest.IsolatedDBWith(t, "ws_sub_cache", repotest.Options{PrepareStmt: prepared}, &schema.WorkspaceSubscription{})
			table := NewSubscriptionRepository(db)
			for _, status := range statuses {
				for name, at := range moments {
					for _, primedAt := range []time.Time{end.Add(-2 * time.Hour), end.Add(2 * time.Hour)} {
						t.Run(fmt.Sprintf("%s/%s/primed %s", status, name, primedAt.Format(time.Kitchen)), func(t *testing.T) {
							workspaceID := uuid.NewString()
							cancelled := end.Add(-72 * time.Hour)
							if err := table.Create(&workspace_plan.WorkspaceSubscription{
								ID: uuid.NewString(), WorkspaceID: workspaceID, PlanDefinitionID: uuid.NewString(), PlanName: "Pro",
								MaxCallChannels: 2, BillingCycle: workspace_plan.BillingCycle("monthly"), Status: status,
								CurrentPeriodStart: end.AddDate(0, -1, 0), CurrentPeriodEnd: end, CancelledAt: &cancelled,
							}); err != nil {
								t.Fatalf("seed: %v", err)
							}
							state := newMemoryState()
							cached := NewCachedSubscriptionRepository(table, state, infracache.NewSharedStateVersions(state, SubscriptionCachePrefix))
							if _, err := cached.GetCurrentByWorkspaceID(workspaceID, primedAt); err != nil && !errors.Is(err, workspace_plan.ErrSubscriptionNotCurrent) {
								t.Fatalf("prime: %v", err)
							}
							got, gotErr := cached.GetCurrentByWorkspaceID(workspaceID, at)
							want, wantErr := table.GetCurrentByWorkspaceID(workspaceID, at)
							if !errors.Is(gotErr, wantErr) || (wantErr == nil && gotErr != nil) {
								t.Fatalf("cache says %v, Postgres says %v", gotErr, wantErr)
							}
							if !reflect.DeepEqual(got, want) {
								t.Fatalf("cache row %+v\nPostgres row %+v", got, want)
							}
						})
					}
				}
			}
		})
	}
}
