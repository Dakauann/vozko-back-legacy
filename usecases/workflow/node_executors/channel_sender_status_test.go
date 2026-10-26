package node_executors

import (
	"context"
	"testing"
	"time"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

func statusSender(adapter conversation.ChannelAdapter, history *recordingHistory) *channelSender {
	return &channelSender{
		adapters:   conversation.NewAdapterRegistry(adapter),
		entryLeads: &entryLeads{}, leads: &knownLeadRepo{},
		history: history,
	}
}

func TestWorkflowSendsOnAnAdapterChannelAreRecordedAsSent(t *testing.T) {
	history := &recordingHistory{}
	adapter := &interactiveFakeAdapter{fakeAdapter: fakeAdapter{entryType: shared.EntryTypeInstagram, windowOpen: true}, shown: []string{"sim"}}
	sender := statusSender(adapter, history)
	ctx := context.Background()

	if _, err := sender.SendText(ctx, igRun(), "olá", conversation.MessageTypeAIResponse); err != nil {
		t.Fatalf("text: %v", err)
	}
	if _, err := sender.SendMedia(ctx, igRun(), "https://cdn.exemplo.com/a.jpg", ""); err != nil {
		t.Fatalf("media: %v", err)
	}
	if _, err := sender.SendInteractive(ctx, igRun(), conversation.SendInteractiveRequest{
		Body: "Deseja continuar?", Options: []conversation.InteractiveOption{{ID: "sim", Title: "Sim"}},
	}); err != nil {
		t.Fatalf("interactive: %v", err)
	}
	if _, err := sender.SendSegments(ctx, igRun(), []string{"um"}, func(time.Duration) {}); err != nil {
		t.Fatalf("segments: %v", err)
	}

	if len(history.records) != 4 {
		t.Fatalf("records = %d, want 4", len(history.records))
	}
	for i, record := range history.records {
		if record.DeliveryStatus != conversation.DeliveryStatusSent {
			t.Fatalf("record %d delivery status = %q, want sent like an operator send", i, record.DeliveryStatus)
		}
	}
}
