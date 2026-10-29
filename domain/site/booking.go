package site

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"vozko/domain/calendar"
	"vozko/domain/siteform"
	"vozko/domain/working_hours"
)

const (
	BookingTag       = "vz-booking"
	BookingAttr      = "booking"
	BookingSlotField = siteform.FieldPrefix + "horario"
	bookingMessage   = siteform.FieldPrefix + "mensagem"
	bookingNameSlot  = "{nome}"
	bookingGuestName = "visitante"

	minBookingMinutes     = 5
	maxBookingMinutes     = 480
	bookingMinuteStep     = 5
	maxBookingBuffer      = 240
	maxBookingNotice      = 30 * 24 * 60
	maxBookingHorizonDays = 90
	maxBookingTitleRunes  = 120
	maxBookingNameRunes   = 120
	maxBookingNoteRunes   = 1000
	maxBookingSlotRunes   = 40
)

const (
	CodeBookingDurationInvalid      = "booking_duration_invalid"
	CodeBookingBufferInvalid        = "booking_buffer_invalid"
	CodeBookingNoticeInvalid        = "booking_notice_invalid"
	CodeBookingHorizonInvalid       = "booking_horizon_invalid"
	CodeBookingHoursInvalid         = "booking_hours_invalid"
	CodeBookingTitleInvalid         = "booking_title_invalid"
	CodeBookingConsentInvalid       = "booking_consent_invalid"
	CodeBookingFieldsInvalid        = "booking_fields_invalid"
	CodeBookingUnconfigured         = "booking_unconfigured"
	CodeBookingIDTaken              = "booking_id_taken"
	CodeBookingCalendarDisconnected = "booking_calendar_disconnected"
)

var (
	ErrBookingDurationInvalid = errors.New("site: a booking meeting lasts 5 to 480 minutes in steps of 5")
	ErrBookingBufferInvalid   = errors.New("site: a booking buffer is 0 to 240 minutes in steps of 5")
	ErrBookingNoticeInvalid   = errors.New("site: a booking notice is 0 minutes to 30 days")
	ErrBookingHorizonInvalid  = errors.New("site: a booking opens 1 to 90 days ahead")
	ErrBookingHoursInvalid    = errors.New("site: the booking hours need a time zone and a window that fits a meeting")
	ErrBookingTitleInvalid    = errors.New("site: the booking event title needs 1 to 120 characters and only the {nome} variable")
	ErrBookingConsentInvalid  = errors.New("site: a booking needs a consent text of up to 2000 characters")
	ErrBookingFieldsInvalid   = errors.New("site: the booking asks for an unknown or repeated field")
)

var bookingVariable = regexp.MustCompile(`\{[^{}]*\}`)

type BookingField string

const (
	BookingFieldEmail   BookingField = "email"
	BookingFieldMessage BookingField = "message"
)

type Booking struct {
	DurationMinutes int                `json:"duration_minutes"`
	BufferMinutes   int                `json:"buffer_minutes,omitempty"`
	NoticeMinutes   int                `json:"notice_minutes,omitempty"`
	HorizonDays     int                `json:"horizon_days"`
	Hours           working_hours.Spec `json:"hours"`
	Title           string             `json:"title"`
	Meet            bool               `json:"meet,omitempty"`
	Fields          []BookingField     `json:"fields,omitempty"`
	ConsentText     string             `json:"consent_text"`
}

func bookingError(id string, err error) error {
	return fmt.Errorf("%w: %w: booking %s", ErrSettingsInvalid, err, id)
}

func steppedWithin(value, low, high int) bool {
	return value >= low && value <= high && value%bookingMinuteStep == 0
}

func (b Booking) validate(id string) error {
	if !steppedWithin(b.DurationMinutes, minBookingMinutes, maxBookingMinutes) {
		return bookingError(id, ErrBookingDurationInvalid)
	}
	if !steppedWithin(b.BufferMinutes, 0, maxBookingBuffer) {
		return bookingError(id, ErrBookingBufferInvalid)
	}
	if b.NoticeMinutes < 0 || b.NoticeMinutes > maxBookingNotice {
		return bookingError(id, ErrBookingNoticeInvalid)
	}
	if b.HorizonDays < 1 || b.HorizonDays > maxBookingHorizonDays {
		return bookingError(id, ErrBookingHorizonInvalid)
	}
	if _, err := b.Schedule(); err != nil {
		return bookingError(id, err)
	}
	if !b.validTitle() {
		return bookingError(id, ErrBookingTitleInvalid)
	}
	if consent := strings.TrimSpace(b.ConsentText); consent == "" || utf8.RuneCountInString(consent) > maxConsentRunes {
		return bookingError(id, ErrBookingConsentInvalid)
	}
	seen := map[BookingField]bool{}
	for _, f := range b.Fields {
		if (f != BookingFieldEmail && f != BookingFieldMessage) || seen[f] {
			return bookingError(id, ErrBookingFieldsInvalid)
		}
		seen[f] = true
	}
	return nil
}

