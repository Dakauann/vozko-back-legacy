package billing

import "time"

func DueOn(periodEnd, due time.Time) bool {
	return !periodEnd.After(due)
}

func LatestIssuedDue(at time.Time) time.Time {
	next := FirstBillingAnchor(at, DefaultEmitDay, DefaultDueDay)
	y, m := subMonth(next.Year(), next.Month())
	return anchorDate(y, m, DefaultDueDay, next.Location())
}

func AccessEnd(periodEnd time.Time) time.Time {
	cutoff := CancelCutoff(periodEnd.In(LocationBRT()), LocationBRT(), DefaultCancelDeadlineDay)
	if cutoff.After(periodEnd) {
		return cutoff
	}
	return periodEnd
}

func subMonth(year int, month time.Month) (int, time.Month) {
	if month == time.January {
		return year - 1, time.December
	}
	return year, month - 1
}
