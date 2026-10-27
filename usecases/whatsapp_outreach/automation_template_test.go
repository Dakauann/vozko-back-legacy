package whatsapp_outreach

import (
	"context"
	"testing"

	"vozko/domain/conversation"
)

func TestAnAutomationTemplateAppearsAsTheAutomationAndClaimsNobody(t *testing.T) {
	f := newConversationUC(t)
	in := conversationInput()
	in.UserID = ""
	in.Automation = "ai:agent-1"

	if _, err := f.uc.Send(context.Background(), in); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(f.history.records) != 1 {
		t.Fatalf("records = %d", len(f.history.records))
	}
	record := f.history.records[0]
	if record.SentBy.Kind() != conversation.SenderAI || record.SentBy.Participant() != "agent-1" {
		t.Fatalf("sent by = %+v, want the agent that scheduled it", record.SentBy)
	}
	if record.From != "ai:agent-1" {
		t.Fatalf("from = %q", record.From)
	}
	if len(f.claims.calls) != 0 {
		t.Fatalf("an automation send must not hand the conversation to anyone: %+v", f.claims.calls)
	}
}
