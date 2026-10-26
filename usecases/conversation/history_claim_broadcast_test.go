package conversation_usecase

import (
	"context"
	"sync"
	"testing"

	"vozko/domain/conversation"
)

type claimWatchingHub struct {
	conversation.EventBroadcaster
	mu      sync.Mutex
	created int
	updated []*conversation.Message
}

func (h *claimWatchingHub) BroadcastNewMessage(string, string, *conversation.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.created++
}

func (h *claimWatchingHub) BroadcastMessageUpdated(_, _ string, message *conversation.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.updated = append(h.updated, message)
}

func TestMessageHistoryManager_Record_AClaimedEchoIsRepaintedForOpenThreads(t *testing.T) {
	repo := &dedupMessageRepo{}
	hub := &claimWatchingHub{}
	mgr := NewMessageHistoryManagerWithHub(repo, hub)

	if err := mgr.Record(context.Background(), providerRecord("entry-1", "PROVIDER-MENU", conversation.SentExternally())); err != nil {
		t.Fatalf("echo: %v", err)
	}
	if err := mgr.Record(context.Background(), providerRecord("entry-1", "PROVIDER-MENU", conversation.SentByWorkflow("wf-1"))); err != nil {
		t.Fatalf("send: %v", err)
	}

	if hub.created != 1 || len(hub.updated) != 1 {
		t.Fatalf("new %d, updated %d: want the echo announced once and repainted once claimed", hub.created, len(hub.updated))
	}
	if repainted := hub.updated[0]; repainted.SentBy != conversation.SentByWorkflow("wf-1") {
		t.Fatalf("repainted sender %+v, want the stored row as the workflow's", repainted.SentBy)
	}
}

func TestMessageHistoryManager_Record_ASilentClaimRepaintsNothing(t *testing.T) {
	repo := &dedupMessageRepo{}
	hub := &claimWatchingHub{}
	mgr := NewMessageHistoryManagerWithHub(repo, hub)
	echo := providerRecord("entry-1", "PROVIDER-MENU", conversation.SentExternally())
	echo.Silent = true
	send := providerRecord("entry-1", "PROVIDER-MENU", conversation.SentByWorkflow("wf-1"))
	send.Silent = true

	for _, record := range []conversation.MessageHistoryRecord{echo, send} {
		if err := mgr.Record(context.Background(), record); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	if len(hub.updated) != 0 {
		t.Fatalf("updated %d, want an import to stay silent", len(hub.updated))
	}
}
