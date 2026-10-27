package whatsapp_campaign_entry

import (
	"slices"

	"vozko/domain/conversation"
	"vozko/domain/whatsapp/template"
)

func OneOffDispatchedAttempts() []template.SendAttemptStatus {
	return []template.SendAttemptStatus{
		template.SendAttemptCharged, template.SendAttemptSent, template.SendAttemptUnknown,
		template.SendAttemptRejected, template.SendAttemptRefunded,
	}
}

func OneOffFailedAttempts() []template.SendAttemptStatus {
	return []template.SendAttemptStatus{template.SendAttemptRejected, template.SendAttemptRefunded}
}

func OneOffReceiptStatuses() map[conversation.DeliveryStatus]SendStatus {
	return map[conversation.DeliveryStatus]SendStatus{
		conversation.DeliveryStatusRead:      SendStatusRead,
		conversation.DeliveryStatusDelivered: SendStatusDelivered,
		conversation.DeliveryStatusFailed:    SendStatusFailed,
	}
}

func OneOffStatus(attempt template.SendAttemptStatus, receipt conversation.DeliveryStatus) (SendStatus, bool) {
	if !slices.Contains(OneOffDispatchedAttempts(), attempt) {
		return "", false
	}
	if slices.Contains(OneOffFailedAttempts(), attempt) {
		return SendStatusFailed, true
	}
	if status, known := OneOffReceiptStatuses()[receipt]; known {
		return status, true
	}
	return SendStatusSent, true
}
