package agentloop

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"vozko/domain/ai"
)

const tenThousandTokens = 40_000

func toolExchange(id, name string, chars int) []ai.Message {
	return []ai.Message{
		{Role: ai.RoleAssistant, Content: "vou ler " + id, ToolCalls: []ai.ToolCall{{ID: id, Name: name, Arguments: map[string]interface{}{"id": id}}}},
		{Role: ai.RoleTool, ToolCallID: id, Content: id + " " + strings.Repeat("a", chars-len(id)-1)},
	}
}

func historyWithOutputs(sizes ...int) []ai.Message {
	h := []ai.Message{{Role: ai.RoleUser, Content: UserRequest("monte o anúncio")}}
	for i, size := range sizes {
		h = append(h, toolExchange(fmt.Sprintf("c%d", i), "studio_read", size)...)
	}
	return append(h, ai.Message{Role: ai.RoleAssistant, Content: "pronto"})
}

func prunedCalls(h []ai.Message) []string {
	var ids []string
	for _, m := range h {
		if m.Role == ai.RoleTool && strings.HasPrefix(m.Content, "[resultado antigo") {
			ids = append(ids, m.ToolCallID)
		}
	}
	return ids
}

func prunedPrefixHolds(t *testing.T, before, after []ai.Message) {
	t.Helper()
	if len(after) < len(before) {
		t.Fatalf("the later history is shorter (%d) than the earlier one (%d)", len(after), len(before))
	}
	for i := range before {
		if !reflect.DeepEqual(before[i], after[i]) {
			t.Fatalf("message %d changed between answers, the cached prefix breaks:\nbefore %+v\nafter  %+v", i, before[i], after[i])
		}
	}
}

func TestToolOutputsInsideTheProtectedWindowAreKept(t *testing.T) {
	h := historyWithOutputs(tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens)
	if got := PruneToolOutputs(h); !reflect.DeepEqual(got, h) {
		t.Fatalf("the newest %d tokens of tool output stay whole, pruned %v", ai.PruneProtectTokens, prunedCalls(got))
	}
}

func TestOlderOutputsStayWhenPruningWouldFreeTooLittle(t *testing.T) {
	h := historyWithOutputs(tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens)
	if got := prunedCalls(PruneToolOutputs(h)); len(got) != 0 {
		t.Fatalf("pruning under %d tokens freed is not worth a cache rewrite, pruned %v", ai.PruneMinimumTokens, got)
	}
}

func TestOlderOutputsBecomeMarkersOldestFirst(t *testing.T) {
	h := historyWithOutputs(tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens)
	got := PruneToolOutputs(h)
	if pruned := prunedCalls(got); !reflect.DeepEqual(pruned, []string{"c0", "c1", "c2"}) {
		t.Fatalf("pruned %v, want the oldest outputs until at least %d tokens are freed", pruned, ai.PruneMinimumTokens)
	}
	if got[2].Content != "[resultado antigo de studio_read removido para economizar contexto; chame a ferramenta de novo se precisar dele]" {
		t.Fatalf("marker %q", got[2].Content)
	}
	if got[2].ToolCallID != "c0" || got[2].Role != ai.RoleTool {
		t.Fatalf("the marker answers the same call, got %+v", got[2])
	}
	for i, m := range got {
		if m.Role != ai.RoleTool && !reflect.DeepEqual(m, h[i]) {
			t.Fatalf("message %d is not a tool output and must stay as it was: %+v", i, m)
		}
	}
	if estimateRequest(&fakeDriver{}, got) >= estimateRequest(&fakeDriver{}, h)-ai.PruneMinimumTokens {
		t.Fatal("pruning must free at least the minimum")
	}
}

func TestPruningLeavesTheHistoryItWasGivenUntouched(t *testing.T) {
	h := historyWithOutputs(tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens)
	original := append([]ai.Message(nil), h...)
	first, second := PruneToolOutputs(h), PruneToolOutputs(h)
	if !reflect.DeepEqual(h, original) {
		t.Fatal("the given history was edited in place")
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("the same history must always prune the same way")
	}
	if !reflect.DeepEqual(PruneToolOutputs(first), first) {
		t.Fatal("a pruned history prunes to itself")
	}
}

func TestThePrunedPrefixHoldsAcrossAnswersUntilAnotherChunkIsFreed(t *testing.T) {
	sizes := []int{tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens}
	earlier := PruneToolOutputs(historyWithOutputs(sizes...))
	if len(prunedCalls(earlier)) == 0 {
		t.Fatal("the first answer must already prune")
	}
	grown := historyWithOutputs(append(sizes, 4_000, 4_000)...)
	later := PruneToolOutputs(grown)
	prunedPrefixHolds(t, earlier[:len(earlier)-1], later)
	moved := PruneToolOutputs(historyWithOutputs(append(sizes, tenThousandTokens, tenThousandTokens, tenThousandTokens)...))
	if len(prunedCalls(moved)) <= len(prunedCalls(later)) {
		t.Fatalf("another full chunk of old output must move the boundary, pruned %v then %v", prunedCalls(later), prunedCalls(moved))
	}
}

func TestShortOutputsAreNeverSwappedForALongerMarker(t *testing.T) {
	sizes := []int{40}
	for range 9 {
		sizes = append(sizes, tenThousandTokens)
	}
	got := PruneToolOutputs(historyWithOutputs(sizes...))
	if got[2].Content != firstOutput(sizes) {
		t.Fatalf("a short result costs less than its marker and stays, got %q", got[2].Content)
	}
	if len(prunedCalls(got)) == 0 {
		t.Fatal("the longer old outputs must still be pruned")
	}
}

func TestAnOutputWithoutItsCallGetsTheGenericMarker(t *testing.T) {
	h := historyWithOutputs(tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens, tenThousandTokens)
	h[1] = ai.Message{Role: ai.RoleAssistant, Content: "vou ler c0"}
	got := PruneToolOutputs(h)
	if got[2].Content != "[resultado antigo removido para economizar contexto]" {
		t.Fatalf("marker %q", got[2].Content)
	}
}

func firstOutput(sizes []int) string {
	return historyWithOutputs(sizes...)[2].Content
}
