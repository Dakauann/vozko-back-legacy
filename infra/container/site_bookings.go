package container

import (
	"log"
	"time"

	domainCache "vozko/domain/cache"
	calendar_domain "vozko/domain/calendar"
	redisCache "vozko/infra/cache"
	calendar_usecase "vozko/usecases/calendar"
)

const (
	siteBookingLockTTL             = 30 * time.Second
	siteBookingLockPoll            = 100 * time.Millisecond
	siteBookingSlotsComputeTimeout = 15 * time.Second
)

func (c *Container) siteBookingCalendar() *calendar_usecase.BookingCalendar {
	if c.google == nil || c.google.CalendarAccess == nil || c.services == nil || c.services.googleCalendar == nil || c.repositories == nil || c.repositories.calendar == nil {
		log.Fatalf("[site-bookings] the Google accounts, the calendar API and the calendar repository must exist before page bookings")
	}
	busy, ok := c.services.googleCalendar.(calendar_domain.BusyReader)
	if !ok {
		log.Fatalf("[site-bookings] the calendar API cannot read busy times")
	}
	calendar, err := calendar_usecase.NewBookingCalendar(c.google.CalendarAccess, c.services.googleCalendar, busy, c.repositories.calendar)
	if err != nil {
		log.Fatalf("[site-bookings] %v", err)
	}
	return calendar
}

func (c *Container) siteBookingLock() domainCache.Locker {
	lock, err := domainCache.NewOwnedLock(c.redisProvider.SharedState(), siteBookingLockTTL, siteBookingLockPoll)
	if err != nil {
		log.Fatalf("[site-bookings] booking lock: %v", err)
	}
	return lock
}

func (c *Container) siteBookingSlots() domainCache.Memo {
	return redisCache.NewSharedStateMemo(c.redisProvider.SharedState(), siteBookingSlotsComputeTimeout)
}
