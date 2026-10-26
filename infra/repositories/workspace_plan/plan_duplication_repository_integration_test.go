package workspace_plan_repository

import (
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspace_plan "vozko/domain/workspace/workspace_plan"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func planDuplicationDB(t *testing.T) *gorm.DB {
	t.Helper()
	return repotest.IsolatedDB(t, "plan_dup",
		&schema.WorkspacePlanDefinition{}, &schema.PlanPricingItem{}, &schema.PlanVisibilityEntry{},
		&schema.WorkspaceSubscription{}, &schema.WorkspacePlanDuplication{})
}

func seedSourcePlan(t *testing.T, db *gorm.DB) *workspace_plan.PlanDefinition {
	t.Helper()
	plans := NewPlanRepository(db)
	source := &workspace_plan.PlanDefinition{
		ID: uuid.NewString(), Name: "Pro", Description: "Tudo incluso", BasePriceBRLCents: 49900, MaxCallChannels: 4,
		IncludedWhatsAppBusinessPhones: 2, MaxBranches: 3, IsGloballyVisible: true,
	}
	if err := plans.Create(source); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	items := []workspace_plan.PlanPricingItem{
		{ID: uuid.NewString(), Category: "whatsapp", Service: "marketing", Metric: "per_message", CostMicros: 11, PriceMicros: 22, MarkupPct: 1.5, Currency: "USD"},
		{ID: uuid.NewString(), Category: "telephony", Service: "landline", Metric: "per_minute", CostMicros: 33, PriceMicros: 44, Currency: "USD"},
	}
	if err := plans.ReplacePricingItems(source.ID, items); err != nil {
		t.Fatalf("seed items: %v", err)
	}
	if err := plans.SetVisibility(source.ID, []string{uuid.NewString()}); err != nil {
		t.Fatalf("seed visibility: %v", err)
	}
	if err := NewSubscriptionRepository(db).Create(&workspace_plan.WorkspaceSubscription{
		ID: uuid.NewString(), WorkspaceID: uuid.NewString(), PlanDefinitionID: source.ID, PlanName: source.Name, MaxCallChannels: 4,
		BillingCycle: workspace_plan.BillingCycleMonthly, Status: workspace_plan.SubscriptionStatusActive,
		CurrentPeriodStart: time.Now().Add(-time.Hour), CurrentPeriodEnd: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	stored, err := plans.GetByID(source.ID)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	return stored
}

func copyOf(source *workspace_plan.PlanDefinition) *workspace_plan.PlanDefinition {
	copied := source.Duplicate(uuid.NewString())
	for i := range copied.PricingItems {
		copied.PricingItems[i].ID = uuid.NewString()
	}
	return copied
}

func recordFor(source, copied *workspace_plan.PlanDefinition, requestedBy, key string) workspace_plan.PlanDuplication {
	return workspace_plan.PlanDuplication{SourcePlanID: source.ID, PlanID: copied.ID, RequestedBy: requestedBy, IdempotencyKey: key}
}

func namedAfter(source *workspace_plan.PlanDefinition) workspace_plan.PlanNamer {
	return func(taken []string) (string, error) { return workspace_plan.DuplicateName(source.Name, taken) }
}

func countRows(t *testing.T, db *gorm.DB, model any, where string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Model(model).Where(where, args...).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestDuplicateWritesThePlanItsPricingItemsAndItsRecordAndNothingElse(t *testing.T) {
	db := planDuplicationDB(t)
	source := seedSourcePlan(t, db)
	repo := NewPlanDuplicationRepository(db)
	requester := uuid.NewString()

	copied := copyOf(source)
	if err := repo.CreateDuplicate(copied, recordFor(source, copied, requester, "k1"), namedAfter(source)); err != nil {
		t.Fatalf("CreateDuplicate: %v", err)
	}

	stored, err := NewPlanRepository(db).GetByID(copied.ID)
	if err != nil {
		t.Fatalf("read copy: %v", err)
	}
	if stored.Name != "Pro (cópia)" || stored.Description != source.Description || stored.BasePriceBRLCents != source.BasePriceBRLCents ||
		stored.MaxCallChannels != source.MaxCallChannels || stored.IncludedWhatsAppBusinessPhones != source.IncludedWhatsAppBusinessPhones ||
		stored.MaxBranches != source.MaxBranches {
		t.Fatalf("stored copy %+v does not match source %+v", stored, source)
	}
	if stored.IsGloballyVisible || stored.ExclusiveAffiliateID != nil || stored.ArchivedAt != nil {
		t.Fatalf("stored copy is not private: %+v", stored)
	}
	if stored.CreatedAt.IsZero() || stored.CreatedAt.Before(source.CreatedAt) {
		t.Fatalf("stored copy has no creation stamp of its own: %v", stored.CreatedAt)
	}
	if len(stored.PricingItems) != len(source.PricingItems) {
		t.Fatalf("copied %d items, want %d", len(stored.PricingItems), len(source.PricingItems))
	}
	byKey := map[string]workspace_plan.PlanPricingItem{}
	for _, item := range source.PricingItems {
		byKey[item.Category+"|"+item.Service+"|"+item.Metric] = item
	}
	for _, item := range stored.PricingItems {
		want := byKey[item.Category+"|"+item.Service+"|"+item.Metric]
		if item.CostMicros != want.CostMicros || item.PriceMicros != want.PriceMicros || item.MarkupPct != want.MarkupPct || item.Currency != want.Currency || item.ID == want.ID {
			t.Fatalf("item %+v, want the values of %+v under a new id", item, want)
		}
	}
	if n := countRows(t, db, &schema.PlanVisibilityEntry{}, "plan_definition_id = ?", copied.ID); n != 0 {
		t.Fatalf("copy was granted to %d workspaces", n)
	}
	if n := countRows(t, db, &schema.WorkspaceSubscription{}, "plan_definition_id = ?", copied.ID); n != 0 {
		t.Fatalf("copy carries %d subscriptions", n)
	}
	if n := countRows(t, db, &schema.WorkspaceSubscription{}, "plan_definition_id = ?", source.ID); n != 1 {
		t.Fatalf("source lost its subscription: %d", n)
	}

	record, err := repo.FindDuplication(requester, "k1")
	if err != nil {
		t.Fatalf("FindDuplication: %v", err)
	}
	if record.SourcePlanID != source.ID || record.PlanID != copied.ID || record.RequestedBy != requester || record.CreatedAt.IsZero() {
		t.Fatalf("record = %+v", record)
	}
	if _, err := repo.FindDuplication(uuid.NewString(), "k1"); !errors.Is(err, workspace_plan.ErrPlanDuplicationNotFound) {
		t.Fatalf("another admin's key err = %v, want ErrPlanDuplicationNotFound", err)
	}
}

func TestDuplicateRollsBackThePlanWhenAPricingItemCannotBeWritten(t *testing.T) {
	db := planDuplicationDB(t)
	source := seedSourcePlan(t, db)
	repo := NewPlanDuplicationRepository(db)
	requester := uuid.NewString()

	copied := copyOf(source)
	copied.PricingItems[1].Currency = "TOO_LONG_FOR_THE_COLUMN"
	if err := repo.CreateDuplicate(copied, recordFor(source, copied, requester, "k1"), namedAfter(source)); err == nil {
		t.Fatal("an unwritable pricing item reported success")
	}
	if _, err := NewPlanRepository(db).GetByID(copied.ID); !errors.Is(err, workspace_plan.ErrPlanNotFound) {
		t.Fatalf("the plan survived the rollback: %v", err)
	}
	if n := countRows(t, db, &schema.PlanPricingItem{}, "plan_definition_id = ?", copied.ID); n != 0 {
		t.Fatalf("%d pricing items survived the rollback", n)
	}
	if _, err := repo.FindDuplication(requester, "k1"); !errors.Is(err, workspace_plan.ErrPlanDuplicationNotFound) {
		t.Fatalf("the record survived the rollback: %v", err)
	}
}

func TestDuplicateRefusesAKeyThatIsAlreadyRecordedAndRollsBackTheSecondPlan(t *testing.T) {
	db := planDuplicationDB(t)
	source := seedSourcePlan(t, db)
	repo := NewPlanDuplicationRepository(db)
	requester := uuid.NewString()

	first := copyOf(source)
	if err := repo.CreateDuplicate(first, recordFor(source, first, requester, "k1"), namedAfter(source)); err != nil {
		t.Fatalf("first: %v", err)
	}
	second := copyOf(source)
	if err := repo.CreateDuplicate(second, recordFor(source, second, requester, "k1"), namedAfter(source)); !errors.Is(err, workspace_plan.ErrPlanDuplicationExists) {
		t.Fatalf("second err = %v, want ErrPlanDuplicationExists", err)
	}
	if _, err := NewPlanRepository(db).GetByID(second.ID); !errors.Is(err, workspace_plan.ErrPlanNotFound) {
		t.Fatalf("the second plan survived: %v", err)
	}
}

func TestConcurrentDuplicatesWithOneKeyCreateOnePlan(t *testing.T) {
	db := planDuplicationDB(t)
	source := seedSourcePlan(t, db)
	repo := NewPlanDuplicationRepository(db)
	requester := uuid.NewString()

	const clicks = 6
	errs := make([]error, clicks)
	var wg sync.WaitGroup
	for i := 0; i < clicks; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			copied := copyOf(source)
			errs[i] = repo.CreateDuplicate(copied, recordFor(source, copied, requester, "double-click"), namedAfter(source))
		}(i)
	}
	wg.Wait()

	created := 0
	for _, err := range errs {
		switch {
		case err == nil:
			created++
		case !errors.Is(err, workspace_plan.ErrPlanDuplicationExists):
			t.Fatalf("unexpected err: %v", err)
		}
	}
	if created != 1 {
		t.Fatalf("%d copies were created, want 1", created)
	}
	if n := countRows(t, db, &schema.WorkspacePlanDefinition{}, "id <> ?", source.ID); n != 1 {
		t.Fatalf("%d plans exist besides the source, want 1", n)
	}
}

func TestConcurrentDuplicatesWithDifferentKeysTakeDifferentNames(t *testing.T) {
	db := planDuplicationDB(t)
	source := seedSourcePlan(t, db)
	repo := NewPlanDuplicationRepository(db)

	const copies = 4
	errs := make([]error, copies)
	var wg sync.WaitGroup
	for i := 0; i < copies; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			copied := copyOf(source)
			errs[i] = repo.CreateDuplicate(copied, recordFor(source, copied, uuid.NewString(), "k"), namedAfter(source))
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("CreateDuplicate: %v", err)
		}
	}

	var names []string
	if err := db.Model(&schema.WorkspacePlanDefinition{}).Where("id <> ?", source.ID).Pluck("name", &names).Error; err != nil {
		t.Fatalf("names: %v", err)
	}
	sort.Strings(names)
	want := []string{"Pro (cópia 2)", "Pro (cópia 3)", "Pro (cópia 4)", "Pro (cópia)"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}
}

