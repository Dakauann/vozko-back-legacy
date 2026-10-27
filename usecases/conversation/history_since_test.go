package conversation_usecase

import (
	"testing"
	"time"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

type sinceMessageRepo struct {
	conversation.MessageRepository
	oldestFirst []*conversation.Message
	asked       conversation.ListMessagesInput
}

func (r *sinceMessageRepo) ListByEntryPaginated(in conversation.ListMessagesInput) ([]*conversation.Message, error) {
	r.asked = in
	return r.oldestFirst, nil
}

type ownedMedia struct {
	conversation.ConversationMediaRepository
	rows []*conversation.ConversationMedia
}

func (m ownedMedia) ListByIDs([]string) ([]*conversation.ConversationMedia, error) { return m.rows, nil }

func TestMessagesSinceReadsForwardFromTheCursorWithTheConversationsOwnMediaOnly(t *testing.T) {
	start := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	own, foreign := "media-own", "media-foreign"
	messages := timeline(start, 3)
	messages[1].MediaID, messages[2].MediaID = &own, &foreign
	repo := &sinceMessageRepo{oldestFirst: messages}
	media := ownedMedia{rows: []*conversation.ConversationMedia{
		{ID: own, EntryID: "e1", EntryType: shared.EntryTypeWhatsApp, URL: "https://cdn.example.com/receipt.jpg"},
		{ID: foreign, EntryID: "another", EntryType: shared.EntryTypeWhatsApp, URL: "https://cdn.example.com/private.jpg"},
	}}
	s := historyWith(HistoryProviderService{messageRepo: repo, mediaRepo: media})
	since := conversation.MessageKeyset{At: start.Add(-time.Minute), ID: "z"}

	got, err := s.MessagesSince("e1", shared.EntryTypeWhatsApp, since, 50)
	if err != nil {
		t.Fatal(err)
	}
	if repo.asked.Since == nil || *repo.asked.Since != since || repo.asked.Limit != 50 || repo.asked.After != nil || repo.asked.Before != nil {
		t.Fatalf("asked = %+v", repo.asked)
	}
	if idsOf(got) != "abc" {
		t.Fatalf("order = %q, want oldest first", idsOf(got))
	}
	if got[1].Media == nil || got[1].Media.URL != "https://cdn.example.com/receipt.jpg" {
		t.Fatalf("the conversation's own media is missing: %+v", got[1].Media)
	}
	if got[2].Media != nil {
		t.Fatalf("media of another conversation leaked: %+v", got[2].Media)
	}
}
