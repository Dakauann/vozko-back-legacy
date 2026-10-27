package ws

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"vozko/domain/conversation"
	"vozko/domain/personalinbox"
)

type groupWorld struct {
	hub       *ConversationHub
	published *publishedBroadcasts
}

func newGroupWorld(t *testing.T, owner string, attention conversation.MessageAttention) *groupWorld {
	t.Helper()
	authorizer := &hubDepartmentTestAuthorizer{
		entryAccess: map[string]bool{"u-owner": true, "u-colleague": true},
		viewOthers:  map[string]bool{"u-owner": true, "u-colleague": true},
	}
	published := &publishedBroadcasts{}
	hub := testHub(ConversationHubDeps{Authorizer: authorizer, SharedState: published})
	hub.historyProvider = &statusTestHistoryProvider{entries: map[string]*conversation.InboxEntry{
		"group-1|unofficial_whatsapp":  {EntryID: "group-1", EntryType: "unofficial_whatsapp", IsGroup: true, AssignedUserID: owner, ConversationStatus: conversation.ConversationStatusOngoing},
		"client-1|unofficial_whatsapp": {EntryID: "client-1", EntryType: "unofficial_whatsapp", AssignedUserID: owner, ConversationStatus: conversation.ConversationStatusNew},
	}}
	hub.cmds().Inbox = historyInbox{history: hub.historyProvider}
	hub.cmds().Attention = &stubAttention{attention: attention}
	hub.workspaceResolver = fixedWorkspace{workspaceID: "ws-1"}
	return &groupWorld{hub: hub, published: published}
}

func (w *groupWorld) connect(userID string, groupsView bool) *WSConnection {
	conn := &WSConnection{ID: "conn-" + userID, UserID: userID, WorkspaceID: "ws-1", GroupsView: groupsView, Send: make(chan []byte, 8)}
	w.hub.connections[conn.ID] = conn
	w.hub.userConnections[userID] = map[string]bool{conn.ID: true}
	return conn
}

func groupContactMessage(entryID string) *conversation.Message {
	return &conversation.Message{ID: "m-" + entryID, EntryID: entryID, SentBy: conversation.SentByContact("5511999990000")}
}

func groupOperatorMessage(entryID string) *conversation.Message {
	return &conversation.Message{ID: "o-" + entryID, EntryID: entryID, SentBy: conversation.SentByPerson("u-owner")}
}

func TestAGroupUpdateJoinsOnlyTheGroupsTabList(t *testing.T) {
	w := newGroupWorld(t, "", conversation.MessageAttention{})
	clients := w.connect("u-owner", false)
	groups := w.connect("u-colleague", true)

	w.hub.BroadcastEntryUpdate("group-1", "unofficial_whatsapp", groupOperatorMessage("group-1"))

	listed := awaitEntryUpdate(t, groups)
	require.Equal(t, "group-1", listed.Entry.EntryID)
	require.False(t, listed.Unlisted)
	requireQuiet(t, clients)
}

func TestAClientUpdateJoinsOnlyTheClientTabsList(t *testing.T) {
	w := newGroupWorld(t, "", conversation.MessageAttention{})
	clients := w.connect("u-owner", false)
	groups := w.connect("u-colleague", true)

	w.hub.BroadcastEntryUpdate("client-1", "unofficial_whatsapp", groupOperatorMessage("client-1"))

	require.False(t, awaitEntryUpdate(t, clients).Unlisted)
	requireQuiet(t, groups)
}

func TestAGroupMessageStillAlertsItsOwnerOnTheClientTabsWithoutJoiningTheList(t *testing.T) {
	w := newGroupWorld(t, "u-owner", conversation.MessageAttention{Owner: "u-owner"})
	owner := w.connect("u-owner", false)
	colleague := w.connect("u-colleague", false)

	w.hub.BroadcastEntryUpdate("group-1", "unofficial_whatsapp", groupContactMessage("group-1"))

	alert := awaitEntryUpdate(t, owner)
	require.True(t, alert.Alert)
	require.True(t, alert.Unlisted, "the alert must not put the group in the client list")
	requireQuiet(t, colleague)
}

