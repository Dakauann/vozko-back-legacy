package ai

import "testing"

func TestTheContextCeilingLeavesTheLargerOfTenPercentOrTheMinimumReserve(t *testing.T) {
	cases := map[int64]int{20_000: 18_000, 40_000: 24_000, 128_000: 112_000, 200_000: 180_000, 1_000_000: 900_000}
	for window, want := range cases {
		got, ok := ModelInfo{ContextLength: window}.ContextCeiling()
		if !ok || got != want {
			t.Errorf("window %d: ceiling %d, want %d", window, got, want)
		}
	}
}

func TestTheWorkingBudgetNeverPassesTheContextCap(t *testing.T) {
	cases := map[int64]int{20_000: 18_000, 40_000: 24_000, 128_000: 112_000, 200_000: 180_000, 240_000: 200_000, 1_000_000: 200_000, 2_000_000: 200_000}
	for window, want := range cases {
		got, ok := ModelInfo{ContextLength: window}.ContextBudget()
		if !ok || got != want {
			t.Errorf("window %d: budget %d, want %d", window, got, want)
		}
	}
}

func TestAnUnknownWindowHasNoBudget(t *testing.T) {
	if _, ok := (ModelInfo{}).ContextBudget(); ok {
		t.Error("an unknown window has no working budget")
	}
	if _, ok := (ModelInfo{}).ContextCeiling(); ok {
		t.Error("an unknown window has no ceiling")
	}
}

func TestPruningProtectsMoreThanItMustFree(t *testing.T) {
	if PruneProtectTokens != 40_000 || PruneMinimumTokens != 20_000 || PruneMinimumTokens >= PruneProtectTokens {
		t.Fatalf("protect %d, minimum %d", PruneProtectTokens, PruneMinimumTokens)
	}
	if ContextBudgetCapTokens <= PruneProtectTokens {
		t.Fatalf("the cap %d must leave room above the protected tool output", ContextBudgetCapTokens)
	}
}
