package sitesubmission

import "errors"

const (
	CodeSlotTaken          = "booking_slot_taken"
	CodeBookingUnavailable = "booking_unavailable"
)

var (
	ErrSlotTaken          = errors.New("sitesubmission: the chosen time is no longer free")
	ErrBookingUnavailable = errors.New("sitesubmission: the calendar of this booking cannot be read right now")
)
