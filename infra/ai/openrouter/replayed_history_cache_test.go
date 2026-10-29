package openrouter

import (
	"encoding/json"
	"strings"
	"testing"

	"vozko/domain/ai"
)

func answerAfterReplay(model string) ai.GenerateInput {
	input := loopInput(model)
	replayed := []ai.Message{
		{Role: ai.RoleUser, Content: "PEDIDO DO USUÁRIO:\ncrie a peça"},
		{Role: ai.RoleAssistant, ToolCalls: []ai.ToolCall{{ID: "c0", Name: "studio_read"}}},
		{Role: ai.RoleTool, ToolCallID: "c0", Content: `{"clips":0}`},
		{Role: ai.RoleAssistant, Content: "Criei a peça."},
	}
	input.Messages = append(replayed, input.Messages...)
	input.ReplayedHistory = len(replayed)
	return input
}

func markedAt(t *testing.T, input ai.GenerateInput) []bool {
	t.Helper()
	req := mustService(t, Config{DefaultModel: input.Model}, nil).buildRequest(input)
	out := make([]bool, 0, len(req.Messages)-1)
	for _, message := range req.Messages[1:] {
		out = append(out, marked(message.Content))
	}
	body, _ := json.Marshal(req)
	if count := strings.Count(string(body), "cache_control"); count > 4 {
		t.Fatalf("Claude takes at most 4 breakpoints, got %d", count)
	}
	return out
}

func sameMarks(got, want []bool) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestClaudeGetsAFourthBreakpointWhereTheReplayedHistoryEnds(t *testing.T) {
	got := markedAt(t, answerAfterReplay("anthropic/claude-sonnet-5.5"))
	want := []bool{false, false, false, true, false, false, false, true, true, false}
	if !sameMarks(got, want) {
		t.Fatalf("marks = %v, want %v: the static system, the end of the replayed history and the rolling tail", got, want)
	}
}

func TestAReplayEndingInAToolCallMarksTheLastMessageWithText(t *testing.T) {
	input := answerAfterReplay("anthropic/claude-sonnet-5.5")
	input.ReplayedHistory = 2
	got := markedAt(t, input)
	want := []bool{true, false, false, false, false, false, false, true, true, false}
	if !sameMarks(got, want) {
		t.Fatalf("marks = %v, want %v", got, want)
	}
}

func TestTheFirstCallOfAnAnswerDoesNotMarkTheReplayTwice(t *testing.T) {
	input := answerAfterReplay("anthropic/claude-sonnet-5.5")
	input.Messages = append(input.Messages[:5:5], ai.Message{Role: ai.RoleUser, Content: "OBSERVAÇÃO DO SISTEMA"})
	got := markedAt(t, input)
	want := []bool{false, false, false, true, true, false}
	if !sameMarks(got, want) {
		t.Fatalf("marks = %v, want %v: the tail already closes the replayed history", got, want)
	}
}

func TestAMarkOutsideTheConversationIsIgnored(t *testing.T) {
	input := answerAfterReplay("anthropic/claude-sonnet-5.5")
	input.ReplayedHistory = len(input.Messages)
	got := markedAt(t, input)
	want := []bool{false, false, false, false, false, false, false, true, true, false}
	if !sameMarks(got, want) {
		t.Fatalf("marks = %v, want %v: the volatile tail is never cached", got, want)
	}
}

func TestProvidersThatCacheOnTheirOwnGetNoReplayMark(t *testing.T) {
	req := mustService(t, Config{DefaultModel: "openai/gpt-5"}, nil).buildRequest(answerAfterReplay("openai/gpt-5"))
	body, _ := json.Marshal(req)
	if strings.Contains(string(body), "cache_control") {
		t.Fatal("only Claude takes explicit breakpoints")
	}
}
