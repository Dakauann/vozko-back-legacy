package siteforms

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"vozko/domain/cache"
	"vozko/domain/calendar"
	"vozko/domain/site"
	"vozko/domain/siteform"
	"vozko/domain/sitesubmission"
	calendar_usecase "vozko/usecases/calendar"
)

const (
	bookingLockPrefix = "sites:booking:lock:"
	slotsKeyPrefix    = "sites:booking:slots:"
	slotsTTL          = time.Minute
	bookingLockWait   = 5 * time.Second
	bookingEndField   = siteform.FieldPrefix + "horario_fim"
	bookingLinkField  = siteform.FieldPrefix + "link_reuniao"
	bookingLogPrefix  = "[site-bookings]"
)

var bookingSlotKey = strings.TrimPrefix(site.BookingSlotField, siteform.FieldPrefix)

type Booked struct {
	Start       time.Time
	End         time.Time
	TimeZone    string
	MeetingLink string
}

type BookingSlots struct {
	TimeZone        string
	DurationMinutes int
	Fields          []site.BookingField
	ConsentText     string
	Slots           []calendar.Slot
	Preview         bool
}

func slotsOf(b site.Booking, slots []calendar.Slot, preview bool) BookingSlots {
	return BookingSlots{
		TimeZone: b.Hours.Timezone, DurationMinutes: b.DurationMinutes, Fields: b.Fields, ConsentText: strings.TrimSpace(b.ConsentText),
		Slots: slots, Preview: preview,
	}
}

func (s *Service) Slots(ctx context.Context, formKey string, v Visitor) (BookingSlots, error) {
	form, err := s.admitted(ctx, formKey, v.Origin)
	if err != nil {
		return BookingSlots{}, err
	}
	if form != nil && form.Booking == nil {
		return BookingSlots{}, sitesubmission.ErrFormUnknown
	}
	if err := s.limitVisitor("slots", formKey, v); err != nil {
		return BookingSlots{}, err
	}
	now := s.now()
	if form == nil {
		preview := site.PreviewBooking()
		schedule, err := preview.Schedule()
		if err != nil {
			return BookingSlots{}, err
		}
		return slotsOf(preview, schedule.Open(now, nil), true), nil
	}
	schedule, err := form.Booking.Schedule()
	if err != nil {
		return BookingSlots{}, err
	}
	open, err := cache.Remember(ctx, s.d.Slots, slotsKeyPrefix+form.DeploymentID+":"+formKey, slotsTTL, func(ctx context.Context) ([]calendar.Slot, error) {
		from, to := schedule.Range(now)
		busy, err := s.d.Calendar.Busy(ctx, form.WorkspaceID, from.Add(-schedule.Buffer), to.Add(schedule.Buffer))
		if err != nil {
			return nil, err
		}
		return schedule.Open(now, busy), nil
	})
	if err != nil {
		log.Printf("%s slots of form %s could not be read: %v", bookingLogPrefix, formKey, err)
		return BookingSlots{}, fmt.Errorf("%w: %w", sitesubmission.ErrBookingUnavailable, err)
	}
	from, _ := schedule.Range(now)
	fresh := make([]calendar.Slot, 0, len(open))
	for _, slot := range open {
		if !slot.Start.Before(from) {
			fresh = append(fresh, slot)
		}
	}
	return slotsOf(*form.Booking, fresh, false), nil
}

func (s *Service) Book(ctx context.Context, formKey string, v Visitor, raw []byte) (SubmitResult, error) {
	return s.submit(ctx, formKey, v, raw, true)
}

