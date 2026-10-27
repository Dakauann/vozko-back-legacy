package ai

import (
	"fmt"
	"time"
)

var portugueseWeekdays = [...]string{
	time.Sunday:    "domingo",
	time.Monday:    "segunda-feira",
	time.Tuesday:   "terça-feira",
	time.Wednesday: "quarta-feira",
	time.Thursday:  "quinta-feira",
	time.Friday:    "sexta-feira",
	time.Saturday:  "sábado",
}

func ClockLine(at time.Time, zone *time.Location) string {
	if zone == nil {
		zone = time.UTC
	}
	local := at.In(zone)
	return fmt.Sprintf("Agora: %s, %s (%s)", portugueseWeekdays[local.Weekday()], local.Format("02/01/2006, 15:04"), zone.String())
}
