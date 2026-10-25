package ws

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"vozko/domain/cache"
	"vozko/domain/conversation"
)

type stubAttention struct {
	mu         sync.Mutex
	attention  conversation.MessageAttention
	knownOwner []string
}

func (s *stubAttention) MessageAttention(_, _, _, knownOwner string) conversation.MessageAttention {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.knownOwner = append(s.knownOwner, knownOwner)
	return s.attention
}

func (s *stubAttention) reads() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.knownOwner)
}

type publishedBroadcasts struct {
	cache.SharedState
	mu       sync.Mutex
	payloads [][]byte
}

func (p *publishedBroadcasts) Publish(_ string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.payloads = append(p.payloads, data)
	return nil
}

func (p *publishedBroadcasts) entryUpdates(t *testing.T) []redisWorkspaceBroadcast {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []redisWorkspaceBroadcast
	for _, raw := range p.payloads {
		var b redisWorkspaceBroadcast
		require.NoError(t, json.Unmarshal(raw, &b))
		if b.Type == "entry_update" {
			out = append(out, b)
		}
	}
	return out
}

type attentionWorld struct {
	hub       *ConversationHub
	owner     *WSConnection
	colleague *WSConnection
	attention *stubAttention
	published *publishedBroadcasts
}

func newAttentionWorld(t *testing.T, owner string, attention conversation.MessageAttention) *attentionWorld {
	t.Helper()
	authorizer := &hubDepartmentTestAuthorizer{
		entryAccess: map[string]bool{"u-owner": true, "u-colleague": true},
		viewOthers:  map[string]bool{"u-owner": true, "u-colleague": true},
	}
	published := &publishedBroadcasts{}
	hub := testHub(ConversationHubDeps{Authorizer: authorizer, SharedState: published})
	hub.historyProvider = &statusTestHistoryProvider{entries: map[string]*conversation.InboxEntry{
		"entry-1|whatsapp": {EntryID: "entry-1", EntryType: "whatsapp", AssignedUserID: owner, ConversationStatus: conversation.ConversationStatusNew},
	}}
	hub.cmds().Inbox = historyInbox{history: hub.historyProvider}
	hub.workspaceResolver = fixedWorkspace{workspaceID: "ws-1"}
	stub := &stubAttention{attention: attention}
	hub.cmds().Attention = stub

	w := &attentionWorld{hub: hub, attention: stub, published: published}
	w.owner = w.connect("u-owner")
	w.colleague = w.connect("u-colleague")
	return w
}

func (w *attentionWorld) connect(userID string) *WSConnection {
	conn := &WSConnection{ID: "conn-" + userID, UserID: userID, WorkspaceID: "ws-1", Send: make(chan []byte, 8)}
	w.hub.connections[conn.ID] = conn
	w.hub.userConnections[userID] = map[string]bool{conn.ID: true}
	return conn
}

func contactMessage() *conversation.Message {
	return &conversation.Message{ID: "m-1", EntryID: "entry-1", SentBy: conversation.SentByContact("5511999990000")}
}

func operatorMessage() *conversation.Message {
	return &conversation.Message{ID: "m-2", EntryID: "entry-1", SentBy: conversation.SentByPerson("u-owner")}
}

func TestACustomerMessageAlertsOnlyTheOwnerWhileEveryViewerGetsTheUpdate(t *testing.T) {
	w := newAttentionWorld(t, "u-owner", conversation.MessageAttention{Owner: "u-owner"})

	w.hub.BroadcastEntryUpdate("entry-1", "whatsapp", contactMessage())

	ownerUpdate := awaitEntryUpdate(t, w.owner)
	colleagueUpdate := awaitEntryUpdate(t, w.colleague)
	require.Equal(t, "entry-1", colleagueUpdate.Entry.EntryID, "the inbox list still updates for everyone who sees it")
	require.True(t, ownerUpdate.Alert)
	require.False(t, colleagueUpdate.Alert)
	require.Equal(t, []string{"u-owner"}, w.attention.knownOwner, "the facts are read once per update, not once per viewer")
}

func TestAManualQueueMessageAlertsOnlyTheMembersWhoCanTakeIt(t *testing.T) {
	w := newAttentionWorld(t, "", conversation.MessageAttention{Distribution: conversation.DistributionManual, Takers: []string{"u-colleague"}})

	w.hub.BroadcastEntryUpdate("entry-1", "whatsapp", contactMessage())

	require.False(t, awaitEntryUpdate(t, w.owner).Alert)
	require.True(t, awaitEntryUpdate(t, w.colleague).Alert)
}