func TestAGroupMessageAlertsEvenFromAClientStatusTab(t *testing.T) {
	w := newGroupWorld(t, "u-owner", conversation.MessageAttention{Owner: "u-owner"})
	owner := w.connect("u-owner", false)
	owner.ConversationStatus = string(conversation.ConversationStatusNew)

	w.hub.BroadcastEntryUpdate("group-1", "unofficial_whatsapp", groupContactMessage("group-1"))

	alert := awaitEntryUpdate(t, owner)
	require.True(t, alert.Alert && alert.Unlisted)
}

func TestHidingGroupsSilencesTheirAlertsAndUpdates(t *testing.T) {
	w := newGroupWorld(t, "u-owner", conversation.MessageAttention{Owner: "u-owner"})
	owner := w.connect("u-owner", false)
	owner.hidesGroups.Store(true)
	owner2 := &WSConnection{ID: "conn-owner-groups", UserID: "u-owner", WorkspaceID: "ws-1", GroupsView: true, Send: make(chan []byte, 8)}
	owner2.hidesGroups.Store(true)
	w.hub.connections[owner2.ID] = owner2
	w.hub.userConnections["u-owner"][owner2.ID] = true

	w.hub.BroadcastEntryUpdate("group-1", "unofficial_whatsapp", groupContactMessage("group-1"))
	requireQuiet(t, owner)
	requireQuiet(t, owner2)

	w.hub.BroadcastEntryUpdate("client-1", "unofficial_whatsapp", groupContactMessage("client-1"))
	clientAlert := awaitEntryUpdate(t, owner)
	require.True(t, clientAlert.Alert)
	require.False(t, clientAlert.Unlisted, "hiding groups never hides a client")
}

func TestAClientMessageAlertsItsOwnerWhileTheyLookAtGroups(t *testing.T) {
	w := newGroupWorld(t, "u-owner", conversation.MessageAttention{Owner: "u-owner"})
	owner := w.connect("u-owner", true)

	w.hub.BroadcastEntryUpdate("client-1", "unofficial_whatsapp", groupContactMessage("client-1"))

	alert := awaitEntryUpdate(t, owner)
	require.True(t, alert.Alert && alert.Unlisted)
}

func TestTheInboxViewAsksForTheGroupScopeOfTheConnection(t *testing.T) {
	cases := []struct {
		name   string
		groups bool
		hides  bool
		want   conversation.GroupScope
	}{
		{"client tabs", false, false, conversation.GroupScopeClients},
		{"groups tab", true, false, conversation.GroupScopeGroups},
		{"groups tab while hidden", true, true, conversation.GroupScopeNone},
		{"client tabs while hidden", false, true, conversation.GroupScopeClients},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			conn := &WSConnection{UserID: "u", WorkspaceID: "ws-1", ViewMode: "global", GroupsView: c.groups}
			conn.hidesGroups.Store(c.hides)
			require.Equal(t, c.want, inboxViewOf(conn).searchInput(1, 20).Groups)
		})
	}
}

type recordingInbox struct {
	quietHubInbox
	mu     sync.Mutex
	inputs []conversation.SearchInboxInput
	done   chan struct{}
}

func (r *recordingInbox) SearchInbox(_ string, input conversation.SearchInboxInput) ([]conversation.InboxEntry, int64, error) {
	r.mu.Lock()
	r.inputs = append(r.inputs, input)
	r.mu.Unlock()
	r.done <- struct{}{}
	return nil, 0, nil
}

