package scheduledstart

import (
	"context"
	"errors"
	"time"

	"vozko/domain/campaign"
	"vozko/domain/campaign/launchcheck"
)

type Failure string

const (
	FailureBlocked       Failure = "schedule_blocked"
	FailureUnconfirmed   Failure = "schedule_unconfirmed"
	FailureNeedsReview   Failure = "schedule_needs_review"
	FailureNothingToSend Failure = "schedule_nothing_to_send"
	FailureStartFailed   Failure = "schedule_start_failed"
)

var causes = []struct {
	err     error
	failure Failure
}{
	{launchcheck.ErrBlocked, FailureBlocked},
	{launchcheck.ErrUnconfirmed, FailureUnconfirmed},
	{campaign.ErrSelectionStartNeedsReview, FailureNeedsReview},
	{campaign.ErrNothingToSend, FailureNothingToSend},
	{campaign.ErrAlreadySent, FailureNothingToSend},
}

func Failures() []Failure {
	return []Failure{FailureBlocked, FailureUnconfirmed, FailureNeedsReview, FailureNothingToSend, FailureStartFailed}
}

func (f Failure) Known() bool {
	for _, known := range Failures() {
		if f == known {
			return true
		}
	}
	return false
}

func FailureOf(err error) Failure {
	for _, cause := range causes {
		if errors.Is(err, cause.err) {
			return cause.failure
		}
	}
	return FailureStartFailed
}

type Recorder interface {
	RecordScheduleFailure(ctx context.Context, campaignID string, failure Failure) error
}

type Schedule struct {
	At                   time.Time
	ConfirmedFingerprint string
}

func Admit(at time.Time, report launchcheck.Report, confirmedFingerprint string) (Schedule, error) {
	if err := report.Admit(confirmedFingerprint); err != nil {
		return Schedule{}, err
	}
	return Schedule{At: at.UTC(), ConfirmedFingerprint: report.Fingerprint()}, nil
}