func TestOurOwnRepliesAndRefreshesAlertNobody(t *testing.T) {
	w := newAttentionWorld(t, "u-owner", conversation.MessageAttention{Owner: "u-owner"})

	w.hub.BroadcastEntryUpdate("entry-1", "whatsapp", operatorMessage())
	require.False(t, awaitEntryUpdate(t, w.owner).Alert)
	require.False(t, awaitEntryUpdate(t, w.colleague).Alert)

	w.hub.BroadcastEntryUpdate("entry-1", "whatsapp", nil)
	require.False(t, awaitEntryUpdate(t, w.owner).Alert)
	require.False(t, awaitEntryUpdate(t, w.colleague).Alert)

	require.Zero(t, w.attention.reads(), "only a customer message needs the alert facts")
}

func TestTheOtherReplicasLearnTheUpdateCarriesACustomerMessage(t *testing.T) {
	w := newAttentionWorld(t, "u-owner", conversation.MessageAttention{Owner: "u-owner"})

	w.hub.BroadcastEntryUpdate("entry-1", "whatsapp", contactMessage())
	w.hub.BroadcastEntryUpdate("entry-1", "whatsapp", operatorMessage())

	relayed := w.published.entryUpdates(t)
	require.Len(t, relayed, 2)
	require.True(t, relayed[0].CustomerMessage)
	require.False(t, relayed[1].CustomerMessage)
}

func TestAnotherReplicasCustomerMessageAlertsTheSameViewers(t *testing.T) {
	w := newAttentionWorld(t, "u-owner", conversation.MessageAttention{Owner: "u-owner"})

	w.hub.deliverWorkspaceBroadcast(redisWorkspaceBroadcast{Type: "entry_update", EntryID: "entry-1", EntryType: "whatsapp", CustomerMessage: true, ReplicaID: "other"})

	require.True(t, awaitEntryUpdate(t, w.owner).Alert)
	require.False(t, awaitEntryUpdate(t, w.colleague).Alert)

	w.hub.deliverWorkspaceBroadcast(redisWorkspaceBroadcast{Type: "entry_update", EntryID: "entry-1", EntryType: "whatsapp", ReplicaID: "other"})

	require.False(t, awaitEntryUpdate(t, w.owner).Alert)
	require.False(t, awaitEntryUpdate(t, w.colleague).Alert)
}

func TestTheAssigneeAloneHearsTheAssignment(t *testing.T) {
	w := newAttentionWorld(t, "u-owner", conversation.MessageAttention{})

	w.hub.AlertAssignee("ws-1", "entry-1", "whatsapp", "u-owner")

	select {
	case raw := <-w.owner.Send:
		var env wsEventEnvelope
		require.NoError(t, json.Unmarshal(raw, &env))
		require.Equal(t, WSEventAssignedToYou, env.Type)
		var payload AssignedToYouPayload
		require.NoError(t, json.Unmarshal(env.Payload, &payload))
		require.Equal(t, "entry-1", payload.Entry.EntryID)
		require.Equal(t, "whatsapp", payload.Entry.EntryType)
		require.False(t, payload.AssignedAt.IsZero())
	case <-time.After(2 * time.Second):
		t.Fatal("the assignee must hear the assignment")
	}
	requireQuiet(t, w.colleague)
}

func TestAnAssignmentFromAnotherReplicaReachesTheLocalAssignee(t *testing.T) {
	w := newAttentionWorld(t, "u-owner", conversation.MessageAttention{})

	w.hub.AlertAssignee("ws-1", "entry-1", "whatsapp", "u-owner")
	<-w.owner.Send

	w.published.mu.Lock()
	var relayed redisWorkspaceBroadcast
	for _, raw := range w.published.payloads {
		var b redisWorkspaceBroadcast
		require.NoError(t, json.Unmarshal(raw, &b))
		if b.Type == assignedToYouBroadcastType {
			relayed = b
		}
	}
	w.published.mu.Unlock()
	require.Equal(t, []string{"u-owner"}, relayed.UserIDs)

	relayed.ReplicaID = "other"
	w.hub.deliverWorkspaceBroadcast(relayed)

	require.Len(t, w.owner.Send, 1)
	require.Len(t, w.colleague.Send, 0)
}
