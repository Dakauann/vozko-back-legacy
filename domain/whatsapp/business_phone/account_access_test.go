package businessphone

import (
	"testing"
	"time"
)

func TestANumberNeedsReconnectingOnlyWhileItsAccountIsUnreachable(t *testing.T) {
	lost := time.Date(2026, 10, 29, 18, 0, 0, 0, time.UTC)
	if !(&WhatsAppBusinessPhoneNumber{AccountAccessLostAt: &lost}).NeedsReconnect() {
		t.Fatal("a number whose account we lost must ask for a reconnection")
	}
	if (&WhatsAppBusinessPhoneNumber{}).NeedsReconnect() {
		t.Fatal("a reachable number must not ask for a reconnection")
	}
}
