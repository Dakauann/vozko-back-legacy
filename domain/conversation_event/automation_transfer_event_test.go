package conversation_event

import "testing"

func TestAnAutomationTransferIsAKnownConversationEvent(t *testing.T) {
	if string(EventAutomationTransferred) != "automation_transferred" || !EventAutomationTransferred.Valid() {
		t.Fatalf("%q valid = %v", EventAutomationTransferred, EventAutomationTransferred.Valid())
	}
}
