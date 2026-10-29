package tutorial

import (
	"errors"
	"testing"
	"time"
)

var at = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func TestTutorialIDsAreAreaDotName(t *testing.T) {
	for _, id := range []string{"sites.editor", "agents.create", "whatsappCampaigns.detail", "studio.image.export"} {
		if !ValidID(id) {
			t.Errorf("%q should be valid", id)
		}
	}
	for _, id := range []string{"", "sites", "Sites.editor", "sites.", ".editor", "sites editor", "sites.editor.a.b", "sites/editor", "sites.ed-itor"} {
		if ValidID(id) {
			t.Errorf("%q should be invalid", id)
		}
	}
}

func TestNewProgressRefusesWhatCannotBeStored(t *testing.T) {
	if _, err := NewProgress("bad id", StatusCompleted, 0, at); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("bad id = %v, want ErrInvalidID", err)
	}
	if _, err := NewProgress("sites.editor", "skipped", 0, at); !errors.Is(err, ErrInvalidProgress) {
		t.Fatalf("unknown status = %v, want ErrInvalidProgress", err)
	}
	for _, step := range []int{-1, MaxStep + 1} {
		if _, err := NewProgress("sites.editor", StatusInProgress, step, at); !errors.Is(err, ErrInvalidProgress) {
			t.Fatalf("step %d = %v, want ErrInvalidProgress", step, err)
		}
	}
	got, err := NewProgress("sites.editor", StatusDismissed, 3, at)
	if err != nil || got != (Progress{TutorialID: "sites.editor", Status: StatusDismissed, Step: 3, UpdatedAt: at}) {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestCompletionStaysUntilTheTutorialIsReset(t *testing.T) {
	later := at.Add(time.Hour)
	done := Progress{TutorialID: "sites.editor", Status: StatusCompleted, Step: 7, UpdatedAt: at}
	for _, status := range []Status{StatusInProgress, StatusDismissed} {
		next := Progress{TutorialID: "sites.editor", Status: status, Step: 2, UpdatedAt: later}
		if got := next.After(done, true); got != (Progress{TutorialID: "sites.editor", Status: StatusCompleted, Step: 7, UpdatedAt: later}) {
			t.Errorf("%s after completed = %+v", status, got)
		}
	}
	started := Progress{TutorialID: "sites.editor", Status: StatusInProgress, Step: 1, UpdatedAt: later}
	dismissed := Progress{TutorialID: "sites.editor", Status: StatusDismissed, Step: 4, UpdatedAt: at}
	if got := started.After(dismissed, true); got != started {
		t.Errorf("starting a dismissed tour = %+v, want %+v", got, started)
	}
	if got := started.After(Progress{}, false); got != started {
		t.Errorf("first progress = %+v", got)
	}
}
