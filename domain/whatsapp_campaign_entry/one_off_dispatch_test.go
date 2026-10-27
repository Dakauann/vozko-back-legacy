package whatsapp_campaign_entry

import (
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/whatsapp/template"
)

func TestOneOffStatusFollowsTheAttemptThenTheReceipt(t *testing.T) {
	cases := []struct {
		name    string
		attempt template.SendAttemptStatus
		receipt conversation.DeliveryStatus
		want    SendStatus
		counted bool
	}{
		{"an attempt never charged was never sent", template.SendAttemptPending, conversation.DeliveryStatusNone, "", false},
		{"a charged attempt is on its way", template.SendAttemptCharged, conversation.DeliveryStatusNone, SendStatusSent, true},
		{"an accepted send without a receipt is in transit", template.SendAttemptSent, conversation.DeliveryStatusSent, SendStatusSent, true},
		{"a delivered receipt makes it delivered", template.SendAttemptSent, conversation.DeliveryStatusDelivered, SendStatusDelivered, true},
		{"a read receipt makes it read", template.SendAttemptSent, conversation.DeliveryStatusRead, SendStatusRead, true},
		{"a failed receipt makes it failed", template.SendAttemptSent, conversation.DeliveryStatusFailed, SendStatusFailed, true},
		{"an unknown outcome stays in transit until a receipt says more", template.SendAttemptUnknown, conversation.DeliveryStatusNone, SendStatusSent, true},
		{"a refused send is a failure", template.SendAttemptRejected, conversation.DeliveryStatusNone, SendStatusFailed, true},
		{"a refunded send is a failure even if a receipt came first", template.SendAttemptRefunded, conversation.DeliveryStatusDelivered, SendStatusFailed, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, counted := OneOffStatus(tc.attempt, tc.receipt)
			if counted != tc.counted || got != tc.want {
				t.Fatalf("OneOffStatus(%s, %q) = %q, %v; want %q, %v", tc.attempt, tc.receipt, got, counted, tc.want, tc.counted)
			}
		})
	}
}

func TestOneOffSetsCoverEveryAttemptStatus(t *testing.T) {
	counted := map[template.SendAttemptStatus]bool{}
	for _, s := range OneOffDispatchedAttempts() {
		counted[s] = true
	}
	for _, s := range []template.SendAttemptStatus{
		template.SendAttemptCharged, template.SendAttemptSent, template.SendAttemptUnknown,
		template.SendAttemptRejected, template.SendAttemptRefunded,
	} {
		if !counted[s] {
			t.Errorf("attempt status %s reached the provider or the ledger and must count", s)
		}
	}
	if counted[template.SendAttemptPending] {
		t.Error("a pending attempt was never charged nor sent")
	}
	for _, s := range OneOffFailedAttempts() {
		if !counted[s] {
			t.Errorf("failed attempt status %s must also be a counted one", s)
		}
	}
}
