package scheduledstart

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"vozko/domain/campaign"
	"vozko/domain/campaign/launchcheck"
)

var risky = launchcheck.Report{Findings: []launchcheck.Finding{{Code: launchcheck.CodeNoExit, Level: launchcheck.Confirm}}}

func TestAFailedScheduledStartKeepsAStableReason(t *testing.T) {
	blocked := &launchcheck.RefusedError{Report: launchcheck.Blocked(launchcheck.CodeTemplateNotReady, 0)}
	cases := []struct {
		err  error
		want Failure
	}{
		{blocked, FailureBlocked},
		{fmt.Errorf("start: %w", &launchcheck.RefusedError{Report: risky}), FailureUnconfirmed},
		{campaign.ErrSelectionStartNeedsReview, FailureNeedsReview},
		{campaign.ErrNothingToSend, FailureNothingToSend},
		{campaign.ErrAlreadySent, FailureNothingToSend},
		{errors.New("queue down"), FailureStartFailed},
	}
	for _, c := range cases {
		if got := FailureOf(c.err); got != c.want {
			t.Errorf("%v: got %q, want %q", c.err, got, c.want)
		}
	}
}

func TestEveryFailureIsKnown(t *testing.T) {
	for _, failure := range Failures() {
		if !failure.Known() {
			t.Errorf("%q is listed but not known", failure)
		}
	}
	if Failure("").Known() || Failure("other").Known() {
		t.Fatal("an empty or foreign reason is not a schedule failure")
	}
	if len(Failures()) != 5 {
		t.Fatalf("got %d reasons, the front translates exactly five", len(Failures()))
	}
}

func TestAnAdmittedScheduleKeepsTheFingerprintTheCheckSaw(t *testing.T) {
	at := time.Date(2026, 11, 2, 9, 0, 0, 0, time.FixedZone("BRT", -3*3600))

	schedule, err := Admit(at, risky, risky.Fingerprint())
	if err != nil {
		t.Fatal(err)
	}
	if !schedule.At.Equal(at) || schedule.At.Location() != time.UTC || schedule.ConfirmedFingerprint != risky.Fingerprint() {
		t.Fatalf("got %+v", schedule)
	}

	clean, err := Admit(at, launchcheck.Report{}, "stale")
	if err != nil || clean.ConfirmedFingerprint != "" {
		t.Fatalf("a report without risks stores no fingerprint: %+v, %v", clean, err)
	}
}

func TestAScheduleIsRefusedLikeAStart(t *testing.T) {
	at := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	if _, err := Admit(at, risky, ""); !errors.Is(err, launchcheck.ErrUnconfirmed) {
		t.Fatalf("unconfirmed: %v", err)
	}
	blocked := launchcheck.Blocked(launchcheck.CodePhoneUnavailable, 0)
	if _, err := Admit(at, blocked, blocked.Fingerprint()); !errors.Is(err, launchcheck.ErrBlocked) {
		t.Fatalf("blocked: %v", err)
	}
}