func (b Booking) validTitle() bool {
	title := strings.TrimSpace(b.Title)
	if title == "" || utf8.RuneCountInString(title) > maxBookingTitleRunes {
		return false
	}
	for _, variable := range bookingVariable.FindAllString(title, -1) {
		if variable != bookingNameSlot {
			return false
		}
	}
	return !strings.ContainsAny(bookingVariable.ReplaceAllString(title, ""), "{}")
}

func (b Booking) Schedule() (calendar.Schedule, error) {
	spec := b.Hours
	compiled, err := spec.Compile()
	if err == nil {
		err = compiled.Validate()
	}
	if err != nil {
		return calendar.Schedule{}, fmt.Errorf("%w: %w", ErrBookingHoursInvalid, err)
	}
	schedule := calendar.Schedule{
		Duration: time.Duration(b.DurationMinutes) * time.Minute, Buffer: time.Duration(b.BufferMinutes) * time.Minute,
		Notice: time.Duration(b.NoticeMinutes) * time.Minute, HorizonDays: b.HorizonDays, Hours: compiled,
	}
	if !schedule.FitsAMeeting() {
		return calendar.Schedule{}, fmt.Errorf("%w: no window fits a %d minute meeting", ErrBookingHoursInvalid, b.DurationMinutes)
	}
	return schedule, nil
}

func (b Booking) asks(f BookingField) bool {
	return slices.Contains(b.Fields, f)
}

func (b Booking) Manifest(id string) siteform.Manifest {
	fields := []siteform.Field{
		{Name: siteform.TargetName, Kind: siteform.KindText, Required: true, MaxLength: maxBookingNameRunes},
		{Name: siteform.TargetPhone, Kind: siteform.KindTel, Required: true},
	}
	if b.asks(BookingFieldEmail) {
		fields = append(fields, siteform.Field{Name: siteform.TargetEmail, Kind: siteform.KindEmail})
	}
	if b.asks(BookingFieldMessage) {
		fields = append(fields, siteform.Field{Name: bookingMessage, Kind: siteform.KindTextarea, MaxLength: maxBookingNoteRunes})
	}
	fields = append(fields,
		siteform.Field{Name: siteform.TargetConsent, Kind: siteform.KindCheckbox, Required: true, Label: strings.TrimSpace(b.ConsentText)},
		siteform.Field{Name: BookingSlotField, Kind: siteform.KindText, Required: true, MaxLength: maxBookingSlotRunes},
	)
	return siteform.Manifest{ID: id, Fields: fields, ConsentText: strings.TrimSpace(b.ConsentText)}
}

func (b Booking) EventTitle(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = bookingGuestName
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(b.Title), bookingNameSlot, name))
}

func BookingIDs(pages []RenderedPage) []string {
	seen := map[string]bool{}
	for _, p := range pages {
		for _, i := range p.Document.ByTag(BookingTag) {
			if id, _ := p.Document.AttrValue(i, BookingAttr); formSettingsID.MatchString(strings.TrimSpace(id)) {
				seen[strings.TrimSpace(id)] = true
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (c CheckedPages) bookingGaps(g *gapList, p RenderedPage, settings Settings) bool {
	found := false
	for _, i := range p.Document.ByTag(BookingTag) {
		found = true
		el := p.Document.Elements[i]
		id, _ := p.Document.AttrValue(i, BookingAttr)
		id = strings.TrimSpace(id)
		if _, ok := settings.Bookings[id]; !ok {
			g.add(CodeBookingUnconfigured+"|"+id+"|"+el.Node+"|"+p.Route, Issue{Code: CodeBookingUnconfigured, Route: p.Route, Node: el.Node, Detail: id})
			continue
		}
		if slices.ContainsFunc(c.Manifests, func(m siteform.Manifest) bool { return m.ID == id }) {
			g.add(CodeBookingIDTaken+"|"+id, Issue{Code: CodeBookingIDTaken, Route: p.Route, Node: el.Node, Detail: id})
		}
		if settings.Forms[id].CheckDealOwner(id) != nil {
			g.add(CodeFormDealOwnerRequired+"|"+id, Issue{Code: CodeFormDealOwnerRequired, Route: p.Route, Node: el.Node, Detail: id})
		}
		if !followUpReachable(settings.Bookings[id].Manifest(id), settings.Forms[id]) {
			g.add(CodeFormFollowUpFieldMissing+"|"+id, Issue{Code: CodeFormFollowUpFieldMissing, Route: p.Route, Node: el.Node, Detail: id})
		}
	}
	return found
}

const previewTimeZone = "America/Sao_Paulo"

func PreviewBooking() Booking {
	weekday := []working_hours.Window{{Start: "09:00", End: "18:00"}}
	return Booking{
		DurationMinutes: 30, NoticeMinutes: 60, HorizonDays: 7, Title: "Prévia com " + bookingNameSlot,
		Hours:       working_hours.Spec{Timezone: previewTimeZone, Days: map[string][]working_hours.Window{"mon": weekday, "tue": weekday, "wed": weekday, "thu": weekday, "fri": weekday}},
		Fields:      []BookingField{BookingFieldEmail, BookingFieldMessage},
		ConsentText: "Autorizo o contato sobre este agendamento.",
	}
}
