package ws

import (
	"context"
	"encoding/json"
	"log"

	"vozko/domain/personalinbox"
)

const inboxPreferencesBroadcastType = "inbox_preferences"

func inboxPreferencesEvent(prefs personalinbox.Preferences) ([]byte, error) {
	return json.Marshal(&WSOutgoingMessage{
		Type:    WSEventInboxPreferences,
		Payload: InboxPreferencesPayload{HideGroups: prefs.HideGroups},
	})
}

func (h *ConversationHub) loadInboxPreferences(conn *WSConnection, view inboxView) inboxView {
	prefs := personalinbox.DefaultPreferences()
	if commands := h.cmds(); commands != nil {
		prefs = commands.PersonalMarks.InboxPreferences(context.Background(), conn.WorkspaceID, conn.UserID)
	}
	conn.hidesGroups.Store(prefs.HideGroups)
	view.hidesGroups = prefs.HideGroups
	data, err := inboxPreferencesEvent(prefs)
	if err != nil {
		log.Printf("[ConversationHub] inbox preferences of %s: %v", conn.UserID, err)
		return view
	}
	h.sendRawToConnection(conn, data)
	return view
}

func (h *ConversationHub) AnnounceInboxPreferences(workspaceID, userID string, prefs personalinbox.Preferences) {
	if h == nil || workspaceID == "" || userID == "" {
		return
	}
	data, err := inboxPreferencesEvent(prefs)
	if err != nil {
		log.Printf("[ConversationHub] inbox preferences of %s: %v", userID, err)
		return
	}
	recipients := []string{userID}
	h.applyInboxPreferences(workspaceID, recipients, prefs, data)
	h.publishUserPayload(inboxPreferencesBroadcastType, workspaceID, recipients, data)
}

func (h *ConversationHub) applyRelayedInboxPreferences(workspaceID string, userIDs []string, data []byte) {
	var event struct {
		Payload InboxPreferencesPayload `json:"payload"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("[ConversationHub] relayed inbox preferences: %v", err)
		return
	}
	h.applyInboxPreferences(workspaceID, userIDs, personalinbox.Preferences{HideGroups: event.Payload.HideGroups}, data)
}

func (h *ConversationHub) applyInboxPreferences(workspaceID string, userIDs []string, prefs personalinbox.Preferences, data []byte) {
	recipients := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		recipients[id] = true
	}
	h.sendToWorkspaceWhere(workspaceID, data, func(_ string, conn *WSConnection) bool {
		if !recipients[conn.UserID] {
			return false
		}
		conn.hidesGroups.Store(prefs.HideGroups)
		return true
	})
}
