package automationneeds

import (
	"errors"
	"strings"
	"testing"

	"vozko/domain/shared"
	"vozko/domain/workflow"
)

func TestEverySlotKindRunsOnAChannelWithDeclaredFacts(t *testing.T) {
	for _, kind := range SlotKinds() {
		if !workflow.IsChannel(string(kind.EntryType())) {
			t.Errorf("%s runs on %q", kind, kind.EntryType())
		}
	}
	if SlotWhatsAppNumber.EntryType() != shared.EntryTypeWhatsApp || SlotUnofficial.EntryType() != shared.EntryTypeUnofficialWhatsApp {
		t.Fatal("the WhatsApp slots run on their entry types")
	}
}

func TestAChannelRefusalNamesTheStepAndTheChannelInPlainWords(t *testing.T) {
	walk, err := ChannelWalkOf(channelWorld(), "ws", workflowRef("direto"), instagram())
	if err != nil {
		t.Fatal(err)
	}
	refused := ChannelRefusal(workflowRef("direto"), walk)
	if !errors.Is(refused, ErrChannelUnsupported) || ErrorCode(refused) != "automation_channel_unsupported" {
		t.Fatalf("err = %v", refused)
	}
	refusal, ok := RefusalOf(refused)
	if !ok || len(refusal.Channels) != 1 || refusal.Automation.Name != "Direto" {
		t.Fatalf("refusal = %+v", refusal)
	}
	sentence := refusal.Sentence()
	for _, want := range []string{"Enviar boleto", "Instagram", "templates do WhatsApp oficial"} {
		if !strings.Contains(sentence, want) {
			t.Fatalf("sentence %q misses %q", sentence, want)
		}
	}
	if strings.Contains(sentence, "{{") {
		t.Fatalf("sentence = %q", sentence)
	}
}

func TestAWalkWithoutBlocksIsNoRefusal(t *testing.T) {
	walk, err := ChannelWalkOf(channelWorld(), "ws", workflowRef("direto"), []shared.EntryType{shared.EntryTypeWhatsApp})
	if err != nil {
		t.Fatal(err)
	}
	if refused := ChannelRefusal(workflowRef("direto"), walk); refused != nil {
		t.Fatalf("err = %v", refused)
	}
}
