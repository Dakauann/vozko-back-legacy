package calendar_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/calendar"
	"vozko/domain/googleaccount"
	"vozko/domain/shared"
)

type bookingAccess struct {
	err error
}

func (a bookingAccess) CalendarAccess(context.Context, string) (googleaccount.Access, error) {
	return googleaccount.Access{Token: "tok", Email: "agenda@clinica.com"}, a.err
}

type bookingGoogle struct {
	calendar.GoogleCalendarAPI
	listed    []*calendar.CalendarEvent
	listErr   error
	busy      []calendar.Slot
	busyErr   error
	created   *calendar.CalendarEvent
	meet      bool
	updates   string
	deleted   string
	deletedBy string
}

func (g *bookingGoogle) ListGoogleEvents(string, time.Time, time.Time, string, int) ([]*calendar.CalendarEvent, error) {
	return g.listed, g.listErr
}

func (g *bookingGoogle) FreeBusy(token string, from, to time.Time) ([]calendar.Slot, error) {
	return g.busy, g.busyErr
}

func (g *bookingGoogle) CreateGoogleEvent(_ string, event *calendar.CalendarEvent, meet bool, updates string) (string, string, error) {
	g.created, g.meet, g.updates = event, meet, updates
	return "g-1", "https://meet.google.com/x", nil
}

func (g *bookingGoogle) DeleteGoogleEvent(_ string, id, updates string) error {
	g.deleted, g.deletedBy = id, updates
	return nil
}

type bookingLocal struct {
	calendar.Repository
	items []*calendar.CalendarEvent
	err   error
}

func (l bookingLocal) ListEvents(calendar.ListEventsInput) (*shared.PaginatedResult[*calendar.CalendarEvent], error) {
	return &shared.PaginatedResult[*calendar.CalendarEvent]{Items: l.items}, l.err
}

