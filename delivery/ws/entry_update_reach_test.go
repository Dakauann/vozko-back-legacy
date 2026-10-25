package ws

import (
	"fmt"
	"testing"

	"vozko/domain/conversation"
)

func entryUpdateReachedWithPermissionFirst(h *ConversationHub, update *entryUpdate, allowEntry func(*WSConnection) bool, conn *WSConnection) bool {
	if !allowEntry(conn) {
		return false
	}
	if update.workspaceID != "" && conn.WorkspaceID != "" && conn.WorkspaceID != update.workspaceID {
		return false
	}
	if update.campaignID != "" && conn.CampaignID != "" && conn.CampaignID != update.campaignID {
		return false
	}
	if conn.CampaignType != "" && conn.CampaignType != update.entryType {
		return false
	}
	if conn.ConversationStatus != "" && string(update.entry.ConversationStatus) != conn.ConversationStatus {
		return false
	}
	assignedUserID := update.entry.AssignedUserID
	if assignedUserID != "" && conn.UserID != assignedUserID && !conn.IsAdmin {
		return h.authorizer.HasWorkspacePermission(conn.UserID, conn.WorkspaceID, "conversations", "view_others", conn.IsAdmin)
	}
	return true
}

func cheapFiltersPass(update *entryUpdate, conn *WSConnection) bool {
	return !(update.workspaceID != "" && conn.WorkspaceID != "" && conn.WorkspaceID != update.workspaceID) &&
		!(update.campaignID != "" && conn.CampaignID != "" && conn.CampaignID != update.campaignID) &&
		!(conn.CampaignType != "" && conn.CampaignType != update.entryType) &&
		!(conn.ConversationStatus != "" && string(update.entry.ConversationStatus) != conn.ConversationStatus)
}

func TestEntryUpdateReachesAnswersExactlyAsBeforeForEveryCombination(t *testing.T) {
	const user = "operator"
	combinations := 0
	for _, permitted := range []bool{true, false} {
		for _, viewOthers := range []bool{true, false} {
			authorizer := &hubDepartmentTestAuthorizer{viewOthers: map[string]bool{user: viewOthers}}
			hub := testHub(ConversationHubDeps{Authorizer: authorizer})
			for _, updateWorkspace := range []string{"", "ws-1"} {
				for _, connWorkspace := range []string{"", "ws-1", "ws-2"} {
					for _, updateCampaign := range []string{"", "c-1"} {
						for _, connCampaign := range []string{"", "c-1", "c-2"} {
							for _, connType := range []string{"", "whatsapp", "telegram"} {
								for _, connStatus := range []string{"", "new", "closed"} {
									for _, assigned := range []string{"", user, "someone-else"} {
										for _, isAdmin := range []bool{true, false} {
											combinations++
											update := &entryUpdate{
												entryID: "entry-1", entryType: "whatsapp",
												workspaceID: updateWorkspace, campaignID: updateCampaign,
												entry: &conversation.InboxEntry{ConversationStatus: conversation.ConversationStatusNew, AssignedUserID: assigned},
											}
											conn := &WSConnection{
												UserID: user, WorkspaceID: connWorkspace, CampaignID: connCampaign,
												CampaignType: connType, ConversationStatus: connStatus, IsAdmin: isAdmin,
											}
											checks := 0
											allowEntry := func(*WSConnection) bool {
												checks++
												return permitted
											}
											name := fmt.Sprintf("permitted=%v viewOthers=%v ws=%q/%q campaign=%q/%q type=%q status=%q assigned=%q admin=%v",
												permitted, viewOthers, updateWorkspace, connWorkspace, updateCampaign, connCampaign, connType, connStatus, assigned, isAdmin)

											want := entryUpdateReachedWithPermissionFirst(hub, update, func(*WSConnection) bool { return permitted }, conn)
											got := hub.entryUpdateReaches(update, allowEntry, conn)

											if got != want {
												t.Fatalf("%s: reaches = %v, before = %v", name, got, want)
											}
											if !cheapFiltersPass(update, conn) && checks != 0 {
												t.Fatalf("%s: the permission check ran although a cheap filter already excluded the connection", name)
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if combinations != 2*2*2*3*2*3*3*3*3*2 {
		t.Fatalf("covered %d combinations", combinations)
	}
}
