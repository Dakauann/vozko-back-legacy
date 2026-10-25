package conversation_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/shared"
)

func TestAConversationOfAnUnregisteredChannelHasNoOwnerAnswer(t *testing.T) {
	resolver := NewCampaignWorkspaceResolver(nil, nil, stubRoutes{})
	if _, err := resolver.GetEntryCampaignID("entry-1", string(shared.EntryTypeTelegram)); !errors.Is(err, ErrChannelNotRegistered) {
		t.Fatalf("campaign err = %v, want ErrChannelNotRegistered", err)
	}
	if _, err := resolver.GetEntryWorkspaceID("entry-1", string(shared.EntryTypeTelegram)); !errors.Is(err, ErrChannelNotRegistered) {
		t.Fatalf("workspace err = %v, want ErrChannelNotRegistered", err)
	}
}

func TestAnUnreadableAutomationFlagReadsAsOff(t *testing.T) {
	provider := historyWith(HistoryProviderService{})
	if provider.automationFor("entry-1", shared.EntryTypeTelegram) {
		t.Fatal("a channel with no automation reader must read as off")
	}
	provider.SetAutomationReader(shared.EntryTypeTelegram, func(context.Context, string) (*bool, error) {
		return nil, errors.New("db down")
	})
	if provider.automationFor("entry-1", shared.EntryTypeTelegram) {
		t.Fatal("an unreadable automation flag must read as off")
	}
}
