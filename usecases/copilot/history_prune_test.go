package copilot_usecase

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"vozko/domain/ai"
	"vozko/domain/aichat"
)

func longReply(t *testing.T, calls int) *aichat.Message {
	t.Helper()
	var turn []ai.Message
	for i := range calls {
		id := fmt.Sprintf("c%d", i)
		turn = append(turn,
			ai.Message{Role: ai.RoleAssistant, ToolCalls: []ai.ToolCall{{ID: id, Name: "load_skill", Arguments: map[string]interface{}{"names": []string{id}}}}},
			ai.Message{Role: ai.RoleTool, ToolCallID: id, Content: id + " " + strings.Repeat("h", 40_000)},
		)
	}
	turn = append(turn, ai.Message{Role: ai.RoleAssistant, Content: "Peça pronta."})
	raw, err := aichat.EncodeTranscript(turn)
	if err != nil {
		t.Fatal(err)
	}
	return &aichat.Message{Role: aichat.RoleAssistant, Content: "Peça pronta.", Model: "m", Transcript: raw}
}

func TestTheNextReplyStartsWithTheOldToolOutputsPruned(t *testing.T) {
	ms := &fakeMessages{list: []*aichat.Message{{Role: aichat.RoleUser, Content: "crie o anúncio"}, longReply(t, 10)}}
	svc := newService(&scriptAI{}, &fakeThreads{thread: testThread()}, ms)
	h, err := svc.buildHistory("t1", "m", true)
	if err != nil {
		t.Fatal(err)
	}
	var pruned, kept int
	for _, m := range h {
		switch {
		case m.Role == ai.RoleTool && strings.HasPrefix(m.Content, "[resultado antigo de load_skill"):
			pruned++
		case m.Role == ai.RoleTool:
			kept++
		}
	}
	if pruned == 0 || kept == 0 {
		t.Fatalf("pruned %d and kept %d tool outputs, want the oldest pruned and the newest kept", pruned, kept)
	}
	if h[1].ToolCalls[0].Name != "load_skill" || h[len(h)-1].Content != "Peça pronta." {
		t.Fatal("the calls and the answer text are never pruned")
	}
	again, err := svc.buildHistory("t1", "m", true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(h, again) {
		t.Fatal("the same thread must rebuild to the same history so the cache holds")
	}
}