func TestTheInboxSearchFollowsTheViewUnlessItAsksOtherwise(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name      string
		groups    bool
		hides     bool
		requested *bool
		want      conversation.GroupScope
	}{
		{"client tabs", false, false, nil, conversation.GroupScopeClients},
		{"groups tab", true, false, nil, conversation.GroupScopeGroups},
		{"explicit groups", false, false, &yes, conversation.GroupScopeGroups},
		{"explicit clients on the groups tab", true, false, &no, conversation.GroupScopeClients},
		{"groups asked while hidden", false, true, &yes, conversation.GroupScopeNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hub := testHub(ConversationHubDeps{Authorizer: multiOpenAuthorizer{}})
			inbox := &recordingInbox{done: make(chan struct{}, 1)}
			hub.cmds().Inbox = inbox
			conn := &WSConnection{ID: "c", UserID: "u", WorkspaceID: "ws-1", ViewMode: "global", GroupsView: c.groups, Send: make(chan []byte, 8)}
			conn.hidesGroups.Store(c.hides)
			payload, _ := json.Marshal(SearchInboxPayload{IsGroup: c.requested})

			hub.handleSearchInbox(conn, payload)
			<-inbox.done

			inbox.mu.Lock()
			defer inbox.mu.Unlock()
			require.Equal(t, c.want, inbox.inputs[0].Groups)
		})
	}
}

func TestAnnouncedPreferencesReachEveryConnectionOfThatPersonOnly(t *testing.T) {
	w := newGroupWorld(t, "", conversation.MessageAttention{})
	ana := w.connect("u-owner", false)
	bia := w.connect("u-colleague", false)

	w.hub.AnnounceInboxPreferences("ws-1", "u-owner", personalinbox.Preferences{HideGroups: true})

	require.True(t, ana.hidesGroups.Load())
	require.False(t, bia.hidesGroups.Load())
	env := drainEvent(t, ana)
	require.Equal(t, WSEventInboxPreferences, env.Type)
	var prefs InboxPreferencesPayload
	require.NoError(t, json.Unmarshal(env.Payload, &prefs))
	require.True(t, prefs.HideGroups)
	requireQuiet(t, bia)
}

func TestPreferencesAnnouncedOnAnotherReplicaApplyHere(t *testing.T) {
	w := newGroupWorld(t, "", conversation.MessageAttention{})
	w.hub.AnnounceInboxPreferences("ws-1", "u-owner", personalinbox.Preferences{HideGroups: true})

	w.published.mu.Lock()
	var relayed redisWorkspaceBroadcast
	for _, raw := range w.published.payloads {
		var b redisWorkspaceBroadcast
		require.NoError(t, json.Unmarshal(raw, &b))
		if b.Type == inboxPreferencesBroadcastType {
			relayed = b
		}
	}
	w.published.mu.Unlock()
	require.Equal(t, []string{"u-owner"}, relayed.UserIDs)

	ana := w.connect("u-owner", false)
	relayed.ReplicaID = "other"
	w.hub.deliverWorkspaceBroadcast(relayed)

	require.True(t, ana.hidesGroups.Load())
	require.Equal(t, WSEventInboxPreferences, drainEvent(t, ana).Type)
}

type savedPreferencesMarks struct {
	quietHubMarks
	prefs personalinbox.Preferences
}

func (s savedPreferencesMarks) InboxPreferences(context.Context, string, string) personalinbox.Preferences {
	return s.prefs
}

func TestAConnectionStartsWithItsSavedPreferences(t *testing.T) {
	hub := testHub(ConversationHubDeps{Authorizer: multiOpenAuthorizer{}})
	hub.cmds().PersonalMarks = savedPreferencesMarks{prefs: personalinbox.Preferences{HideGroups: true}}
	conn := &WSConnection{ID: "c", UserID: "u", WorkspaceID: "ws-1", Send: make(chan []byte, 8)}

	view := hub.loadInboxPreferences(conn, inboxViewOf(conn))

	require.True(t, conn.hidesGroups.Load())
	require.True(t, view.hidesGroups)
	env := drainEvent(t, conn)
	require.Equal(t, WSEventInboxPreferences, env.Type)
}
