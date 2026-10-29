package siteforms

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"vozko/domain/calendar"
	"vozko/domain/site"
	"vozko/domain/siteform"
	"vozko/domain/sitesubmission"
	"vozko/domain/working_hours"
	calendar_usecase "vozko/usecases/calendar"
)

type calendarDesk struct {
	busy       []calendar.Slot
	busyErr    error
	busyCalls  int
	reserved   []calendar_usecase.BookingRequest
	reserveErr error
	released   []string
}

func (c *calendarDesk) Busy(_ context.Context, workspaceID string, from, to time.Time) ([]calendar.Slot, error) {
	c.busyCalls++
	return c.busy, c.busyErr
}

func (c *calendarDesk) Reserve(_ context.Context, req calendar_usecase.BookingRequest) (calendar.Reservation, error) {
	if c.reserveErr != nil {
		return calendar.Reservation{}, c.reserveErr
	}
	c.reserved = append(c.reserved, req)
	return calendar.Reservation{GoogleEventID: "g-1", MeetingLink: "https://meet.google.com/abc"}, nil
}

func (c *calendarDesk) Release(_ context.Context, workspaceID, googleEventID string) error {
	c.released = append(c.released, googleEventID)
	return nil
}

type memoShelf struct {
	items map[string][]byte
}

func (m *memoShelf) Remember(ctx context.Context, key string, _ time.Duration, compute func(context.Context) ([]byte, error)) ([]byte, error) {
	if raw, ok := m.items[key]; ok {
		return raw, nil
	}
	raw, err := compute(ctx)
	if err != nil {
		return nil, err
	}
	m.items[key] = raw
	return raw, nil
}

type lockDesk struct {
	err      error
	acquired []string
	released int
}

func (l *lockDesk) Acquire(_ context.Context, key string) (func() (bool, error), error) {
	if l.err != nil {
		return nil, l.err
	}
	l.acquired = append(l.acquired, key)
	return func() (bool, error) { l.released++; return true, nil }, nil
}

func thursdayBooking() site.Booking {
	return site.Booking{
		DurationMinutes: 30, NoticeMinutes: 60, HorizonDays: 7, Title: "Avaliação com {nome}", Meet: true, ConsentText: "Autorizo o contato.",
		Fields: []site.BookingField{site.BookingFieldEmail},
		Hours:  working_hours.Spec{Timezone: "America/Sao_Paulo", Days: map[string][]working_hours.Window{"thu": {{Start: "09:00", End: "11:00"}}}},
	}
}

func (w *world) publishBooking(t *testing.T, booking site.Booking, settings site.FormSettings) string {
	t.Helper()
	err := w.svc.Activate(context.Background(), ActivateInput{
		WorkspaceID: "w1", ProjectID: "p1", DeploymentID: "d1", Manifests: []siteform.Manifest{booking.Manifest("agenda")},
		Settings: map[string]site.FormSettings{"agenda": settings}, Bookings: map[string]site.Booking{"agenda": booking},
	})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := w.svc.KeysFor(context.Background(), "w1", "p1", []string{"agenda"})
	if err != nil {
		t.Fatal(err)
	}
	return keys["agenda"]
}

func bookingFields(start string) map[string]any {
	return map[string]any{"lead.name": "Ana", "lead.phone": "+55 84 99999-0000", "lead.email": "ana@example.com", "consent": "on", site.BookingSlotField: start}
}

const thursdayNine = "2026-10-29T09:00:00-03:00"

func starts(slots []calendar.Slot) []string {
	out := make([]string, 0, len(slots))
	for _, s := range slots {
		out = append(out, s.Start.UTC().Format("15:04"))
	}
	return out
}

func TestSlotsListOnlyFreeTimesFromTheCalendarAndCacheThem(t *testing.T) {
	w := newWorld(t)
	key := w.publishBooking(t, thursdayBooking(), site.FormSettings{})
	at := func(hour, minute int) time.Time { return time.Date(2026, 10, 29, hour, minute, 0, 0, time.UTC) }
	w.calendar.busy = []calendar.Slot{{Start: at(12, 30), End: at(13, 0)}}
	got, err := w.svc.Slots(context.Background(), key, visitor)
	if err != nil {
		t.Fatal(err)
	}
	if s := starts(got.Slots); len(s) != 3 || s[0] != "12:00" || s[1] != "13:00" || s[2] != "13:30" {
		t.Fatalf("slots = %v", s)
	}
	if got.TimeZone != "America/Sao_Paulo" || got.DurationMinutes != 30 || got.ConsentText != "Autorizo o contato." || len(got.Fields) != 1 || got.Preview {
		t.Fatalf("result = %+v", got)
	}
	w.calendar.busy = nil
	if again, err := w.svc.Slots(context.Background(), key, visitor); err != nil || len(again.Slots) != 3 || w.calendar.busyCalls != 1 {
		t.Fatalf("again = %+v, %v, calls %d", again, err, w.calendar.busyCalls)
	}
	w.clock = time.Date(2026, 10, 29, 11, 30, 0, 0, time.UTC)
	if later, err := w.svc.Slots(context.Background(), key, visitor); err != nil || len(later.Slots) != 2 {
		t.Fatalf("a cached slot inside the notice was still offered: %+v, %v", later, err)
	}
}

