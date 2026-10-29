package container

import (
	"context"
	"time"

	"vozko/domain/calendar"
	"vozko/domain/customfield"
	calendar_usecase "vozko/usecases/calendar"
)

type e2eCalendar struct{}

func (e2eCalendar) Busy(context.Context, string, time.Time, time.Time) ([]calendar.Slot, error) {
	return nil, nil
}

func (e2eCalendar) Reserve(context.Context, calendar_usecase.BookingRequest) (calendar.Reservation, error) {
	return calendar.Reservation{GoogleEventID: "g-e2e"}, nil
}

func (e2eCalendar) Release(context.Context, string, string) error {
	return nil
}

func (e2eCalendar) Connected(context.Context, string) (bool, error) {
	return true, nil
}

type e2eMemo struct{}

func (e2eMemo) Remember(ctx context.Context, _ string, _ time.Duration, compute func(context.Context) ([]byte, error)) ([]byte, error) {
	return compute(ctx)
}

type e2eBookingLock struct{}

func (e2eBookingLock) Acquire(context.Context, string) (func() (bool, error), error) {
	return func() (bool, error) { return true, nil }, nil
}

type e2eLeadFields struct{}

func (e2eLeadFields) ListByObject(string, customfield.ObjectType) ([]*customfield.Definition, error) {
	return nil, nil
}
