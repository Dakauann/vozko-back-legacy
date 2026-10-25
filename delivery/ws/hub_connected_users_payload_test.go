package ws

import (
	"testing"

	"github.com/stretchr/testify/require"

	"vozko/domain/conversation"
)

func TestDepartmentPeersNeverShareAConnectedUsersListShapedByTheOthersCampaignAccess(t *testing.T) {
	authorizer := &hubDepartmentTestAuthorizer{
		campaignAccess: map[string]bool{
			"with-access|ws-1|whatsapp|camp-1":    true,
			"without-access|ws-1|whatsapp|camp-1": false,
		},
		departmentScopes: map[string]conversation.DepartmentAccessScope{
			"with-access":    {DepartmentIDs: []string{"dept-1"}, Restrict: true},
			"without-access": {DepartmentIDs: []string{"dept-1"}, Restrict: true},
			"campaign-user":  {DepartmentIDs: []string{"dept-1"}, Restrict: true},
		},
		workspaceMembers: map[string]bool{"with-access": true, "without-access": true, "campaign-user": true},
	}
	hub := testHub(ConversationHubDeps{Authorizer: authorizer})
	withAccess := &WSConnection{ID: "with-access", UserID: "with-access", WorkspaceID: "ws-1", DepartmentID: "dept-1", ViewMode: "global", Send: make(chan []byte, 1)}
	withoutAccess := &WSConnection{ID: "without-access", UserID: "without-access", WorkspaceID: "ws-1", DepartmentID: "dept-1", ViewMode: "global", Send: make(chan []byte, 1)}
	campaignUser := &WSConnection{ID: "campaign-user", UserID: "campaign-user", WorkspaceID: "ws-1", CampaignID: "camp-1", CampaignType: "whatsapp", ViewMode: "campaign", Send: make(chan []byte, 1)}
	for _, conn := range []*WSConnection{withAccess, withoutAccess, campaignUser} {
		hub.connections[conn.ID] = conn
	}

	for round := 0; round < 64; round++ {
		hub.broadcastConnectedUsersToWorkspace("ws-1")

		require.ElementsMatch(t, []string{"with-access", "without-access", "campaign-user"}, drainConnectedUsers(t, withAccess), "round %d", round)
		require.ElementsMatch(t, []string{"with-access", "without-access"}, drainConnectedUsers(t, withoutAccess), "round %d", round)
		drainConnectedUsers(t, campaignUser)
	}
}