func (s *Service) reserve(ctx context.Context, form ActiveForm, values siteform.Values, record *sitesubmission.Submission) (string, error) {
	booking := *form.Booking
	schedule, err := booking.Schedule()
	if err != nil {
		return "", err
	}
	start, err := time.Parse(time.RFC3339, values.Fields[bookingSlotKey])
	if err != nil {
		return "", &sitesubmission.FieldsError{Details: map[string]string{site.BookingSlotField: siteform.CodeValueInvalid}}
	}
	slot, offered := schedule.Offers(s.now(), start)
	if !offered {
		return "", sitesubmission.ErrSlotTaken
	}
	waiting, cancel := context.WithTimeout(ctx, bookingLockWait)
	defer cancel()
	unlock, err := s.d.BookingLock.Acquire(waiting, bookingLockPrefix+form.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("%w: %w", sitesubmission.ErrBookingUnavailable, err)
	}
	defer func() {
		if _, err := unlock(); err != nil {
			log.Printf("%s the booking lock of workspace %s waits for its expiry: %v", bookingLogPrefix, form.WorkspaceID, err)
		}
	}()
	reserved, err := s.d.Calendar.Reserve(ctx, calendar_usecase.BookingRequest{
		WorkspaceID: form.WorkspaceID, Slot: slot, Guard: schedule.Guard(slot), Title: booking.EventTitle(values.Name),
		Description: bookingDescription(values), TimeZone: booking.Hours.Timezone, Attendee: attendeeOf(values), Meet: booking.Meet,
	})
	switch {
	case errors.Is(err, calendar.ErrSlotConflict):
		return "", sitesubmission.ErrSlotTaken
	case err != nil:
		log.Printf("%s form %s could not book %s: %v", bookingLogPrefix, form.FormKey, slot.Start.Format(time.RFC3339), err)
		return "", fmt.Errorf("%w: %w", sitesubmission.ErrBookingUnavailable, err)
	}
	loc := schedule.Hours.Location()
	record.Fields[site.BookingSlotField] = slot.Start.In(loc).Format(time.RFC3339)
	record.Fields[bookingEndField] = slot.End.In(loc).Format(time.RFC3339)
	if reserved.MeetingLink != "" {
		record.Fields[bookingLinkField] = reserved.MeetingLink
	}
	return reserved.GoogleEventID, nil
}

func (s *Service) release(ctx context.Context, form ActiveForm, googleEventID string) {
	if googleEventID == "" {
		return
	}
	if err := s.d.Calendar.Release(context.WithoutCancel(ctx), form.WorkspaceID, googleEventID); err != nil {
		log.Printf("%s event %s of form %s stays on the calendar after a booking that was not saved: %v", bookingLogPrefix, googleEventID, form.FormKey, err)
	}
}

func attendeeOf(values siteform.Values) *calendar.Attendee {
	if values.Email == "" {
		return nil
	}
	return &calendar.Attendee{Email: values.Email, Name: values.Name}
}

func bookingDescription(values siteform.Values) string {
	lines := []string{"Agendado pelo site."}
	add := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			lines = append(lines, label+": "+strings.TrimSpace(value))
		}
	}
	add("Nome", values.Name)
	if values.Phone != "" {
		add("WhatsApp", "+"+strings.TrimPrefix(values.Phone, "+"))
	}
	add("E-mail", values.Email)
	add("Mensagem", values.Fields["mensagem"])
	return strings.Join(lines, "\n")
}

func bookedOf(form ActiveForm, fields map[string]string) *Booked {
	if form.Booking == nil {
		return nil
	}
	start, err := time.Parse(time.RFC3339, fields[site.BookingSlotField])
	if err != nil {
		return nil
	}
	end, err := time.Parse(time.RFC3339, fields[bookingEndField])
	if err != nil {
		end = start.Add(time.Duration(form.Booking.DurationMinutes) * time.Minute)
	}
	return &Booked{Start: start, End: end, TimeZone: form.Booking.Hours.Timezone, MeetingLink: fields[bookingLinkField]}
}

func previewBooked(body sitesubmission.Body) *Booked {
	fields, _ := body.Values()
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(fields[site.BookingSlotField]))
	if err != nil {
		return nil
	}
	preview := site.PreviewBooking()
	return &Booked{Start: start, End: start.Add(time.Duration(preview.DurationMinutes) * time.Minute), TimeZone: preview.Hours.Timezone}
}
