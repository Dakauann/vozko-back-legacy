package carriermessaging_usecase

import (
	"context"
	"testing"
	"time"

	"vozko/domain/conversation"
	"vozko/domain/rcs"
)

type ownerAwareTranscript struct {
	inbox         *fakeInbox
	ownedAtRecord []bool
}

func (t *ownerAwareTranscript) Record(_ context.Context, r conversation.MessageHistoryRecord) error {
	t.inbox.mu.Lock()
	defer t.inbox.mu.Unlock()
	owned := false
	for _, conversationID := range t.inbox.assignments {
		owned = owned || conversationID == r.EntryID
	}
	t.ownedAtRecord = append(t.ownedAtRecord, owned)
	return nil
}

func TestACarrierReplyReachesTheInboxOnlyAfterItsConversationHasAnOwner(t *testing.T) {
	h := newHarness(t)
	deliveredMessage(t, h, rcs.Content{})
	transcript := &ownerAwareTranscript{inbox: h.inbox}
	h.svc.Transcript = transcript
	h.now = t0.Add(time.Minute)

	if err := h.svc.HandleInbound(context.Background(), reply("in-1", "Quero", h.now)); err != nil {
		t.Fatalf("HandleInbound = %v", err)
	}

	if len(transcript.ownedAtRecord) != 1 || !transcript.ownedAtRecord[0] {
		t.Fatalf("owned at record = %v, the reply was broadcast before the conversation had an owner", transcript.ownedAtRecord)
	}
}
