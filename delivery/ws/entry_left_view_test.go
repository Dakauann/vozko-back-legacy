package ws

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"vozko/domain/conversation"
)

func finishedEntryHub(t *testing.T, access map[string]bool) *ConversationHub {
	t.Helper()
	hub := testHub(ConversationHubDeps{Authorizer: &hubDepartmentTestAuthorizer{entryAccess: access, viewOthers: access}})
	hub.historyProvider = &statusTestHistoryProvider{
		entries: map[string]*conversation.InboxEntry{
			"entry-1|whatsapp": {EntryID: "entry-1", EntryType: "whatsapp", ConversationStatus: conversation.ConversationStatusFinished},
		},
	}
	hub.cmds().Inbox = historyInbox{history: hub.historyProvider}
	return hub
}

func watching(hub *ConversationHub, conns ...*WSConnection) {
	for _, c := range conns {
		hub.connections[c.ID] = c
		hub.userConnections[c.UserID] = map[string]bool{c.ID: true}
	}
}

func receivedEvent(t *testing.T, conn *WSConnection) WSOutgoingMessage {
	t.Helper()
	require.Len(t, conn.Send, 1)
	return receivedEventAt(t, conn)
}

func receivedEventAt(t *testing.T, conn *WSConnection) WSOutgoingMessage {
	t.Helper()
	require.NotEmpty(t, conn.Send)
	var msg WSOutgoingMessage
	require.NoError(t, json.Unmarshal(<-conn.Send, &msg))
	return msg
}

func TestAFinishedConversationLeavesTheActiveTabByTheServersWord(t *testing.T) {
	hub := finishedEntryHub(t, map[string]bool{"user-active": true, "user-finished": true})
	active := &WSConnection{ID: "conn-active", UserID: "user-active", WorkspaceID: "ws-1", Send: make(chan []byte, 2)}
	finished := &WSConnection{ID: "conn-finished", UserID: "user-finished", WorkspaceID: "ws-1", ConversationStatus: "finished", Send: make(chan []byte, 2)}
	watching(hub, active, finished)

	hub.broadcastEntryUpdateLocal("entry-1", "whatsapp", entryChanged)

	left := receivedEvent(t, active)
	require.Equal(t, WSEventEntryRemoved, left.Type)
	payload, _ := json.Marshal(left.Payload)
	var removed EntryRemovedPayload
	require.NoError(t, json.Unmarshal(payload, &removed))
	require.Equal(t, EntryRemovedPayload{EntryID: "entry-1", EntryType: "whatsapp", Reason: entryLeftViewReason}, removed)

	require.Equal(t, WSEventEntryUpdate, receivedEvent(t, finished).Type)
}

func TestAConversationNobodyMaySeeNeverLeavesTheirViewEither(t *testing.T) {
	hub := finishedEntryHub(t, map[string]bool{})
	outsider := &WSConnection{ID: "conn-outsider", UserID: "user-outsider", WorkspaceID: "ws-1", Send: make(chan []byte, 2)}
	watching(hub, outsider)

	hub.broadcastEntryUpdateLocal("entry-1", "whatsapp", entryChanged)

	require.Len(t, outsider.Send, 0)
}
