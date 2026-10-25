package billing

import "time"

func InEmitWindow(at time.Time) bool {
	day := at.In(LocationBRT()).Day()
	return day >= DefaultEmitDay && day < DefaultDueDay
}
