package googlesheet

import (
	"testing"

	"vozko/domain/shared"
)

func TestAnOriginLinksToAConversationOnlyWhenItCameFromOne(t *testing.T) {
	cases := []struct {
		name   string
		origin Origin
		want   shared.EntryRef
		linked bool
	}{
		{"agent reply", Origin{Kind: OriginAgent, AgentID: "a", EntryID: "conv-1", EntryType: "whatsapp"}, shared.EntryRef{EntryID: "conv-1", EntryType: shared.EntryTypeWhatsApp}, true},
		{"workflow run on a conversation", Origin{Kind: OriginWorkflow, EntryID: " conv-2 ", EntryType: "instagram"}, shared.EntryRef{EntryID: "conv-2", EntryType: shared.EntryTypeInstagram}, true},
		{"workflow run on a lead", Origin{Kind: OriginWorkflow, EntryID: "lead-1", EntryType: "lead", LeadID: "lead-1"}, shared.EntryRef{}, false},
		{"manual add", Origin{Kind: OriginManual, ActorID: "user-1"}, shared.EntryRef{}, false},
		{"type without an id", Origin{Kind: OriginAgent, EntryType: "whatsapp"}, shared.EntryRef{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, linked := c.origin.Conversation()
			if linked != c.linked || got != c.want {
				t.Fatalf("Conversation() = %+v, %v; want %+v, %v", got, linked, c.want, c.linked)
			}
		})
	}
}
