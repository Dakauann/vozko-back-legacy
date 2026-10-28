package workflow

import (
	"strings"
	"testing"
)

func messageFlowToText() *Graph {
	return flowOf([]Node{messageTrigger("t"), textStep("s")}, Edge{Source: "t", Target: "s"}, Edge{Source: "s", Target: "end"})
}

func TestTheContactNumberExplainsItIsAPhoneOnlyOnPhoneChannels(t *testing.T) {
	number := variableByRef(catalogAt(t, messageFlowToText(), VariableSources{}, "s"), DataKeyContactNumber)
	if number == nil || number.Channels == nil {
		t.Fatalf("contact_number = %+v", number)
	}
	note := number.Channels
	if note.Key != string(ChannelFactPhoneHandle) || !strings.Contains(note.Text, "Instagram") || strings.Contains(note.Text, "{{") {
		t.Fatalf("note = %+v", note)
	}
	for _, channel := range note.Channels {
		if channel == "whatsapp" || channel == "sms" {
			t.Fatalf("a phone channel is listed as lacking the phone: %v", note.Channels)
		}
	}
	if len(note.Channels) == 0 {
		t.Fatal("the channels without a phone are listed")
	}
}

func TestTheLeadNumberExplainsItMayBeEmptyOffPhoneChannels(t *testing.T) {
	number := variableByRef(catalogAt(t, messageFlowToText(), VariableSources{}, "s"), ConversationRef("lead_number"))
	if number == nil || number.Channels == nil || number.Channels.Key != string(ChannelFactContactPhone) {
		t.Fatalf("lead_number = %+v", number)
	}
}

func TestAVariableNoChannelChangesHasNoNote(t *testing.T) {
	message := variableByRef(catalogAt(t, messageFlowToText(), VariableSources{}, "s"), DataKeyMessage)
	if message == nil || message.Channels != nil {
		t.Fatalf("message = %+v", message)
	}
}
