package shared

import "regexp"

var clockTime = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func IsClockTime(text string) bool {
	return clockTime.MatchString(text)
}
