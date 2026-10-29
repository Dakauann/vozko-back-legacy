package looptest

import (
	"encoding/json"
	"fmt"
	"testing"

	"vozko/domain/ai"
)

func Wire(v interface{}) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("unencodable: %v", err)
	}
	return string(raw)
}

func Clip(text string) string {
	runes := []rune(text)
	if len(runes) <= 160 {
		return text
	}
	return string(runes[:160]) + "..."
}

func Stable(in ai.GenerateInput) []ai.Message {
	return in.Messages[:len(in.Messages)-min(max(in.VolatileTail, 0), len(in.Messages))]
}

func SameHead(prev, next ai.GenerateInput) bool {
	return prev.Model == next.Model && prev.SystemPrompt == next.SystemPrompt && Wire(prev.Tools) == Wire(next.Tools) && prev.AsOf.Equal(next.AsOf)
}

func SharedPrefix(prev, next ai.GenerateInput) int {
	return SharedMessages(Stable(prev), next.Messages)
}

func SharedMessages(want, got []ai.Message) int {
	n := 0
	for n < len(want) && n < len(got) && Wire(want[n]) == Wire(got[n]) {
		n++
	}
	return n
}

func Break(prev, next ai.GenerateInput) string {
	if !SameHead(prev, next) {
		return "the model, system prompt, toolset or clock changed"
	}
	return MessagesBreak(Stable(prev), next.Messages)
}

func MessagesBreak(want, got []ai.Message) string {
	if len(got) < len(want) {
		return fmt.Sprintf("the conversation shrank from %d to %d messages", len(want), len(got))
	}
	if at := SharedMessages(want, got); at < len(want) {
		return fmt.Sprintf("message %d was rewritten: %s became %s", at, Clip(Wire(want[at])), Clip(Wire(got[at])))
	}
	return ""
}

func Breaks(inputs []ai.GenerateInput, rewrites ...int) []string {
	deliberate := map[int]bool{}
	for _, call := range rewrites {
		deliberate[call] = true
	}
	var out []string
	for i := 1; i < len(inputs); i++ {
		if deliberate[i] {
			continue
		}
		if why := Break(inputs[i-1], inputs[i]); why != "" {
			out = append(out, fmt.Sprintf("call %d does not extend call %d: %s", i+1, i, why))
		}
	}
	return out
}

func AssertAppendOnly(t *testing.T, inputs []ai.GenerateInput, rewrites ...int) {
	t.Helper()
	for _, why := range Breaks(inputs, rewrites...) {
		t.Error(why)
	}
}