func newBookingCalendar(t *testing.T, access calendar.AccessProvider, google *bookingGoogle, local calendar.Repository) *BookingCalendar {
	t.Helper()
	b, err := NewBookingCalendar(access, google, google, local)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBookingBusyJoinsGoogleBusyTimeAndLocalEvents(t *testing.T) {
	from := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	google := &bookingGoogle{busy: []calendar.Slot{{Start: from.Add(time.Hour), End: from.Add(2 * time.Hour)}}}
	local := bookingLocal{items: []*calendar.CalendarEvent{{ID: "l-1", StartTime: from.Add(5 * time.Hour), EndTime: from.Add(6 * time.Hour)}}}
	busy, err := newBookingCalendar(t, bookingAccess{}, google, local).Busy(context.Background(), "ws", from, from.Add(24*time.Hour))
	if err != nil || len(busy) != 2 || !busy[1].Start.Equal(from.Add(5*time.Hour)) {
		t.Fatalf("busy = %+v, %v", busy, err)
	}
}

func TestBookingBusyFailsClosed(t *testing.T) {
	from := time.Now()
	cases := map[string]*BookingCalendar{
		"disconnected": newBookingCalendar(t, bookingAccess{err: calendar.ErrGoogleNotConnected}, &bookingGoogle{}, bookingLocal{}),
		"google down":  newBookingCalendar(t, bookingAccess{}, &bookingGoogle{busyErr: errors.New("503")}, bookingLocal{}),
		"local down":   newBookingCalendar(t, bookingAccess{}, &bookingGoogle{}, bookingLocal{err: errors.New("db")}),
	}
	for name, b := range cases {
		if _, err := b.Busy(context.Background(), "ws", from, from.Add(time.Hour)); err == nil {
			t.Fatalf("%s: busy time was read as free", name)
		}
	}
}

func TestBookingReserveCreatesAPrivateEventAndInvitesOnlyTheVisitor(t *testing.T) {
	start := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	google := &bookingGoogle{}
	b := newBookingCalendar(t, bookingAccess{}, google, bookingLocal{})
	reserved, err := b.Reserve(context.Background(), BookingRequest{
		WorkspaceID: "ws", Slot: calendar.Slot{Start: start, End: start.Add(30 * time.Minute)}, Guard: calendar.Slot{Start: start.Add(-10 * time.Minute), End: start.Add(40 * time.Minute)},
		Title: "Avaliação com Ana", Description: "Agendado pela página", TimeZone: "America/Sao_Paulo", Attendee: &calendar.Attendee{Email: "ana@email.com", Name: "Ana"}, Meet: true,
	})
	if err != nil || reserved.GoogleEventID != "g-1" || reserved.MeetingLink == "" {
		t.Fatalf("reserved = %+v, %v", reserved, err)
	}
	ev := google.created
	if ev.Title != "Avaliação com Ana" || len(ev.Attendees) != 1 || ev.GuestsCanSeeOtherGuests || ev.GuestsCanInviteOthers || ev.Transparency != "opaque" || !google.meet || google.updates != "all" {
		t.Fatalf("event = %+v meet %v updates %q", ev, google.meet, google.updates)
	}
	google.created = nil
	if _, err := b.Reserve(context.Background(), BookingRequest{WorkspaceID: "ws", Slot: calendar.Slot{Start: start, End: start.Add(time.Hour)}, Guard: calendar.Slot{Start: start, End: start.Add(time.Hour)}, Title: "x", TimeZone: "UTC"}); err != nil || google.updates != "none" {
		t.Fatalf("updates %q, %v", google.updates, err)
	}
}

func TestBookingReserveRefusesATakenSlotAndAnUnreachableCalendar(t *testing.T) {
	start := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	req := BookingRequest{WorkspaceID: "ws", Slot: calendar.Slot{Start: start, End: start.Add(time.Hour)}, Guard: calendar.Slot{Start: start, End: start.Add(time.Hour)}, Title: "x", TimeZone: "UTC"}
	taken := &bookingGoogle{listed: []*calendar.CalendarEvent{{StartTime: start, EndTime: start.Add(time.Hour)}}}
	if _, err := newBookingCalendar(t, bookingAccess{}, taken, bookingLocal{}).Reserve(context.Background(), req); !errors.Is(err, calendar.ErrSlotConflict) || taken.created != nil {
		t.Fatalf("err = %v", err)
	}
	down := &bookingGoogle{listErr: errors.New("timeout")}
	if _, err := newBookingCalendar(t, bookingAccess{}, down, bookingLocal{}).Reserve(context.Background(), req); !errors.Is(err, calendar.ErrConflictCheckUnavailable) || down.created != nil {
		t.Fatalf("err = %v", err)
	}
	if _, err := newBookingCalendar(t, bookingAccess{err: calendar.ErrGoogleNotConnected}, &bookingGoogle{}, bookingLocal{}).Reserve(context.Background(), req); !errors.Is(err, calendar.ErrGoogleNotConnected) {
		t.Fatalf("err = %v", err)
	}
}

func TestBookingReleaseCancelsTheEventForEveryone(t *testing.T) {
	google := &bookingGoogle{}
	if err := newBookingCalendar(t, bookingAccess{}, google, bookingLocal{}).Release(context.Background(), "ws", "g-1"); err != nil || google.deleted != "g-1" || google.deletedBy != "all" {
		t.Fatalf("deleted %q by %q, %v", google.deleted, google.deletedBy, err)
	}
}

func TestBookingConnectedSeparatesADisconnectedAccountFromAnOutage(t *testing.T) {
	cases := map[string]struct {
		err       error
		connected bool
		fails     bool
	}{
		"connected":    {nil, true, false},
		"disconnected": {calendar.ErrGoogleNotConnected, false, false},
		"reconnect":    {googleaccount.ErrNeedsReconnect, false, false},
		"outage":       {errors.New("google auth: timeout"), false, true},
	}
	for name, c := range cases {
		connected, err := newBookingCalendar(t, bookingAccess{err: c.err}, &bookingGoogle{}, bookingLocal{}).Connected(context.Background(), "ws")
		if connected != c.connected || (err != nil) != c.fails {
			t.Fatalf("%s: connected %v err %v", name, connected, err)
		}
	}
}