func TestSlotsFailClosedAndRefuseWhatIsNotABooking(t *testing.T) {
	w := newWorld(t)
	key := w.publishBooking(t, thursdayBooking(), site.FormSettings{})
	w.calendar.busyErr = calendar.ErrGoogleNotConnected
	if _, err := w.svc.Slots(context.Background(), key, visitor); !errors.Is(err, sitesubmission.ErrBookingUnavailable) {
		t.Fatalf("err = %v", err)
	}
	formKey := w.publish(t, site.FormSettings{})
	if _, err := w.svc.Slots(context.Background(), formKey, visitor); !errors.Is(err, sitesubmission.ErrFormUnknown) {
		t.Fatalf("err = %v", err)
	}
	if _, err := w.svc.Slots(context.Background(), key, Visitor{Origin: "https://evil.example", IP: "1.1.1.1"}); err == nil {
		t.Fatal("a foreign origin read the slots")
	}
	w.ipLimit.deny = map[string]bool{}
	w.ipLimit.err = errors.New("redis down")
	if _, err := w.svc.Slots(context.Background(), key, visitor); err == nil {
		t.Fatal("slots were listed without a working limiter")
	}
}

func TestPreviewSlotsComeFromTheDemoScheduleWithoutTheCalendar(t *testing.T) {
	w := newWorld(t)
	preview := Visitor{Origin: "https://preview--abc.paginas.vozko.dev", IP: "203.0.113.7"}
	got, err := w.svc.Slots(context.Background(), sitesubmission.PreviewFormKey, preview)
	if err != nil || !got.Preview || len(got.Slots) == 0 || w.calendar.busyCalls != 0 {
		t.Fatalf("preview = %+v, %v", got, err)
	}
	challenge, err := w.svc.Challenge(context.Background(), sitesubmission.PreviewFormKey, preview)
	if err != nil {
		t.Fatal(err)
	}
	w.clock = w.clock.Add(4 * time.Second)
	raw, _ := json.Marshal(map[string]any{"utm": map[string]string{}, "idempotencyKey": "k-12345678", "route": "/", "fields": bookingFields(got.Slots[0].Start.Format(time.RFC3339)), "challengeToken": challenge.Token, "nonce": solve(t, challenge)})
	result, err := w.svc.Book(context.Background(), sitesubmission.PreviewFormKey, preview, raw)
	if err != nil || result.Booked == nil || len(w.calendar.reserved) != 0 || len(w.subs.rows) != 0 || len(w.leads.writes) != 0 {
		t.Fatalf("preview booking = %+v, %v", result, err)
	}
}

func TestABookingReservesTheSlotAndThenRunsTheFormPipeline(t *testing.T) {
	w := newWorld(t)
	key := w.publishBooking(t, thursdayBooking(), site.FormSettings{Notify: true})
	result, err := w.svc.Book(context.Background(), key, visitor, w.body(t, key, bookingFields(thursdayNine)))
	if err != nil {
		t.Fatal(err)
	}
	req := w.calendar.reserved[0]
	if req.WorkspaceID != "w1" || req.Title != "Avaliação com Ana" || !req.Meet || req.Attendee == nil || req.Attendee.Email != "ana@example.com" || req.TimeZone != "America/Sao_Paulo" {
		t.Fatalf("reserve %+v", req)
	}
	if !req.Slot.Start.Equal(time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)) || req.Slot.End.Sub(req.Slot.Start) != 30*time.Minute || !req.Guard.Start.Equal(req.Slot.Start) {
		t.Fatalf("slot %+v guard %+v", req.Slot, req.Guard)
	}
	if len(w.lock.acquired) != 1 || w.lock.acquired[0] != bookingLockPrefix+"w1" || w.lock.released != 1 {
		t.Fatalf("lock %+v", w.lock)
	}
	row := w.subs.rows[0]
	if row.FormID != "agenda" || row.Fields[site.BookingSlotField] != thursdayNine || row.Fields[bookingEndField] != "2026-10-29T09:30:00-03:00" || row.Fields[bookingLinkField] != "https://meet.google.com/abc" || row.LeadID != "lead-1" {
		t.Fatalf("row %+v", row)
	}
	if result.Booked == nil || !result.Booked.Start.Equal(req.Slot.Start) || result.Booked.MeetingLink == "" || result.Booked.TimeZone != "America/Sao_Paulo" || result.SubmissionID != row.ID {
		t.Fatalf("result %+v", result)
	}
	if len(w.notices.drafts) != 1 || len(w.leads.triggers) != 0 {
		t.Fatalf("notices %d triggers %d", len(w.notices.drafts), len(w.leads.triggers))
	}
	again, err := w.svc.Book(context.Background(), key, visitor, w.body(t, key, bookingFields(thursdayNine)))
	if err != nil || again.SubmissionID != row.ID || len(w.calendar.reserved) != 1 || again.Booked == nil {
		t.Fatalf("replay %+v, %v, reserved %d", again, err, len(w.calendar.reserved))
	}
}

