package calendar_usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"vozko/domain/calendar"
	"vozko/domain/googleaccount"
)

const bookingInvites = "all"

var ErrBookingCalendarIncomplete = errors.New("calendar: the booking calendar needs the access provider, google, the busy reader and the local events")

type BookingCalendar struct {
	access calendar.AccessProvider
	google calendar.GoogleCalendarAPI
	busy   calendar.BusyReader
	local  calendar.Repository
}

type BookingRequest struct {
	WorkspaceID string
	Slot        calendar.Slot
	Guard       calendar.Slot
	Title       string
	Description string
	TimeZone    string
	Attendee    *calendar.Attendee
	Meet        bool
}

func NewBookingCalendar(access calendar.AccessProvider, google calendar.GoogleCalendarAPI, busy calendar.BusyReader, local calendar.Repository) (*BookingCalendar, error) {
	if access == nil || google == nil || busy == nil || local == nil {
		return nil, ErrBookingCalendarIncomplete
	}
	return &BookingCalendar{access: access, google: google, busy: busy, local: local}, nil
}

func (b *BookingCalendar) token(ctx context.Context, workspaceID string) (string, error) {
	granted, err := b.access.CalendarAccess(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	return granted.Token, nil
}

func (b *BookingCalendar) Busy(ctx context.Context, workspaceID string, from, to time.Time) ([]calendar.Slot, error) {
	token, err := b.token(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	busy, err := b.busy.FreeBusy(token, from, to)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", calendar.ErrConflictCheckUnavailable, err)
	}
	local, err := calendar.WithLocalEvents(b.local, workspaceID, from, to, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", calendar.ErrConflictCheckUnavailable, err)
	}
	return append(busy, calendar.BusyOf(local)...), nil
}

func (b *BookingCalendar) Reserve(ctx context.Context, req BookingRequest) (calendar.Reservation, error) {
	token, err := b.token(ctx, req.WorkspaceID)
	if err != nil {
		return calendar.Reservation{}, err
	}
	event := &calendar.CalendarEvent{
		WorkspaceID: req.WorkspaceID, Title: req.Title, Description: req.Description, StartTime: req.Slot.Start, EndTime: req.Slot.End,
		TimeZone: req.TimeZone, Status: "confirmed", Visibility: "default", Transparency: "opaque", RemindersUseDefault: true,
	}
	updates := "none"
	if req.Attendee != nil {
		event.Attendees, updates = []calendar.Attendee{*req.Attendee}, bookingInvites
	}
	return calendar.ConflictCheck{Google: b.google, AccessToken: token, Local: b.local}.Reserve(req.WorkspaceID, req.Guard, event, req.Meet, updates)
}

func (b *BookingCalendar) Release(ctx context.Context, workspaceID, googleEventID string) error {
	token, err := b.token(ctx, workspaceID)
	if err != nil {
		return err
	}
	return b.google.DeleteGoogleEvent(token, googleEventID, bookingInvites)
}

func (b *BookingCalendar) Connected(ctx context.Context, workspaceID string) (bool, error) {
	_, err := b.access.CalendarAccess(ctx, workspaceID)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, calendar.ErrGoogleNotConnected), errors.Is(err, googleaccount.ErrNotConnected):
		return false, nil
	}
	return false, err
}
