package conversation_usecase

import (
	"context"
	"testing"

	"vozko/domain/channel"
	"vozko/domain/conversation"
	"vozko/usecases/conversation/loopguard"
)

type repeatGuard struct {
	threshold int
	seen      map[string]int
	recorded  []string
}

func newRepeatGuard(threshold int) *repeatGuard {
	return &repeatGuard{threshold: threshold, seen: map[string]int{}}
}

func (g *repeatGuard) CheckInbound(_ context.Context, _, conversationID, text string) loopguard.Decision {
	key := conversationID + "|" + text
	g.seen[key]++
	if g.seen[key] >= g.threshold {
		return loopguard.Decision{Block: true, Reason: loopguard.ReasonDuplicateInbound, Count: int64(g.seen[key])}
	}
	return loopguard.Decision{Count: int64(g.seen[key])}
}

func (g *repeatGuard) RecordAIResponse(_ context.Context, _, conversationID string) loopguard.Decision {
	g.recorded = append(g.recorded, conversationID)
	return loopguard.Decision{}
}

func TestALoopingConversationOnANonOfficialChannelIsNotAnswered(t *testing.T) {
	guard := newRepeatGuard(3)
	h := newTextRulesHarnessWithGuard(t, channel.Capabilities{}, guard, "ok", "ok", "ok")
	req := textRulesRequest()

	for range 2 {
		if _, err := h.svc.Reply(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	msg, err := h.svc.Reply(context.Background(), req)
	if msg != nil || err != nil {
		t.Fatalf("msg = %v err = %v, want the looping message ignored", msg, err)
	}
	if len(h.ai.inputs) != 2 || len(h.sender.sent) != 2 {
		t.Fatalf("generations = %d sent = %d, want the third repeat blocked before the model runs", len(h.ai.inputs), len(h.sender.sent))
	}
}

func TestEveryAgentReplyOnANonOfficialChannelCountsTowardTheRateLimit(t *testing.T) {
	guard := newRepeatGuard(10)
	h := newTextRulesHarnessWithGuard(t, channel.Capabilities{}, guard, "ok")

	if _, err := h.svc.Reply(context.Background(), textRulesRequest()); err != nil {
		t.Fatal(err)
	}
	if len(guard.recorded) != 1 || guard.recorded[0] != "conv-1" {
		t.Fatalf("recorded = %v, want the reply counted for conv-1", guard.recorded)
	}
}

func TestNewChannelAIReplyServiceRefusesAMissingLoopGuard(t *testing.T) {
	var typedNil *repeatGuard
	for name, guard := range map[string]loopguard.Guard{"nil": nil, "typed nil": typedNil} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("a service without a loop guard must fail boot")
				}
			}()
			channelAIWith(ChannelAIReplyDeps{Agents: textRulesAgents{}, AI: &scriptedAI{}, Messages: stubMessageRepo{}, Sender: &recordingAgentSender{}, Adapters: conversation.NewAdapterRegistry(), HandOff: &recordingHandOff{}, Events: &recordingEvents{}, Guard: guard})
		})
	}
}
