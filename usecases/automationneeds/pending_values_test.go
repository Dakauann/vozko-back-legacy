package automationneeds

import (
	"context"
	"errors"
	"testing"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/shared"
)

func TestValuesAboutToBeStoredCountAsTheConversations(t *testing.T) {
	s := newService(t, fakeValues{values: map[string]string{"lead_name": "Ana"}})
	target := ref(conversation.AutomationAgent, "escola")

	if err := s.ConversationMissingWith(context.Background(), "ws", "e1", shared.EntryTypeInstagram, target, map[string]string{"turma": "Noite"}); err != nil {
		t.Fatalf("pending values meet the need: %v", err)
	}
	if err := s.ConversationMissingWith(context.Background(), "ws", "e1", shared.EntryTypeInstagram, target, map[string]string{"turma": "  "}); !errors.Is(err, domain.ErrConversationMissing) {
		t.Fatalf("a blank pending value meets nothing: %v", err)
	}
	if err := s.ConversationMissingWith(context.Background(), "ws", "e1", shared.EntryTypeInstagram, target, nil); !errors.Is(err, domain.ErrConversationMissing) {
		t.Fatalf("without pending values the stored ones decide: %v", err)
	}
}
