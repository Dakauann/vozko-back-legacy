package unofficial_whatsapp

import (
	"context"
	"sync"
	"testing"
	"time"

	"vozko/domain/conversation"
)

type arrivalOrder struct {
	mu    sync.Mutex
	steps []string
}

func (o *arrivalOrder) Record(_ context.Context, _ conversation.MessageHistoryRecord) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.steps = append(o.steps, "broadcast")
	return nil
}

func (o *arrivalOrder) EnsureAssignment(_, _, _ string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.steps = append(o.steps, "owner")
	return "workflow:wf-1"
}

func (o *arrivalOrder) first() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.steps) == 0 {
		return ""
	}
	return o.steps[0]
}

func TestACustomerMessageReachesTheInboxOnlyAfterItsConversationHasAnOwner(t *testing.T) {
	h := newGroupHarness(t, false).withFreshGate()
	order := &arrivalOrder{}
	h.uc.history = order
	h.uc.assignments = order

	h.deliver(t, map[string]any{
		"messageid": "msg-first", "chatid": "5511999999999@s.whatsapp.net",
		"sender": "5511999999999@s.whatsapp.net", "senderName": "Miriam",
		"messageType": "text", "text": "oi",
		"messageTimestamp": time.Now().UnixMilli(),
	})

	if got := order.first(); got != "owner" {
		t.Fatalf("steps = %v, the message was broadcast before the conversation had an owner", order.steps)
	}
}