func TestABookingRefusesTimesTheScheduleNeverOfferedOrTheCalendarTook(t *testing.T) {
	w := newWorld(t)
	key := w.publishBooking(t, thursdayBooking(), site.FormSettings{})
	for name, start := range map[string]string{"off grid": "2026-10-29T09:10:00-03:00", "closed day": "2026-10-30T09:00:00-03:00", "inside notice": "2026-10-28T09:30:00-03:00"} {
		_, err := w.svc.Book(context.Background(), key, visitor, w.body(t, key, bookingFields(start), withKey(name)))
		if !errors.Is(err, sitesubmission.ErrSlotTaken) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
	_, err := w.svc.Book(context.Background(), key, visitor, w.body(t, key, bookingFields("amanhã"), withKey("garbled")))
	var fields *sitesubmission.FieldsError
	if !errors.As(err, &fields) || fields.Details[site.BookingSlotField] == "" {
		t.Fatalf("err = %v", err)
	}
	w.calendar.reserveErr = calendar.ErrSlotConflict
	if _, err := w.svc.Book(context.Background(), key, visitor, w.body(t, key, bookingFields(thursdayNine), withKey("taken"))); !errors.Is(err, sitesubmission.ErrSlotTaken) {
		t.Fatalf("err = %v", err)
	}
	w.calendar.reserveErr = calendar.ErrConflictCheckUnavailable
	if _, err := w.svc.Book(context.Background(), key, visitor, w.body(t, key, bookingFields(thursdayNine), withKey("down"))); !errors.Is(err, sitesubmission.ErrBookingUnavailable) {
		t.Fatalf("err = %v", err)
	}
	w.calendar.reserveErr = nil
	w.lock.err = context.DeadlineExceeded
	if _, err := w.svc.Book(context.Background(), key, visitor, w.body(t, key, bookingFields(thursdayNine), withKey("locked"))); !errors.Is(err, sitesubmission.ErrBookingUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if len(w.leads.writes) != 0 || len(w.subs.rows) != 0 || len(w.calendar.reserved) != 0 {
		t.Fatalf("a refused booking left traces: writes %d rows %d reserved %d", len(w.leads.writes), len(w.subs.rows), len(w.calendar.reserved))
	}
}

func TestAFailedSaveCancelsTheReservedEvent(t *testing.T) {
	w := newWorld(t)
	key := w.publishBooking(t, thursdayBooking(), site.FormSettings{})
	w.subs.failSave = errors.New("db down")
	if _, err := w.svc.Book(context.Background(), key, visitor, w.body(t, key, bookingFields(thursdayNine))); err == nil {
		t.Fatal("a failed save reported success")
	}
	if len(w.calendar.released) != 1 || w.calendar.released[0] != "g-1" {
		t.Fatalf("released %v", w.calendar.released)
	}
}

func TestBookingsAndFormsNeverCrossTheirEndpoints(t *testing.T) {
	w := newWorld(t)
	bookingKey := w.publishBooking(t, thursdayBooking(), site.FormSettings{})
	if _, err := w.svc.Submit(context.Background(), bookingKey, visitor, w.body(t, bookingKey, bookingFields(thursdayNine))); !errors.Is(err, sitesubmission.ErrFormUnknown) {
		t.Fatalf("a booking was submitted as a plain form: %v", err)
	}
	formKey := w.publish(t, site.FormSettings{})
	if _, err := w.svc.Book(context.Background(), formKey, visitor, w.body(t, formKey, goodFields(), withKey("form"))); !errors.Is(err, sitesubmission.ErrFormUnknown) {
		t.Fatalf("a plain form was booked: %v", err)
	}
}

func withKey(suffix string) func(map[string]any) {
	return func(b map[string]any) {
		b["idempotencyKey"] = "key-" + strings.ReplaceAll(suffix, " ", "-") + "-0123456789"
	}
}
