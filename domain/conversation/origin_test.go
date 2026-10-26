package conversation

import (
	"errors"
	"slices"
	"testing"
)

func TestOriginOfTheFirstSender(t *testing.T) {
	cases := map[SenderKind]Origin{
		SenderContact:     OriginReceptive,
		SenderCampaign:    OriginCampaign,
		SenderHuman:       OriginOutreach,
		SenderAI:          OriginOutreach,
		SenderWorkflow:    OriginOutreach,
		SenderExternal:    OriginOutreach,
		SenderKind(""):    OriginUnknown,
		"unknown":         OriginUnknown,
		"Contact":         OriginUnknown,
		SenderKind("bot"): OriginUnknown,
	}
	for kind, want := range cases {
		if got := OriginOf(kind); got != want {
			t.Fatalf("OriginOf(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestEveryKnownOriginListsTheSendersThatStartIt(t *testing.T) {
	for _, origin := range Origins() {
		for _, kind := range OriginSenders(origin) {
			if got := OriginOf(kind); got != origin {
				t.Fatalf("%q is listed under %q but OriginOf says %q", kind, origin, got)
			}
		}
	}
	if senders := OriginSenders(OriginUnknown); len(senders) != 0 {
		t.Fatalf("unknown must be what no sender gives, got %v", senders)
	}
	if got := OriginSenders(OriginOutreach); !slices.Equal(got, ReplySenderKinds()) {
		t.Fatalf("the team and its automations start outreach: got %v, want %v", got, ReplySenderKinds())
	}
}

func TestTheSystemNeverOpensAConversation(t *testing.T) {
	if SenderSystem.OpensConversation() {
		t.Fatal("a system notice or an import seed must never decide who started a conversation")
	}
	for _, kind := range []SenderKind{SenderContact, SenderHuman, SenderAI, SenderWorkflow, SenderCampaign, SenderExternal, "unknown"} {
		if !kind.OpensConversation() {
			t.Fatalf("%q must count as the opening message, so an unknown start stays unknown", kind)
		}
	}
	for _, origin := range Origins() {
		if slices.Contains(OriginSenders(origin), SenderSystem) {
			t.Fatalf("%q lists the system as a sender", origin)
		}
	}
}

func TestParseOriginAcceptsOnlyTheVocabulary(t *testing.T) {
	for _, origin := range Origins() {
		got, err := ParseOrigin(string(origin))
		if err != nil || got != origin {
			t.Fatalf("ParseOrigin(%q) = %q, %v", origin, got, err)
		}
	}
	for _, raw := range []string{"", "Receptive", " receptive", "channel", "all"} {
		if _, err := ParseOrigin(raw); !errors.Is(err, ErrOriginInvalid) {
			t.Fatalf("ParseOrigin(%q) err = %v, want ErrOriginInvalid", raw, err)
		}
	}
}

func TestTheAssistantDigestTellsWhoOpenedTheConversation(t *testing.T) {
	if got := DigestInboxEntry(InboxEntry{EntryID: "e1", Origin: OriginReceptive}).Origin; got != OriginReceptive {
		t.Fatalf("digest origin = %q, want receptive", got)
	}
}
