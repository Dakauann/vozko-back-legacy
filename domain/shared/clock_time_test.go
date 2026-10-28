package shared

import "testing"

func TestIsClockTimeAcceptsHHMMFromMidnightToTheLastMinute(t *testing.T) {
	for text, want := range map[string]bool{"00:00": true, "23:59": true, "17:00": true, "24:00": false, "9:00": false, "10:60": false, "": false} {
		if IsClockTime(text) != want {
			t.Errorf("%q = %v", text, !want)
		}
	}
}
