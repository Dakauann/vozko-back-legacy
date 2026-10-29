package calendar

import (
	"time"

	"vozko/domain/working_hours"
)

type Schedule struct {
	Duration    time.Duration
	Buffer      time.Duration
	Notice      time.Duration
	HorizonDays int
	Hours       *working_hours.Schedule
}

func (s Schedule) location() *time.Location {
	return s.Hours.Location()
}

func (s Schedule) startOfDay(at time.Time) time.Time {
	day := at.In(s.location())
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, s.location())
}

func (s Schedule) Range(now time.Time) (time.Time, time.Time) {
	today := s.startOfDay(now)
	return now.Add(s.Notice), today.AddDate(0, 0, s.HorizonDays)
}

func (s Schedule) candidates(from, to time.Time) []Slot {
	var out []Slot
	if s.Duration <= 0 || s.Hours == nil {
		return out
	}
	for day := s.startOfDay(from).AddDate(0, 0, -1); day.Before(to); day = day.AddDate(0, 0, 1) {
		for _, segment := range s.Hours.Segments(day) {
			for start := segment[0]; !start.Add(s.Duration).After(segment[1]); start = start.Add(s.Duration) {
				if !start.Before(from) && start.Before(to) {
					out = append(out, Slot{Start: start, End: start.Add(s.Duration)})
				}
			}
		}
	}
	return out
}

func (s Schedule) Guard(slot Slot) Slot {
	return Slot{Start: slot.Start.Add(-s.Buffer), End: slot.End.Add(s.Buffer)}
}

func (s Schedule) Open(now time.Time, busy []Slot) []Slot {
	from, to := s.Range(now)
	blocked := mergeSlots(from.Add(-s.Buffer), to.Add(s.Buffer), busy)
	var out []Slot
	for _, c := range s.candidates(from, to) {
		if !overlapsAny(s.Guard(c), blocked) {
			out = append(out, c)
		}
	}
	return out
}

func overlapsAny(slot Slot, busy []Slot) bool {
	for _, b := range busy {
		if b.Start.Before(slot.End) && b.End.After(slot.Start) {
			return true
		}
	}
	return false
}

func (s Schedule) Offers(now, start time.Time) (Slot, bool) {
	from, to := s.Range(now)
	if s.Hours == nil || start.Before(from) || !start.Before(to) {
		return Slot{}, false
	}
	day := s.startOfDay(start)
	for _, c := range s.candidates(laterOf(day, from), earlierOf(day.AddDate(0, 0, 1), to)) {
		if c.Start.Equal(start) {
			return c, true
		}
	}
	return Slot{}, false
}

func (s Schedule) FitsAMeeting() bool {
	if s.Hours == nil || s.Duration <= 0 {
		return false
	}
	sunday := time.Date(2001, time.January, 7, 0, 0, 0, 0, s.location())
	for offset := 0; offset < 7; offset++ {
		for _, segment := range s.Hours.Segments(sunday.AddDate(0, 0, offset)) {
			if segment[1].Sub(segment[0]) >= s.Duration {
				return true
			}
		}
	}
	return false
}
