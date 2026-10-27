package unofficial_whatsapp_repository

import (
	"testing"

	conversation_repository "vozko/infra/repositories/conversation"
)

func TestTheUnofficialStatusCountsKnowWhichConversationsAreGroups(t *testing.T) {
	repo := NewConversationRepository(nil).(*conversationRepository)
	if repo.GroupColumn != conversation_repository.UnofficialGroupColumn {
		t.Fatalf("group column = %q, want %q so groups leave the client tab counts", repo.GroupColumn, conversation_repository.UnofficialGroupColumn)
	}
}
