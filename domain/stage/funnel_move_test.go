package stage

import (
	"errors"
	"testing"
)

func TestCrossesFunnelOnlyWhenBothSidesNameADifferentFunnel(t *testing.T) {
	for name, tc := range map[string]struct {
		from, to string
		want     bool
	}{
		"another funnel":          {"pl-atendimento", "pl-altiva", true},
		"the same funnel":         {"pl-altiva", "pl-altiva", false},
		"no stage yet":            {"", "pl-altiva", false},
		"a legacy campaign stage": {"pl-atendimento", "", false},
		"padded ids":              {" pl-altiva ", "pl-altiva", false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := CrossesFunnel(tc.from, tc.to); got != tc.want {
				t.Fatalf("CrossesFunnel(%q, %q) = %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestAMoveReportsAFunnelChangeFromItsTwoSides(t *testing.T) {
	jumped := Move{From: FunnelStage{StageID: "s-a", PipelineID: "pl-a"}, To: FunnelStage{StageID: "s-b", PipelineID: "pl-b"}}
	if !jumped.FunnelChanged() {
		t.Fatal("a move between two funnels changed funnel")
	}
	stayed := Move{From: FunnelStage{StageID: "s-a", PipelineID: "pl-a"}, To: FunnelStage{StageID: "s-c", PipelineID: "pl-a"}}
	if stayed.FunnelChanged() {
		t.Fatal("a move inside one funnel did not change funnel")
	}
	entered := Move{To: FunnelStage{StageID: "s-b", PipelineID: "pl-b"}}
	if entered.FunnelChanged() {
		t.Fatal("a conversation entering its first funnel did not leave another one")
	}
}

func TestMoveRefusalCodesNameEveryKnownRefusal(t *testing.T) {
	for err, want := range map[error]string{
		ErrStageNotConversationFunnel: "stage_not_conversation_funnel",
		ErrUnauthorized:               "stage_not_found",
		ErrTagNotFound:                "stage_not_found",
		ErrStagePipelineMismatch:      "stage_other_funnel",
		ErrEntryAccess:                "stage_entry_access",
		ErrInvalidEntryType:           "stage_invalid_entry",
		errors.New("boom"):            "stage_move_failed",
	} {
		if got := MoveRefusalCode(err); got != want {
			t.Errorf("MoveRefusalCode(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestEveryMoveRefusalCodeIsListed(t *testing.T) {
	listed := map[string]bool{}
	for _, code := range MoveRefusalCodes() {
		listed[code] = true
	}
	for _, err := range []error{ErrStageNotConversationFunnel, ErrUnauthorized, ErrStagePipelineMismatch, ErrEntryAccess, ErrInvalidEntryType, errors.New("boom")} {
		if !listed[MoveRefusalCode(err)] {
			t.Errorf("%q is produced but not listed", MoveRefusalCode(err))
		}
	}
}
