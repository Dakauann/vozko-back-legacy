package ws

import (
	"encoding/json"
	"log"
	"time"

	"vozko/domain/conversation"
)

type entryChange int

const (
	entryChanged entryChange = iota
	entryRefreshed
	customerWrote
)

func changeFor(message *conversation.Message) entryChange {
	if message.FromCustomer() {
		return customerWrote
	}
	return entryChanged
}

func relayedChange(customerMessage bool) entryChange {
	if customerMessage {
		return customerWrote
	}
	return entryChanged
}

const assignedToYouBroadcastType = "assigned_to_you"

func (h *ConversationHub) messageAttention(update *entryUpdate, change entryChange) conversation.MessageAttention {
	if change != customerWrote {
		return conversation.MessageAttention{}
	}
	owner := update.entry.AssignedUserID
	commands := h.cmds()
	if commands == nil {
		log.Printf("[ConversationHub] attention for entry %s (%s) alerts only its owner: %v", update.entryID, update.entryType, ErrHubCommandsUnbound)
		return conversation.MessageAttention{Owner: owner}
	}
	return commands.Attention.MessageAttention(update.workspaceID, update.entryID, update.entryType, owner)
}

func (h *ConversationHub) publishEntryChange(entryID, entryType string, change entryChange) {
	if h.sharedState == nil {
		return
	}
	data, err := json.Marshal(redisWorkspaceBroadcast{
		Type:            "entry_update",
		EntryID:         entryID,
		EntryType:       entryType,
		CustomerMessage: change == customerWrote,
		ReplicaID:       h.replicaID,
	})
	if err != nil {
		return
	}
	_ = h.sharedState.Publish("hub:workspace_broadcast", data)
}

func (h *ConversationHub) AlertAssignee(workspaceID, entryID, entryType, userID string) {
	if h == nil || workspaceID == "" || userID == "" {
		return
	}
	data, err := json.Marshal(&WSOutgoingMessage{
		Type:    WSEventAssignedToYou,
		Payload: AssignedToYouPayload{Entry: h.assignedEntry(entryID, entryType), AssignedAt: time.Now().UTC()},
	})
	if err != nil {
		log.Printf("[ConversationHub] assignment alert for entry %s (%s): %v", entryID, entryType, err)
		return
	}
	recipients := []string{userID}
	h.sendToUsers(workspaceID, recipients, data)
	h.publishUserPayload(assignedToYouBroadcastType, workspaceID, recipients, data)
}

func (h *ConversationHub) assignedEntry(entryID, entryType string) conversation.InboxEntry {
	entry, ok := h.inboxEntry(entryID, entryType)
	if !ok {
		return conversation.InboxEntry{EntryID: entryID, EntryType: entryType}
	}
	return *entry
}
