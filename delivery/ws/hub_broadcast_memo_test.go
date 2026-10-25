package ws

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"vozko/domain/conversation"
)

type scopeCountingAuthorizer struct {
	hubDepartmentTestAuthorizer
	mu     sync.Mutex
	scopes map[string]int
}

func (a *scopeCountingAuthorizer) GetDepartmentScope(userID, workspaceID string, isAdmin bool) (conversation.DepartmentAccessScope, bool) {
	a.mu.Lock()
	a.scopes[userID+"|"+workspaceID]++
	a.mu.Unlock()
	return a.hubDepartmentTestAuthorizer.GetDepartmentScope(userID, workspaceID, isAdmin)
}

func TestConnectedUsersBroadcastReadsEachDepartmentScopeOnce(t *testing.T) {
	departments := []string{"dept-a", "dept-b", "dept-c"}
	auth := &scopeCountingAuthorizer{
		hubDepartmentTestAuthorizer: hubDepartmentTestAuthorizer{departmentScopes: map[string]conversation.DepartmentAccessScope{}},
		scopes:                      map[string]int{},
	}
	hub := testHub(ConversationHubDeps{Authorizer: auth})
	conns := map[string]*WSConnection{}
	for i := 0; i < 6; i++ {
		user := fmt.Sprintf("user-%d", i)
		auth.departmentScopes[user] = conversation.DepartmentAccessScope{
			DepartmentIDs: []string{departments[i%3]}, Restrict: true, WorkspaceHasDepartments: true,
		}
		conn := &WSConnection{ID: "conn-" + user, UserID: user, WorkspaceID: "ws-1", ViewMode: "global", Send: make(chan []byte, 4)}
		hub.connections[conn.ID] = conn
		hub.userConnections[user] = map[string]bool{conn.ID: true}
		conns[user] = conn
	}

	hub.broadcastConnectedUsersToWorkspace("ws-1")

	for i := 0; i < 6; i++ {
		user := fmt.Sprintf("user-%d", i)
		seen := drainConnectedUsers(t, conns[user])
		sort.Strings(seen)
		want := []string{fmt.Sprintf("user-%d", i%3), fmt.Sprintf("user-%d", i%3+3)}
		require.Equal(t, want, seen, "%s sees exactly the people of its own department", user)
		require.Equal(t, 1, auth.scopes[user+"|ws-1"], "%s's department scope must be read once per broadcast", user)
	}
}

type countingStatusInbox struct {
	conversation.InboxService
	mu     sync.Mutex
	counts map[string]int64
	reads  int
}

func (c *countingStatusInbox) BuildInboxEntry(entryID, entryType string) (*conversation.InboxEntry, error) {
	return &conversation.InboxEntry{EntryID: entryID, EntryType: entryType, ConversationStatus: conversation.ConversationStatusNew}, nil
}

func (c *countingStatusInbox) GetConversationStatusCounts(string, string, string) (map[string]int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads++
	out := make(map[string]int64, len(c.counts))
	for k, v := range c.counts {
		out[k] = v
	}
	return out, nil
}

func (c *countingStatusInbox) setCount(status string, n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[status] = n
}

func (c *countingStatusInbox) readCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reads
}

func statusCountsHub(t *testing.T) (*ConversationHub, *WSConnection, *countingStatusInbox) {
	t.Helper()
	hub := testHub(ConversationHubDeps{Authorizer: &hubDepartmentTestAuthorizer{entryAccess: map[string]bool{"viewer": true}}})
	hub.statusCountsWindow = 40 * time.Millisecond
	inbox := &countingStatusInbox{counts: map[string]int64{"new": 1}}
	hub.cmds().Inbox = inbox
	hub.workspaceResolver = fixedWorkspace{workspaceID: "ws-1"}
	viewer := &WSConnection{ID: "conn-viewer", UserID: "viewer", WorkspaceID: "ws-1", Send: make(chan []byte, 32)}
	hub.connections[viewer.ID] = viewer
	hub.userConnections[viewer.UserID] = map[string]bool{viewer.ID: true}
	return hub, viewer, inbox
}

func statusCountsWithin(t *testing.T, conn *WSConnection, wait time.Duration) []map[string]int64 {
	t.Helper()
	var pushed []map[string]int64
	deadline := time.After(wait)
	for {
		select {
		case raw := <-conn.Send:
			var msg struct {
				Type    WSEventType `json:"type"`
				Payload struct {
					Counts map[string]int64 `json:"counts"`
				} `json:"payload"`
			}
			require.NoError(t, json.Unmarshal(raw, &msg))
			if msg.Type == WSEventConversationStatusCountsUpdate {
				pushed = append(pushed, msg.Payload.Counts)
			}
		case <-deadline:
			return pushed
		}
	}
}

func TestStatusCountsFromABurstOfChangesArriveOnceWithTheLatestValues(t *testing.T) {
	hub, viewer, inbox := statusCountsHub(t)

	for i := 0; i < 5; i++ {
		hub.broadcastEntryUpdateLocal("entry-1", "whatsapp", entryChanged)
	}
	inbox.setCount("new", 5)

	pushed := statusCountsWithin(t, viewer, 300*time.Millisecond)
	require.Equal(t, []map[string]int64{{"new": 5}}, pushed, "a burst is one push carrying the counts as they are when it fires")
	require.Equal(t, 1, inbox.readCount(), "the counts are read once for the whole burst")
}

func TestStatusCountsAfterTheWindowAreNotLost(t *testing.T) {
	hub, viewer, inbox := statusCountsHub(t)

	hub.broadcastEntryUpdateLocal("entry-1", "whatsapp", entryChanged)
	require.Len(t, statusCountsWithin(t, viewer, 200*time.Millisecond), 1)

	inbox.setCount("new", 2)
	hub.broadcastEntryUpdateLocal("entry-1", "whatsapp", entryChanged)
	require.Equal(t, []map[string]int64{{"new": 2}}, statusCountsWithin(t, viewer, 200*time.Millisecond))
}

func TestARefreshStillPushesNoStatusCounts(t *testing.T) {
	hub, viewer, inbox := statusCountsHub(t)

	hub.broadcastEntryUpdateLocal("entry-1", "whatsapp", entryRefreshed)

	require.Empty(t, statusCountsWithin(t, viewer, 150*time.Millisecond))
	require.Equal(t, 0, inbox.readCount())
}

func TestStatusCountsOfDifferentScopesAreNotMerged(t *testing.T) {
	hub, viewer, inbox := statusCountsHub(t)
	other := &WSConnection{ID: "conn-other", UserID: "other", WorkspaceID: "ws-2", Send: make(chan []byte, 32)}
	hub.connections[other.ID] = other
	hub.userConnections[other.UserID] = map[string]bool{other.ID: true}

	hub.scheduleStatusCounts("ws-1", "", "whatsapp")
	hub.scheduleStatusCounts("ws-2", "", "whatsapp")

	require.Len(t, statusCountsWithin(t, viewer, 200*time.Millisecond), 1)
	require.Len(t, statusCountsWithin(t, other, 50*time.Millisecond), 1)
	require.Equal(t, 2, inbox.readCount())
}
