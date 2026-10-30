package ws

import (
	"encoding/json"
	"log"

	"vozko/domain/conversation"
)

const entryLeftViewReason = "left_view"

type entryUpdate struct {
	entryID      string
	entryType    string
	workspaceID  string
	campaignID   string
	entry        *conversation.InboxEntry
	data         []byte
	alertData    []byte
	unlistedData []byte
	leftViewData []byte
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
	leftViewData, err := json.Marshal(&WSOutgoingMessage{
		Type:    WSEventEntryRemoved,
		Payload: EntryRemovedPayload{EntryID: entryID, EntryType: entryType, Reason: entryLeftViewReason},
	})
	if err != nil {
		log.Printf("[ConversationHub] Error marshaling entry left view: %v", err)
		return nil, false
	}
	update := &entryUpdate{
		entryID: entryID, entryType: entryType, workspaceID: workspaceID,
		campaignID: campaignID, entry: entry, data: data, leftViewData: leftViewData,
	}
	if change == customerWrote {
		payload.Alert = true
		if update.alertData, err = json.Marshal(&WSOutgoingMessage{Type: WSEventEntryUpdate, Payload: payload}); err != nil {
			log.Printf("[ConversationHub] Error marshaling entry update alert: %v", err)
			return nil, false
		}
		payload.Unlisted = true
		if update.unlistedData, err = json.Marshal(&WSOutgoingMessage{Type: WSEventEntryUpdate, Payload: payload}); err != nil {
			log.Printf("[ConversationHub] Error marshaling unlisted entry update alert: %v", err)
			return nil, false
		}
	}
	return update, true
}

func connInOtherWorkspace(conn *WSConnection, workspaceID string) bool {
	return workspaceID != "" && conn.WorkspaceID != "" && conn.WorkspaceID != workspaceID
}

func (h *ConversationHub) entryUpdateReach(update *entryUpdate, allowEntry func(*WSConnection) bool, conn *WSConnection) conversation.EntryReach {
	reach := conversation.ReachOf(update.entry.IsGroup, conn.groupScope(nil), conn.hidesGroups.Load())
	switch {
	case reach == conversation.EntryListed && h.entryUpdateReaches(update, allowEntry, conn):
		return conversation.EntryListed
	case reach == conversation.EntryListed && h.entryUpdateVisible(update, allowEntry, conn, false):
		return conversation.EntryLeftView
	case reach == conversation.EntryElsewhere && h.entryUpdateVisible(update, allowEntry, conn, false):
		return conversation.EntryElsewhere
	}
	return conversation.EntryUnseen
}

func (h *ConversationHub) entryUpdateReaches(update *entryUpdate, allowEntry func(*WSConnection) bool, conn *WSConnection) bool {
	return h.entryUpdateVisible(update, allowEntry, conn, true)
}

func (h *ConversationHub) entryUpdateVisible(update *entryUpdate, allowEntry func(*WSConnection) bool, conn *WSConnection, inStatusTab bool) bool {
	if connInOtherWorkspace(conn, update.workspaceID) {
		return false
	}
	if update.campaignID != "" && conn.CampaignID != "" && conn.CampaignID != update.campaignID {
		return false
	}
	if conn.CampaignType != "" && conn.CampaignType != update.entryType {
		return false
	}
	if inStatusTab && !conn.inboxView().Includes(update.entry.ConversationStatus) {
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

func (h *ConversationHub) deliverEntryUpdate(conn *WSConnection, update *entryUpdate, reach conversation.EntryReach, alert bool) {
	var data []byte
	switch {
	case reach == conversation.EntryListed && alert && update.alertData != nil:
		data = update.alertData
	case reach == conversation.EntryListed:
		data = update.data
	case reach == conversation.EntryElsewhere && alert && update.unlistedData != nil:
		data = update.unlistedData
	case reach == conversation.EntryLeftView:
		data = update.leftViewData
	default:
		return
	}
	select {
	case conn.Send <- data:
	default:
		log.Printf("[ConversationHub] Send buffer full for user %s (connection %s)", conn.UserID, conn.ID)
	}
}
