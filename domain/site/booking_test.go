package site

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"vozko/domain/siteform"
	"vozko/domain/sitehtml"
	"vozko/domain/working_hours"
)

const bookingSettings = `{
  "bookings": {"agenda": {
    "duration_minutes": 30, "buffer_minutes": 10, "notice_minutes": 120, "horizon_days": 14,
    "hours": {"timezone": "America/Sao_Paulo", "days": {"mon": [{"start": "09:00", "end": "12:00"}], "wed": [{"start": "14:00", "end": "18:00"}]}},
    "title": "Avaliação com {nome}", "meet": true, "fields": ["email", "message"],
    "consent_text": "Autorizo o contato pelo WhatsApp."
  }},
  "forms": {"agenda": {"notify": true}}
}`

func validBooking() Booking {
	return Booking{
		DurationMinutes: 30, HorizonDays: 7, Title: "Reunião com {nome}", ConsentText: "Autorizo o contato.",
		Hours: workingHours(`{"timezone":"America/Sao_Paulo","days":{"tue":[{"start":"09:00","end":"10:00"}]}}`),
	}
}

func TestDecodeSettingsReadsBookings(t *testing.T) {
	s, err := DecodeSettings([]byte(bookingSettings))
	if err != nil {
		t.Fatal(err)
	}
	b := s.Bookings["agenda"]
	if b.DurationMinutes != 30 || b.BufferMinutes != 10 || !b.Meet || len(b.Fields) != 2 || b.Hours.Timezone != "America/Sao_Paulo" {
		t.Fatalf("booking = %+v", b)
	}
	schedule, err := b.Schedule()
	if err != nil {
		t.Fatal(err)
	}
	if schedule.Duration != 30*time.Minute || schedule.Buffer != 10*time.Minute || schedule.Notice != 2*time.Hour || schedule.HorizonDays != 14 {
		t.Fatalf("schedule = %+v", schedule)
	}
	monday := time.Date(2026, 11, 2, 10, 0, 0, 0, schedule.Hours.Location())
	if got := schedule.Hours.Segments(monday); len(got) != 1 || got[0][0].Hour() != 9 {
		t.Fatalf("segments = %v", got)
	}
}

