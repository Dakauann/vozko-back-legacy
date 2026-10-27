package workflow

import (
	"strings"
	"time"

	"vozko/domain/conversation"
)

const (
	DataKeyChannel            = "channel"
	DataKeyCampaignID         = "campaign_id"
	DataKeyCampaignWorkflowID = "campaign_workflow_id"
	DataKeyAccountWorkflowID  = "account_workflow_id"
)

const (
	InboundKindText  = "text"
	InboundKindMedia = "media"
	InboundKindAudio = "audio"
)

type InboundTrigger struct {
	WorkspaceID      string
	EntryID          string
	EntryType        string
	Text             string
	SentAt           time.Time
	Media            conversation.MediaType
	Selection        *OptionSelection
	ContactNumber    string
	TriggerMessageID string
	CampaignID       string
	WorkflowID       string
}

func (t *InboundTrigger) StoredAs(stored *conversation.Message) {
	if stored == nil {
		return
	}
	t.TriggerMessageID = stored.ID
	t.Media = stored.MediaType
}

func InboundKindOf(media conversation.MediaType) string {
	switch media {
	case "":
		return InboundKindText
	case conversation.MediaTypeAudio:
		return InboundKindAudio
	}
	return InboundKindMedia
}

func (t InboundTrigger) Event() TriggerEvent {
	data := InboundMessageData(t.Text, InboundKindOf(t.Media), string(t.Media), t.Selection)
	data[DataKeyChannel] = t.EntryType
	data[DataKeyWorkspaceID] = t.WorkspaceID
	ApplyContactNumber(data, t.ContactNumber)
	ApplyTriggerMessage(data, t.TriggerMessageID)
	if campaignID := strings.TrimSpace(t.CampaignID); campaignID != "" {
		data[DataKeyCampaignID] = campaignID
	}
	if workflowID := strings.TrimSpace(t.WorkflowID); workflowID != "" {
		if _, campaign := data[DataKeyCampaignID]; campaign {
			data[DataKeyCampaignWorkflowID] = workflowID
		} else {
			data[DataKeyAccountWorkflowID] = workflowID
		}
	}
	return TriggerEvent{
		WorkspaceID: t.WorkspaceID,
		EntryID:     t.EntryID,
		EntryType:   t.EntryType,
		TriggerType: TriggerMessageReceived,
		SentAt:      t.SentAt,
		Data:        data,
	}
}

func (e TriggerEvent) AsFirstMessage() TriggerEvent {
	first := e
	first.TriggerType = TriggerFirstMessage
	first.Reply = nil
	return first
}

func (e TriggerEvent) NamedResponsibleWorkflow() (string, bool) {
	for _, key := range []string{DataKeyCampaignWorkflowID, DataKeyAccountWorkflowID} {
		if id, _ := e.Data[key].(string); strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id), true
		}
	}
	return "", false
}
