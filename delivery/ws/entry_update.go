package ws

import (
	"encoding/json"
	"log"

	"vozko/domain/conversation"
)

type entryUpdate struct {
	entryID     string
	entryType   string
	workspaceID string
	campaignID  string
	entry       *conversation.InboxEntry
	data        []byte
	alertData   []byte
}

func (h *ConversationHub) entryWorkspaceID(entryID, entryType string) string {
	workspaceID, _ := h.workspaceResolver.GetEntryWorkspaceID(entryID, entryType)
	return workspaceID
}

func (h *ConversationHub) inboxEntry(entryID, entryType string) (*conversation.InboxEntry, bool) {
	commands := h.cmds()
	if commands == nil {
		log.Printf("[ConversationHub] inbox entry %s (%s) not built: %v", entryID, entryType, ErrHubCommandsUnbound)
		return nil, false
	}
	entry, err := commands.Inbox.BuildInboxEntry(entryID, entryType)
	if err != nil {
		log.Printf("[ConversationHub] Error building inbox entry %s (%s): %v", entryID, entryType, err)
		return nil, false
	}
	if entry == nil {
		log.Printf("[ConversationHub] inbox entry %s (%s) not found", entryID, entryType)
		return nil, false
	}
	return entry, true
}

func (h *ConversationHub) buildEntryUpdate(entryID, entryType, workspaceID string, change entryChange) (*entryUpdate, bool) {
	entry, ok := h.inboxEntry(entryID, entryType)
	if !ok {
		return nil, false
	}

	var campaignID string
	campaignID, _ = h.workspaceResolver.GetEntryCampaignID(entryID, entryType)

	payload := EntryUpdatePayload{Entry: *entry, Silent: change == entryRefreshed}
	data, err := json.Marshal(&WSOutgoingMessage{Type: WSEventEntryUpdate, Payload: payload})
	if err != nil {
		log.Printf("[ConversationHub] Error marshaling entry update: %v", err)
		return nil, false
	}
	update := &entryUpdate{
		entryID: entryID, entryType: entryType, workspaceID: workspaceID,
		campaignID: campaignID, entry: entry, data: data,
	}
	if change == customerWrote {
		payload.Alert = true
		if update.alertData, err = json.Marshal(&WSOutgoingMessage{Type: WSEventEntryUpdate, Payload: payload}); err != nil {
			log.Printf("[ConversationHub] Error marshaling entry update alert: %v", err)
			return nil, false
		}
	}
	return update, true
}

func connInOtherWorkspace(conn *WSConnection, workspaceID string) bool {
	return workspaceID != "" && conn.WorkspaceID != "" && conn.WorkspaceID != workspaceID
}

func (h *ConversationHub) entryUpdateReaches(update *entryUpdate, allowEntry func(*WSConnection) bool, conn *WSConnection) bool {
	if connInOtherWorkspace(conn, update.workspaceID) {
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
	if !allowEntry(conn) {
		return false
	}
	assignedUserID := update.entry.AssignedUserID
	if assignedUserID != "" && conn.UserID != assignedUserID && !conn.IsAdmin {
		return h.authorizer.HasWorkspacePermission(conn.UserID, conn.WorkspaceID, "conversations", "view_others", conn.IsAdmin)
	}
	return true
}

func (h *ConversationHub) deliverEntryUpdate(conn *WSConnection, update *entryUpdate, alert bool) {
	data := update.data
	if alert && update.alertData != nil {
		data = update.alertData
	}
	select {
	case conn.Send <- data:
	default:
		log.Printf("[ConversationHub] Send buffer full for user %s (connection %s)", conn.UserID, conn.ID)
	}
}