func TestBookingRulesRefuseWithTheirOwnCodes(t *testing.T) {
	cases := map[string]struct {
		edit func(*Booking)
		want error
	}{
		"short meeting":       {func(b *Booking) { b.DurationMinutes = 3 }, ErrBookingDurationInvalid},
		"odd meeting":         {func(b *Booking) { b.DurationMinutes = 32 }, ErrBookingDurationInvalid},
		"long meeting":        {func(b *Booking) { b.DurationMinutes = 485 }, ErrBookingDurationInvalid},
		"negative buffer":     {func(b *Booking) { b.BufferMinutes = -5 }, ErrBookingBufferInvalid},
		"huge buffer":         {func(b *Booking) { b.BufferMinutes = 245 }, ErrBookingBufferInvalid},
		"negative notice":     {func(b *Booking) { b.NoticeMinutes = -1 }, ErrBookingNoticeInvalid},
		"notice past a month": {func(b *Booking) { b.NoticeMinutes = 43201 }, ErrBookingNoticeInvalid},
		"no horizon":          {func(b *Booking) { b.HorizonDays = 0 }, ErrBookingHorizonInvalid},
		"far horizon":         {func(b *Booking) { b.HorizonDays = 91 }, ErrBookingHorizonInvalid},
		"unknown zone":        {func(b *Booking) { b.Hours.Timezone = "Mars/Olympus" }, ErrBookingHoursInvalid},
		"no zone":             {func(b *Booking) { b.Hours.Timezone = "" }, ErrBookingHoursInvalid},
		"never open":          {func(b *Booking) { b.Hours = workingHours(`{"timezone":"America/Sao_Paulo","days":{}}`) }, ErrBookingHoursInvalid},
		"window too short": {func(b *Booking) {
			b.Hours = workingHours(`{"timezone":"America/Sao_Paulo","days":{"mon":[{"start":"09:00","end":"09:20"}]}}`)
		}, ErrBookingHoursInvalid},
		"overlapping": {func(b *Booking) {
			b.Hours = workingHours(`{"timezone":"America/Sao_Paulo","days":{"mon":[{"start":"09:00","end":"11:00"},{"start":"10:00","end":"12:00"}]}}`)
		}, ErrBookingHoursInvalid},
		"empty title":      {func(b *Booking) { b.Title = "  " }, ErrBookingTitleInvalid},
		"long title":       {func(b *Booking) { b.Title = strings.Repeat("a", 121) }, ErrBookingTitleInvalid},
		"unknown variable": {func(b *Booking) { b.Title = "Reunião com {email}" }, ErrBookingTitleInvalid},
		"no consent":       {func(b *Booking) { b.ConsentText = " " }, ErrBookingConsentInvalid},
		"long consent":     {func(b *Booking) { b.ConsentText = strings.Repeat("a", 2001) }, ErrBookingConsentInvalid},
		"unknown field":    {func(b *Booking) { b.Fields = []BookingField{"cpf"} }, ErrBookingFieldsInvalid},
		"field twice":      {func(b *Booking) { b.Fields = []BookingField{BookingFieldEmail, BookingFieldEmail} }, ErrBookingFieldsInvalid},
		"valid":            {nil, nil},
	}
	for name, c := range cases {
		b := validBooking()
		if c.edit != nil {
			c.edit(&b)
		}
		err := Settings{Bookings: map[string]Booking{"agenda": b}}.Validate()
		if c.want == nil {
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			continue
		}
		if !errors.Is(err, c.want) || !errors.Is(err, ErrSettingsInvalid) {
			t.Fatalf("%s: err = %v", name, err)
		}
		if ErrorCode(err) == CodeSettingsInvalid || ErrorCode(err) == "" {
			t.Fatalf("%s: code = %q", name, ErrorCode(err))
		}
	}
}

