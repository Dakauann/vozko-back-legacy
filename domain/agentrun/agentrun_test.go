package agentrun

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestAFailedCallIsFollowedByWhatTheModelDidInItsNextCall(t *testing.T) {
	steps := []Step{
		{Name: "consultar_pedido", Failed: true, ModelCall: 0},
		{Name: "buscar_cliente", Failed: false, ModelCall: 0},
		{Name: "consultar_pedido", Failed: true, ModelCall: 1},
		{Name: "transferir", Failed: false, ModelCall: 2},
		{Name: "agendar", Failed: true, ModelCall: 3},
	}
	got := FollowUps(steps, true)
	want := []FollowedBy{FollowedRetry, "", FollowedOtherTool, "", FollowedAnswer}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if last := FollowUps(steps[4:], false); last[0] != FollowedNothing {
		t.Fatalf("got %v", last)
	}
}

func TestOnlyARetryOrAnotherToolIsAHandledFailure(t *testing.T) {
	for f, handled := range map[FollowedBy]bool{FollowedRetry: true, FollowedOtherTool: true, FollowedAnswer: false, FollowedNothing: false, "": false} {
		if f.Handled() != handled {
			t.Fatalf("%q handled %v", f, f.Handled())
		}
	}
}

func TestSandboxedSourcesNeverCountAsLiveTraffic(t *testing.T) {
	if !SourceSimulator.Sandboxed() || !SourceScenario.Sandboxed() || SourceConversation.Sandboxed() || SourceWorkflow.Sandboxed() {
		t.Fatal("sandbox rule")
	}
	if Source("other").Known() || !SourceWorkflow.Known() {
		t.Fatal("known rule")
	}
}

func TestTheStatusSaysWhetherTheModelOrAToolFailed(t *testing.T) {
	if StatusOf(false, 0) != StatusOK || StatusOf(false, 2) != StatusToolFailed || StatusOf(true, 0) != StatusFailed {
		t.Fatal("status rule")
	}
}

func TestAFilterWithAnUnknownSourceStatusOrAnInvertedRangeIsRefused(t *testing.T) {
	now := time.Now()
	bad := []Filter{{Source: "x"}, {Status: "x"}, {From: now, To: now.Add(-time.Hour)}, {EntryID: "not-a-uuid"}}
	for _, f := range bad {
		if !errors.Is(f.Check(), ErrFilterInvalid) {
			t.Fatalf("%+v accepted", f)
		}
	}
	if (Filter{Source: SourceConversation, Status: StatusToolFailed, From: now.Add(-time.Hour), To: now, EntryID: "6f1c2a52-0b5e-4bb4-9d0f-7d1a3a9c2b10"}).Check() != nil {
		t.Fatal("a valid filter was refused")
	}
}

func TestOnlyARunOnAConversationEntryIsBoundByTheConversationRule(t *testing.T) {
	if !(Run{EntryID: "e", EntryType: "whatsapp"}).InConversation() || (Run{}).InConversation() || (Run{EntryID: "e", EntryType: "sip_call"}).InConversation() || (Run{EntryID: "e", EntryType: "lead"}).InConversation() {
		t.Fatal("conversation rule")
	}
}
