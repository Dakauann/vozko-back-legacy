package campaign

import (
	"errors"
	"time"
)

type ScheduleClaimer interface {
	ClaimScheduledStart(campaignID string, dueBy time.Time) (bool, error)
}

var ErrSelectionScheduleRefused = errors.New("campaign: a send prepared from a lead selection starts from its review in the leads page and cannot be scheduled")

func RefuseSelectionSchedule(source string, scheduled bool) error {
	if source == SourceLeadSelection && scheduled {
		return ErrSelectionScheduleRefused
	}
	return nil
}
