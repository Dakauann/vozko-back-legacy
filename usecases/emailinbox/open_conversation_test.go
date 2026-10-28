package emailinbox

import (
	"context"
	"testing"
)

func TestOpeningAConversationLinksTheLeadAndSendsNothing(t *testing.T) {
	w := newWorld(t)
	w.addSupportSender()
	ctx := context.Background()
	in := OpenConversationInput{WorkspaceID: wsA, SenderID: senderSupport, Address: anaAddress, LeadID: leadAna, LeadName: "Ana Souza"}
	id, err := w.transcript().OpenConversation(ctx, in)
	if err != nil || id == "" {
		t.Fatalf("open = %q, %v", id, err)
	}
	again, err := w.transcript().OpenConversation(ctx, in)
	if err != nil || again != id {
		t.Fatalf("opening again = %q, %v, want %q", again, err, id)
	}
	if len(w.history.records) != 0 {
		t.Fatalf("opening wrote %d messages", len(w.history.records))
	}
	conv, err := w.convs.FindByID(ctx, id)
	if err != nil || conv.SenderID != senderSupport {
		t.Fatalf("conversation = %+v, %v", conv, err)
	}
	contact, err := w.contacts.FindByAddress(ctx, wsA, anaAddress)
	if err != nil || contact.LeadID == nil || *contact.LeadID != leadAna {
		t.Fatalf("contact = %+v, %v", contact, err)
	}
}

func TestOpeningAConversationOnAnUnknownInboxFails(t *testing.T) {
	w := newWorld(t)
	_, err := w.transcript().OpenConversation(context.Background(), OpenConversationInput{WorkspaceID: wsA, SenderID: "5e000000-0000-0000-0000-0000000000ff", Address: anaAddress, LeadID: leadAna})
	if err == nil {
		t.Fatal("an inbox of no sender must refuse")
	}
	if _, err := w.transcript().OpenConversation(context.Background(), OpenConversationInput{WorkspaceID: wsA, SenderID: senderSupport, Address: "not an address", LeadID: leadAna}); err == nil {
		t.Fatal("an invalid address must refuse")
	}
}
