package workspace_plan_usecase

import (
	"errors"
	"testing"
	"time"

	"vozko/domain/shared/deps"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

type memoryDuplications struct {
	plans      *testPlanRepo
	records    map[string]workspace_plan.PlanDuplication
	creates    int
	failCreate error
	beforeTx   func()
}

func newMemoryDuplications(plans *testPlanRepo) *memoryDuplications {
	return &memoryDuplications{plans: plans, records: map[string]workspace_plan.PlanDuplication{}}
}

func duplicationKey(requestedBy, key string) string { return requestedBy + "|" + key }

func (m *memoryDuplications) PlanNames() ([]string, error) {
	names := make([]string, 0, len(m.plans.plans))
	for _, plan := range m.plans.plans {
		names = append(names, plan.Name)
	}
	return names, nil
}

func (m *memoryDuplications) FindDuplication(requestedBy, key string) (*workspace_plan.PlanDuplication, error) {
	record, ok := m.records[duplicationKey(requestedBy, key)]
	if !ok {
		return nil, workspace_plan.ErrPlanDuplicationNotFound
	}
	return &record, nil
}

func (m *memoryDuplications) CreateDuplicate(plan *workspace_plan.PlanDefinition, record workspace_plan.PlanDuplication, name workspace_plan.PlanNamer) error {
	m.creates++
	if m.beforeTx != nil {
		m.beforeTx()
	}
	if m.failCreate != nil {
		return m.failCreate
	}
	if _, exists := m.records[duplicationKey(record.RequestedBy, record.IdempotencyKey)]; exists {
		return workspace_plan.ErrPlanDuplicationExists
	}
	taken, _ := m.PlanNames()
	chosen, err := name(taken)
	if err != nil {
		return err
	}
	plan.Name = chosen
	stored := *plan
	stored.PricingItems = nil
	m.plans.plans[plan.ID] = &stored
	m.plans.pricingItems[plan.ID] = append([]workspace_plan.PlanPricingItem(nil), plan.PricingItems...)
	m.records[duplicationKey(record.RequestedBy, record.IdempotencyKey)] = record
	return nil
}

type duplicateRig struct {
	plans        *testPlanRepo
	duplications *memoryDuplications
	duplicate    workspace_plan.DuplicatePlanDefinitionUseCase
	preview      workspace_plan.PreviewPlanDuplicateUseCase
}

func newDuplicateRig(t *testing.T) *duplicateRig {
	t.Helper()
	plans := newTestPlanRepo()
	duplications := newMemoryDuplications(plans)
	duplicate, err := NewDuplicatePlanDefinitionUseCase(DuplicatePlanDeps{Plans: plans, Duplications: duplications})
	if err != nil {
		t.Fatalf("duplicate use case: %v", err)
	}
	preview, err := NewPreviewPlanDuplicateUseCase(DuplicatePlanDeps{Plans: plans, Duplications: duplications})
	if err != nil {
		t.Fatalf("preview use case: %v", err)
	}
	return &duplicateRig{plans: plans, duplications: duplications, duplicate: duplicate, preview: preview}
}

func (r *duplicateRig) seedSource() *workspace_plan.PlanDefinition {
	affiliate := "aff-1"
	stamp := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	source := &workspace_plan.PlanDefinition{
		ID: "source", Name: "Pro", Description: "Tudo incluso", BasePriceBRLCents: 49900, MaxCallChannels: 4,
		IncludedWhatsAppBusinessPhones: 2, MaxBranches: 3, IsGloballyVisible: true, ExclusiveAffiliateID: &affiliate,
		CreatedAt: stamp, UpdatedAt: stamp,
	}
	r.plans.plans[source.ID] = source
	r.plans.pricingItems[source.ID] = []workspace_plan.PlanPricingItem{
		{ID: "item-1", PlanDefinitionID: "source", Category: "whatsapp", Service: "marketing", Metric: "per_message", CostMicros: 11, PriceMicros: 22, MarkupPct: 1.5, Currency: "USD"},
		{ID: "item-2", PlanDefinitionID: "source", Category: "retired", Service: "legacy", Metric: "per_unit", CostMicros: 33, PriceMicros: 44, Currency: "USD"},
	}
	r.plans.visibility[source.ID] = []string{"ws-1"}
	return source
}

func duplicateInput(key string) workspace_plan.DuplicatePlanInput {
	return workspace_plan.DuplicatePlanInput{SourcePlanID: "source", RequestedBy: "admin-1", IdempotencyKey: key}
}

func TestDuplicatePlanCopiesEverySettingAndEveryStoredPricingItem(t *testing.T) {
	rig := newDuplicateRig(t)
	source := rig.seedSource()

	result, err := rig.duplicate.Execute(duplicateInput("k1"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	copied := rig.plans.plans[result.Plan.ID]
	if copied == nil || copied.ID == source.ID {
		t.Fatalf("no new plan was stored: %+v", result.Plan)
	}
	if copied.Description != source.Description || copied.BasePriceBRLCents != source.BasePriceBRLCents ||
		copied.MaxCallChannels != source.MaxCallChannels || copied.IncludedWhatsAppBusinessPhones != source.IncludedWhatsAppBusinessPhones ||
		copied.MaxBranches != source.MaxBranches {
		t.Fatalf("settings were not copied: %+v", copied)
	}
	items := rig.plans.pricingItems[copied.ID]
	want := rig.plans.pricingItems[source.ID]
	if len(items) != len(want) {
		t.Fatalf("copied %d pricing items, want %d", len(items), len(want))
	}
	for i := range items {
		if items[i].ID == "" || items[i].ID == want[i].ID || items[i].PlanDefinitionID != copied.ID {
			t.Fatalf("item %d keeps the source identity: %+v", i, items[i])
		}
		if items[i].CostMicros != want[i].CostMicros || items[i].PriceMicros != want[i].PriceMicros ||
			items[i].MarkupPct != want[i].MarkupPct || items[i].Currency != want[i].Currency || items[i].Service != want[i].Service {
			t.Fatalf("item %d = %+v, want the values of %+v", i, items[i], want[i])
		}
	}
	if result.Replayed {
		t.Fatal("a first duplication reports a replay")
	}
}

func TestDuplicatePlanIsPrivateAndReachesNoAudience(t *testing.T) {
	rig := newDuplicateRig(t)
	rig.seedSource()

	result, err := rig.duplicate.Execute(duplicateInput("k1"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	copied := rig.plans.plans[result.Plan.ID]
	if copied.IsGloballyVisible || copied.ExclusiveAffiliateID != nil || copied.ArchivedAt != nil {
		t.Fatalf("the copy is not private: %+v", copied)
	}
	if ids := rig.plans.visibility[copied.ID]; len(ids) != 0 {
		t.Fatalf("the copy is granted to %v", ids)
	}
	if result.Plan.IsGloballyVisible || len(result.Plan.AllowedWorkspaceIDs) != 0 {
		t.Fatalf("the answer shows a public copy: %+v", result.Plan)
	}
}

func TestDuplicatePlanNumbersTheCopyNameAroundTakenNames(t *testing.T) {
	rig := newDuplicateRig(t)
	rig.seedSource()
	rig.plans.plans["other"] = &workspace_plan.PlanDefinition{ID: "other", Name: "Pro (cópia)", MaxCallChannels: 1}

	preview, err := rig.preview.Execute("source")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	first, err := rig.duplicate.Execute(duplicateInput("k1"))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := rig.duplicate.Execute(duplicateInput("k2"))
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if preview.Name != "Pro (cópia 2)" || first.Plan.Name != "Pro (cópia 2)" || second.Plan.Name != "Pro (cópia 3)" {
		t.Fatalf("preview %q, first %q, second %q", preview.Name, first.Plan.Name, second.Plan.Name)
	}
}

func TestDuplicatePlanRepeatedWithTheSameKeyReturnsTheSamePlan(t *testing.T) {
	rig := newDuplicateRig(t)
	rig.seedSource()

	first, err := rig.duplicate.Execute(duplicateInput("k1"))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	again, err := rig.duplicate.Execute(duplicateInput("  k1  "))
	if err != nil {
		t.Fatalf("again: %v", err)
	}
	if again.Plan.ID != first.Plan.ID || !again.Replayed {
		t.Fatalf("repeat answered %+v, want a replay of %s", again, first.Plan.ID)
	}
	if rig.duplications.creates != 1 || len(rig.plans.plans) != 2 {
		t.Fatalf("a repeat wrote again: %d creates, %d plans", rig.duplications.creates, len(rig.plans.plans))
	}
}

func TestDuplicatePlanThatLosesARaceOnItsKeyReturnsTheWinner(t *testing.T) {
	rig := newDuplicateRig(t)
	rig.seedSource()
	rig.duplications.beforeTx = func() {
		rig.duplications.beforeTx = nil
		rig.plans.plans["winner"] = &workspace_plan.PlanDefinition{ID: "winner", Name: "Pro (cópia)", MaxCallChannels: 4}
		rig.duplications.records[duplicationKey("admin-1", "k1")] = workspace_plan.PlanDuplication{SourcePlanID: "source", PlanID: "winner", RequestedBy: "admin-1", IdempotencyKey: "k1"}
	}

	result, err := rig.duplicate.Execute(duplicateInput("k1"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Plan.ID != "winner" || !result.Replayed {
		t.Fatalf("got %+v, want the winner replayed", result)
	}
}

func TestDuplicatePlanRefusesAKeyThatDuplicatedAnotherPlan(t *testing.T) {
	rig := newDuplicateRig(t)
	rig.seedSource()
	rig.plans.plans["second"] = &workspace_plan.PlanDefinition{ID: "second", Name: "Basic", MaxCallChannels: 1}

	if _, err := rig.duplicate.Execute(duplicateInput("k1")); err != nil {
		t.Fatalf("first: %v", err)
	}
	other := duplicateInput("k1")
	other.SourcePlanID = "second"
	if _, err := rig.duplicate.Execute(other); !errors.Is(err, workspace_plan.ErrDuplicateKeyReused) {
		t.Fatalf("err = %v, want ErrDuplicateKeyReused", err)
	}
}

func TestDuplicatePlanRefusesAMissingSourcePlan(t *testing.T) {
	rig := newDuplicateRig(t)
	if _, err := rig.duplicate.Execute(duplicateInput("k1")); !errors.Is(err, workspace_plan.ErrPlanNotFound) {
		t.Fatalf("err = %v, want ErrPlanNotFound", err)
	}
	if _, err := rig.preview.Execute("source"); !errors.Is(err, workspace_plan.ErrPlanNotFound) {
		t.Fatalf("preview err = %v, want ErrPlanNotFound", err)
	}
	if rig.duplications.creates != 0 {
		t.Fatal("a missing source still wrote")
	}
}

func TestDuplicatePlanRunsTheCopyThroughThePlanValidation(t *testing.T) {
	rig := newDuplicateRig(t)
	rig.seedSource().MaxCallChannels = 0

	if _, err := rig.duplicate.Execute(duplicateInput("k1")); !errors.Is(err, workspace_plan.ErrInvalidMaxCallChannels) {
		t.Fatalf("err = %v, want ErrInvalidMaxCallChannels", err)
	}
	if rig.duplications.creates != 0 {
		t.Fatal("an invalid copy still wrote")
	}
}

func TestDuplicatePlanRefusesAnInputWithoutKeyOrRequester(t *testing.T) {
	rig := newDuplicateRig(t)
	rig.seedSource()
	for name, tc := range map[string]struct {
		input workspace_plan.DuplicatePlanInput
		want  error
	}{
		"no key":       {workspace_plan.DuplicatePlanInput{SourcePlanID: "source", RequestedBy: "admin-1"}, workspace_plan.ErrDuplicateKeyRequired},
		"no requester": {workspace_plan.DuplicatePlanInput{SourcePlanID: "source", IdempotencyKey: "k1"}, workspace_plan.ErrDuplicateRequesterRequired},
	} {
		if _, err := rig.duplicate.Execute(tc.input); !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	if rig.duplications.creates != 0 {
		t.Fatal("a refused input still wrote")
	}
}

func TestDuplicatePlanLeavesNothingBehindWhenTheWriteFails(t *testing.T) {
	rig := newDuplicateRig(t)
	rig.seedSource()
	rig.duplications.failCreate = errors.New("pricing item insert failed")

	if _, err := rig.duplicate.Execute(duplicateInput("k1")); err == nil {
		t.Fatal("a failed write reported success")
	}
	if len(rig.plans.plans) != 1 || len(rig.duplications.records) != 0 {
		t.Fatalf("a failed write left %d plans and %d records", len(rig.plans.plans), len(rig.duplications.records))
	}
}

func TestDuplicatePlanUseCasesNeedTheirRepositories(t *testing.T) {
	if _, err := NewDuplicatePlanDefinitionUseCase(DuplicatePlanDeps{}); !errors.Is(err, deps.ErrMissing) {
		t.Fatalf("duplicate err = %v, want deps.ErrMissing", err)
	}
	if _, err := NewPreviewPlanDuplicateUseCase(DuplicatePlanDeps{}); !errors.Is(err, deps.ErrMissing) {
		t.Fatalf("preview err = %v, want deps.ErrMissing", err)
	}
}