func TestPlanNamesIncludeArchivedPlans(t *testing.T) {
	db := planDuplicationDB(t)
	source := seedSourcePlan(t, db)
	plans := NewPlanRepository(db)
	archived := &workspace_plan.PlanDefinition{ID: uuid.NewString(), Name: "Pro (cópia)", MaxCallChannels: 1}
	if err := plans.Create(archived); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := plans.Archive(archived.ID, time.Now()); err != nil {
		t.Fatalf("archive: %v", err)
	}

	names, err := NewPlanDuplicationRepository(db).PlanNames()
	if err != nil {
		t.Fatalf("PlanNames: %v", err)
	}
	sort.Strings(names)
	if len(names) != 2 || names[0] != source.Name || names[1] != archived.Name {
		t.Fatalf("names = %v", names)
	}
}

func TestCreateStoresAPrivatePlanAsPrivate(t *testing.T) {
	db := planDuplicationDB(t)
	plans := NewPlanRepository(db)
	private := &workspace_plan.PlanDefinition{ID: uuid.NewString(), Name: "Interno", MaxCallChannels: 1, MaxBranches: 1}
	if err := plans.Create(private); err != nil {
		t.Fatalf("Create: %v", err)
	}
	stored, err := plans.GetByID(private.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.IsGloballyVisible {
		t.Fatal("the column default made a private plan public")
	}
}
