package calendar

import (
	"testing"
	"time"

	"vozko/domain/working_hours"
)

func bookingSchedule(t *testing.T) Schedule {
	t.Helper()
	return Schedule{
		Duration:    30 * time.Minute,
		Buffer:      15 * time.Minute,
		Notice:      2 * time.Hour,
		HorizonDays: 3,
		Hours: working_hours.New(saoPaulo(t), map[time.Weekday][]working_hours.Interval{
			time.Wednesday: {{StartMin: 9 * 60, EndMin: 11 * 60}, {StartMin: 14 * 60, EndMin: 15 * 60}},
			time.Thursday:  {{StartMin: 9 * 60, EndMin: 10 * 60}},
		}),
	}
}

func slotLabels(slots []Slot, loc *time.Location) []string {
	out := make([]string, 0, len(slots))
	for _, s := range slots {
		out = append(out, s.Start.In(loc).Format("Mon 15:04")+"-"+s.End.In(loc).Format("15:04"))
	}
	return out
}

func sameLabels(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("slots = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slots = %v, want %v", got, want)
		}
	}
}

func TestOpenSlotsFollowEachWeekdayWindowTheNoticeAndTheHorizon(t *testing.T) {
	s := bookingSchedule(t)
	loc := s.Hours.Location()
	now := time.Date(2026, 10, 28, 8, 10, 0, 0, loc)
	got := slotLabels(s.Open(now, nil), loc)
	sameLabels(t, got, []string{"Wed 10:30-11:00", "Wed 14:00-14:30", "Wed 14:30-15:00", "Thu 09:00-09:30", "Thu 09:30-10:00"})
}

func TestOpenSlotsKeepTheBufferAroundBusyTimeAndIgnoreFreeEvents(t *testing.T) {
	s := bookingSchedule(t)
	loc := s.Hours.Location()
	now := time.Date(2026, 10, 27, 7, 0, 0, 0, loc)
	at := func(day, hour, minute int) time.Time { return time.Date(2026, 10, day, hour, minute, 0, 0, loc) }
	busy := BusyOf([]*CalendarEvent{
		{StartTime: at(28, 9, 50), EndTime: at(28, 10, 0)},
		{StartTime: at(29, 9, 0), EndTime: at(29, 10, 0), Transparency: "transparent"},
	})
	got := slotLabels(s.Open(now, busy), loc)
	sameLabels(t, got, []string{"Wed 09:00-09:30", "Wed 10:30-11:00", "Wed 14:00-14:30", "Wed 14:30-15:00", "Thu 09:00-09:30", "Thu 09:30-10:00"})
}

func TestTheHorizonCountsTodayAsTheFirstDay(t *testing.T) {
	s := bookingSchedule(t)
	s.HorizonDays = 1
	loc := s.Hours.Location()
	now := time.Date(2026, 10, 28, 6, 0, 0, 0, loc)
	got := slotLabels(s.Open(now, nil), loc)
	sameLabels(t, got, []string{"Wed 09:00-09:30", "Wed 09:30-10:00", "Wed 10:00-10:30", "Wed 10:30-11:00", "Wed 14:00-14:30", "Wed 14:30-15:00"})
}

func TestOffersAcceptsOnlyAStartTheScheduleWouldShow(t *testing.T) {
	s := bookingSchedule(t)
	loc := s.Hours.Location()
	now := time.Date(2026, 10, 28, 8, 10, 0, 0, loc)
	cases := map[string]struct {
		start time.Time
		want  bool
	}{
		"listed":           {time.Date(2026, 10, 29, 9, 30, 0, 0, loc), true},
		"listed elsewhere": {time.Date(2026, 10, 29, 12, 30, 0, 0, time.UTC), true},
		"inside notice":    {time.Date(2026, 10, 28, 9, 30, 0, 0, loc), false},
		"off the grid":     {time.Date(2026, 10, 28, 14, 10, 0, 0, loc), false},
		"closed day":       {time.Date(2026, 10, 30, 9, 0, 0, 0, loc), false},
		"past the horizon": {time.Date(2026, 11, 4, 9, 0, 0, 0, loc), false},
	}
	for name, c := range cases {
		slot, ok := s.Offers(now, c.start)
		if ok != c.want {
			t.Fatalf("%s: offered = %v", name, ok)
		}
		if ok && (!slot.Start.Equal(c.start) || slot.End.Sub(slot.Start) != s.Duration) {
			t.Fatalf("%s: slot = %+v", name, slot)
		}
	}
}

func TestGuardWidensTheSlotByTheBuffer(t *testing.T) {
	s := bookingSchedule(t)
	start := time.Date(2026, 10, 28, 9, 0, 0, 0, s.Hours.Location())
	guarded := s.Guard(Slot{Start: start, End: start.Add(s.Duration)})
	if !guarded.Start.Equal(start.Add(-15*time.Minute)) || !guarded.End.Equal(start.Add(45*time.Minute)) {
		t.Fatalf("guarded = %+v", guarded)
	}
}

func TestAnOvernightWindowStillOffersItsEarlyMorningSlots(t *testing.T) {
	loc := saoPaulo(t)
	s := Schedule{Duration: time.Hour, HorizonDays: 1, Hours: working_hours.New(loc, map[time.Weekday][]working_hours.Interval{
		time.Tuesday: {{StartMin: 22 * 60, EndMin: 26 * 60}},
	})}
	now := time.Date(2026, 10, 28, 0, 30, 0, 0, loc)
	sameLabels(t, slotLabels(s.Open(now, nil), loc), []string{"Wed 01:00-02:00"})
}

func TestAScheduleWithoutHoursOffersNothing(t *testing.T) {
	s := Schedule{Duration: time.Hour, HorizonDays: 7}
	if got := s.Open(time.Now(), nil); len(got) != 0 {
		t.Fatalf("slots = %v", got)
	}
	if _, ok := s.Offers(time.Now(), time.Now().Add(time.Hour)); ok {
		t.Fatal("a schedule without hours offered a slot")
	}
}

func TestFitsAMeetingNeedsOneWindowAsLongAsTheMeeting(t *testing.T) {
	s := bookingSchedule(t)
	if !s.FitsAMeeting() {
		t.Fatal("a schedule with two hour windows did not fit a half hour meeting")
	}
	s.Duration = 3 * time.Hour
	if s.FitsAMeeting() {
		t.Fatal("a three hour meeting fitted two hour windows")
	}
}
