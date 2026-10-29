package calendar

import (
	"errors"
	"fmt"
	"time"
)

const (
	CodeSlotConflict             = "calendar_slot_conflict"
	CodeConflictCheckUnavailable = "calendar_conflict_check_unavailable"
	CodeCalendarFailed           = "calendar_failed"

	conflictLookupLimit = 10
)

var ErrConflictCheckUnavailable = errors.New("não foi possível consultar a agenda para conferir conflitos, o horário não foi reservado")

type ConflictCheck struct {
	Google      GoogleCalendarAPI
	AccessToken string
	Local       Repository
}

func (c ConflictCheck) Conflict(workspaceID string, start, end time.Time) (*CalendarEvent, error) {
	events, err := c.Google.ListGoogleEvents(c.AccessToken, start, end, "", conflictLookupLimit)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConflictCheckUnavailable, err)
	}
	events, err = WithLocalEvents(c.Local, workspaceID, start, end, events)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConflictCheckUnavailable, err)
	}
	for _, ev := range events {
		if ev != nil && ev.Transparency != "transparent" && ev.StartTime.Before(end) && ev.EndTime.After(start) {
			return ev, nil
		}
	}
	return nil, nil
}

func ErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrSlotConflict):
		return CodeSlotConflict
	case errors.Is(err, ErrConflictCheckUnavailable):
		return CodeConflictCheckUnavailable
	}
	return CodeCalendarFailed
}

type Reservation struct {
	GoogleEventID string
	MeetingLink   string
	Conflict      *CalendarEvent
}

func (c ConflictCheck) Reserve(workspaceID string, guard Slot, event *CalendarEvent, createMeet bool, sendUpdates string) (Reservation, error) {
	conflict, err := c.Conflict(workspaceID, guard.Start, guard.End)
	if err != nil {
		return Reservation{}, err
	}
	if conflict != nil {
		return Reservation{Conflict: conflict}, ErrSlotConflict
	}
	googleEventID, meetingLink, err := c.Google.CreateGoogleEvent(c.AccessToken, event, createMeet, sendUpdates)
	if err != nil {
		return Reservation{}, fmt.Errorf("google calendar create: %w", err)
	}
	return Reservation{GoogleEventID: googleEventID, MeetingLink: meetingLink}, nil
}
