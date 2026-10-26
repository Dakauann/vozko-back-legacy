package crmtools

import (
	"context"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/copilot"
	"vozko/usecases/copilot/copilottools/tooltest"
)

func TestSearchConversationsFiltersByOrigin(t *testing.T) {
	inbox := &fakeInbox{}
	res := NewSearchConversationsTool(conversationDeps(inbox, &fakeHistory{})).Execute(context.Background(), tooltest.Member(), map[string]interface{}{
		"origin": "receptive",
	})
	if res.Status != copilot.StatusOK || inbox.input.Origin != conversation.OriginReceptive {
		t.Fatalf("status %s, origin %q", res.Status, inbox.input.Origin)
	}
}

func TestSearchConversationsRefusesAnUnknownOrigin(t *testing.T) {
	inbox := &fakeInbox{}
	res := NewSearchConversationsTool(conversationDeps(inbox, &fakeHistory{})).Execute(context.Background(), tooltest.Member(), map[string]interface{}{
		"origin": "inbound",
	})
	if res.Status != copilot.StatusError || inbox.calls != 0 {
		t.Fatalf("status %s after %d searches", res.Status, inbox.calls)
	}
}