func TestABookingIDFollowsTheFormIDShape(t *testing.T) {
	err := Settings{Bookings: map[string]Booking{"Agenda Principal": validBooking()}}.Validate()
	if !errors.Is(err, ErrSettingsInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestTheBookingManifestCarriesTheChosenFieldsAndTheSlot(t *testing.T) {
	b := validBooking()
	plain := b.Manifest("agenda")
	if plain.ID != "agenda" || plain.ConsentText != b.ConsentText || !plain.HasIdentity() {
		t.Fatalf("manifest = %+v", plain)
	}
	names := func(m siteform.Manifest) string {
		var out []string
		for _, f := range m.Fields {
			out = append(out, f.Name)
		}
		return strings.Join(out, ",")
	}
	if got := names(plain); got != "lead.name,lead.phone,consent,field.horario" {
		t.Fatalf("fields = %s", got)
	}
	b.Fields = []BookingField{BookingFieldMessage, BookingFieldEmail}
	if got := names(b.Manifest("agenda")); got != "lead.name,lead.phone,lead.email,field.mensagem,consent,field.horario" {
		t.Fatalf("fields = %s", got)
	}
	values, err := plain.Validate(map[string]string{"lead.name": "Ana", "lead.phone": "11999990000", "consent": "on", BookingSlotField: "2026-11-03T09:00:00-03:00"})
	if err != nil || values.Phone == "" || values.Fields["horario"] == "" {
		t.Fatalf("values = %+v, %v", values, err)
	}
	if _, err := plain.Validate(map[string]string{"lead.name": "Ana", "lead.phone": "11999990000", "consent": "on"}); err == nil {
		t.Fatal("a booking without a slot was accepted")
	}
}

func TestTheEventTitleNamesTheVisitor(t *testing.T) {
	b := validBooking()
	if got := b.EventTitle("  Ana Souza "); got != "Reunião com Ana Souza" {
		t.Fatalf("title = %q", got)
	}
	if got := b.EventTitle(""); got != "Reunião com visitante" {
		t.Fatalf("title = %q", got)
	}
}

func bookingPage(t *testing.T, html string) []RenderedPage {
	t.Helper()
	doc, err := sitehtml.Parse(html)
	if err != nil {
		t.Fatal(err)
	}
	return []RenderedPage{{Route: "/", Document: doc}}
}

func issueLabels(issues []Issue) []string {
	var out []string
	for _, i := range issues {
		out = append(out, i.Code+":"+i.Detail)
	}
	return out
}

func TestBookingBlocksOpenGapsWhenTheyCannotBeBooked(t *testing.T) {
	html := `<main><a href="/privacidade">Privacidade</a><vz-booking booking="agenda"></vz-booking><vz-booking booking="outra"></vz-booking><vz-booking></vz-booking>` +
		`<form data-vz-form="contato"><input name="lead.phone"><label><input type="checkbox" name="consent">Aceito</label></form><vz-booking booking="contato"></vz-booking></main>`
	pages := bookingPage(t, html)
	settings := Settings{Bookings: map[string]Booking{"agenda": validBooking(), "contato": validBooking()}}
	got := strings.Join(issueLabels(CheckPages(pages).OpenGaps(pages, settings)), " ")
	for _, want := range []string{CodeBookingUnconfigured + ":outra", CodeBookingUnconfigured + ":", CodeBookingIDTaken + ":contato"} {
		if !strings.Contains(got, want) {
			t.Fatalf("gaps %q miss %q", got, want)
		}
	}
	if strings.Contains(got, CodeBookingUnconfigured+":agenda") {
		t.Fatalf("a configured booking opened a gap: %q", got)
	}
}

func TestABookingNeedsAPrivacyLinkAndADealOwnerLikeAForm(t *testing.T) {
	pages := bookingPage(t, `<main><vz-booking booking="agenda"></vz-booking></main>`)
	settings := Settings{Bookings: map[string]Booking{"agenda": validBooking()}, Forms: map[string]FormSettings{"agenda": {FunnelID: "fun_1"}}}
	got := strings.Join(issueLabels(CheckPages(pages).OpenGaps(pages, settings)), " ")
	if !strings.Contains(got, CodePrivacyLinkMissing) || !strings.Contains(got, CodeFormDealOwnerRequired+":agenda") {
		t.Fatalf("gaps = %q", got)
	}
	if ids := BookingIDs(pages); len(ids) != 1 || ids[0] != "agenda" {
		t.Fatalf("ids = %v", ids)
	}
}

func TestBookingCodesAreKnownGapsAndErrorCodes(t *testing.T) {
	for _, code := range []string{CodeBookingUnconfigured, CodeBookingIDTaken, CodeBookingCalendarDisconnected} {
		if !IsGap(code) {
			t.Fatalf("%s is not a gap", code)
		}
	}
	codes := strings.Join(Codes(), " ")
	for _, code := range []string{CodeBookingDurationInvalid, CodeBookingBufferInvalid, CodeBookingNoticeInvalid, CodeBookingHorizonInvalid, CodeBookingHoursInvalid, CodeBookingTitleInvalid, CodeBookingConsentInvalid, CodeBookingFieldsInvalid, CodeBookingCalendarDisconnected} {
		if !strings.Contains(codes, code) {
			t.Fatalf("%s is missing from Codes", code)
		}
	}
}

func workingHours(raw string) working_hours.Spec {
	var spec working_hours.Spec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		panic(err)
	}
	return spec
}

func TestThePreviewBookingIsAValidWeekdaySchedule(t *testing.T) {
	b := PreviewBooking()
	if err := (Settings{Bookings: map[string]Booking{"previa": b}}).Validate(); err != nil {
		t.Fatal(err)
	}
	schedule, _ := b.Schedule()
	monday := time.Date(2026, 11, 2, 6, 0, 0, 0, schedule.Hours.Location())
	if got := schedule.Open(monday, nil); len(got) == 0 || got[0].Start.Hour() != 9 {
		t.Fatalf("slots = %v", got)
	}
}
