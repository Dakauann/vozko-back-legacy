package campaign

import (
	"errors"
	"testing"
)

func TestALeadSelectionSendIsNeverScheduled(t *testing.T) {
	if err := RefuseSelectionSchedule(SourceLeadSelection, true); !errors.Is(err, ErrSelectionScheduleRefused) {
		t.Fatalf("got %v", err)
	}
	if err := RefuseSelectionSchedule(SourceLeadSelection, false); err != nil {
		t.Fatalf("clearing a schedule is always allowed: %v", err)
	}
	if err := RefuseSelectionSchedule("", true); err != nil {
		t.Fatalf("a campaign from the wizard may be scheduled: %v", err)
	}
	if ErrorCode(ErrSelectionScheduleRefused) != "send_schedule_from_leads" {
		t.Fatal(ErrorCode(ErrSelectionScheduleRefused))
	}
}
