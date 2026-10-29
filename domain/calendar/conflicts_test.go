package calendar

import (
	"errors"
	"testing"
	"time"
)

type listingCalendar struct {
	GoogleCalendarAPI
	events []*CalendarEvent
	err    error
}

func (l listingCalendar) ListGoogleEvents(string, time.Time, time.Time, string, int) ([]*CalendarEvent, error) {
	return l.events, l.err
}

func TestSlotConflictRefusesWhenTheCalendarCannotBeRead(t *testing.T) {
	start := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	for name, check := range map[string]ConflictCheck{
		"google listing fails": {Google: listingCalendar{err: errors.New("google down")}},
		"local events fail":    {Google: listingCalendar{}, Local: localEvents{err: errors.New("db down")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := check.Conflict("ws", start, end); !errors.Is(err, ErrConflictCheckUnavailable) || ErrorCode(err) != CodeConflictCheckUnavailable {
				t.Fatalf("err = %v, code %q", err, ErrorCode(err))
			}
		})
	}
}

func TestSlotConflictFindsTheFirstOverlappingBusyEvent(t *testing.T) {
	start := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	events := []*CalendarEvent{
		{Title: "livre", StartTime: start, EndTime: end, Transparency: "transparent"},
		{Title: "antes", StartTime: start.Add(-time.Hour), EndTime: start},
		{Title: "consulta", StartTime: start.Add(30 * time.Minute), EndTime: end.Add(time.Hour)},
	}
	conflict, err := ConflictCheck{Google: listingCalendar{events: events}}.Conflict("ws", start, end)
	if err != nil || conflict == nil || conflict.Title != "consulta" {
		t.Fatalf("conflict = %+v, %v", conflict, err)
	}
	free, err := ConflictCheck{Google: listingCalendar{events: events[:2]}}.Conflict("ws", start, end)
	if err != nil || free != nil {
		t.Fatalf("free slot reported busy: %+v, %v", free, err)
	}
}

func TestErrorCodeNamesSlotConflicts(t *testing.T) {
	if ErrorCode(ErrSlotConflict) != CodeSlotConflict {
		t.Fatal(ErrorCode(ErrSlotConflict))
	}
}

type bookingCalendar struct {
	listingCalendar
	created []*CalendarEvent
	meet    bool
	updates string
	failure error
}

func (b *bookingCalendar) CreateGoogleEvent(_ string, event *CalendarEvent, meet bool, updates string) (string, string, error) {
	if b.failure != nil {
		return "", "", b.failure
	}
	b.created, b.meet, b.updates = append(b.created, event), meet, updates
	return "g-1", "https://meet.google.com/abc", nil
}

func TestReserveCreatesTheEventOnlyWhenTheGuardedSlotIsFree(t *testing.T) {
	start := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	slot := Slot{Start: start, End: start.Add(time.Hour)}
	guard := Slot{Start: start.Add(-15 * time.Minute), End: slot.End.Add(15 * time.Minute)}
	event := &CalendarEvent{Title: "Avaliação", StartTime: slot.Start, EndTime: slot.End}

	free := &bookingCalendar{listingCalendar: listingCalendar{events: []*CalendarEvent{{Title: "antes", StartTime: guard.Start.Add(-time.Hour), EndTime: guard.Start}}}}
	reserved, err := ConflictCheck{Google: free}.Reserve("ws", guard, event, true, "all")
	if err != nil || reserved.GoogleEventID != "g-1" || reserved.MeetingLink == "" || len(free.created) != 1 || !free.meet || free.updates != "all" {
		t.Fatalf("reserved = %+v, %v, %+v", reserved, err, free)
	}

	busy := &bookingCalendar{listingCalendar: listingCalendar{events: []*CalendarEvent{{Title: "café", StartTime: slot.End, EndTime: slot.End.Add(10 * time.Minute)}}}}
	reserved, err = ConflictCheck{Google: busy}.Reserve("ws", guard, event, false, "none")
	if !errors.Is(err, ErrSlotConflict) || reserved.Conflict == nil || reserved.Conflict.Title != "café" || len(busy.created) != 0 {
		t.Fatalf("reserved = %+v, %v", reserved, err)
	}

	down := &bookingCalendar{listingCalendar: listingCalendar{err: errors.New("google down")}}
	if _, err := (ConflictCheck{Google: down}).Reserve("ws", guard, event, false, "none"); !errors.Is(err, ErrConflictCheckUnavailable) || len(down.created) != 0 {
		t.Fatalf("err = %v", err)
	}

	failing := &bookingCalendar{failure: errors.New("quota")}
	if _, err := (ConflictCheck{Google: failing}).Reserve("ws", guard, event, false, "none"); err == nil || errors.Is(err, ErrSlotConflict) {
		t.Fatalf("err = %v", err)
	}
}
