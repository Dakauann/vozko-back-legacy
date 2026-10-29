package siteformhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"vozko/delivery/http/response/responsetest"
	"vozko/domain/calendar"
	"vozko/domain/site"
	"vozko/domain/sitesubmission"
	"vozko/usecases/siteforms"
)

type bookingForms struct {
	fakeForms
	slots   siteforms.BookingSlots
	booked  siteforms.SubmitResult
	bookErr error
	slotErr error
	booking []byte
}

func (b *bookingForms) Slots(_ context.Context, _ string, v siteforms.Visitor) (siteforms.BookingSlots, error) {
	b.visitor = v
	return b.slots, b.slotErr
}

func (b *bookingForms) Book(_ context.Context, _ string, v siteforms.Visitor, body []byte) (siteforms.SubmitResult, error) {
	b.visitor, b.booking = v, body
	return b.booked, b.bookErr
}

func (f *fakeForms) Slots(_ context.Context, _ string, v siteforms.Visitor) (siteforms.BookingSlots, error) {
	f.visitor = v
	return siteforms.BookingSlots{}, f.err
}

func (f *fakeForms) Book(_ context.Context, _ string, v siteforms.Visitor, body []byte) (siteforms.SubmitResult, error) {
	f.visitor, f.body = v, body
	return f.result, f.err
}

func TestSlotsAnswerOnlyStartAndEndForThePublishedOrigin(t *testing.T) {
	start := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	forms := &bookingForms{fakeForms: fakeForms{allowed: true}, slots: siteforms.BookingSlots{
		TimeZone: "America/Sao_Paulo", DurationMinutes: 30, Fields: []site.BookingField{site.BookingFieldEmail}, ConsentText: "Autorizo",
		Slots: []calendar.Slot{{Start: start, End: start.Add(30 * time.Minute)}},
	}}
	rec := send(publicServer(forms), http.MethodGet, "/public/sites/forms/key-1/slots", pageOrigin, "")
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != pageOrigin || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("slots %d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
	want := `{"timeZone":"America/Sao_Paulo","durationMinutes":30,"fields":["email"],"consentText":"Autorizo","preview":false,"slots":[{"start":"2026-10-29T12:00:00Z","end":"2026-10-29T12:30:00Z"}]}`
	if strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("body %s", rec.Body.String())
	}
	if forms.visitor.Origin != pageOrigin || forms.visitor.IP != "203.0.113.7" {
		t.Fatalf("visitor %+v", forms.visitor)
	}
	forms.slots = siteforms.BookingSlots{TimeZone: "America/Sao_Paulo"}
	if rec := send(publicServer(forms), http.MethodGet, "/public/sites/forms/key-1/slots", pageOrigin, ""); !strings.Contains(rec.Body.String(), `"fields":[],`) || !strings.Contains(rec.Body.String(), `"slots":[]`) {
		t.Fatalf("empty lists must stay lists: %s", rec.Body.String())
	}
	forms.slotErr = sitesubmission.ErrBookingUnavailable
	rec = send(publicServer(forms), http.MethodGet, "/public/sites/forms/key-1/slots", pageOrigin, "")
	if rec.Code != http.StatusServiceUnavailable || responsetest.CodeOf(t, rec) != sitesubmission.CodeBookingUnavailable {
		t.Fatalf("unavailable %d %s", rec.Code, rec.Body.String())
	}
	foreign := &bookingForms{fakeForms: fakeForms{allowed: false}, slotErr: sitesubmission.ErrOriginRefused}
	if rec := send(publicServer(foreign), http.MethodGet, "/public/sites/forms/key-1/slots", "https://evil.example", ""); rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Code != http.StatusForbidden {
		t.Fatalf("foreign %d %v", rec.Code, rec.Header())
	}
}

func TestABookingAnswersTheSubmissionAndTheBookedTime(t *testing.T) {
	start := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	forms := &bookingForms{fakeForms: fakeForms{allowed: true}, booked: siteforms.SubmitResult{
		SubmissionID: "s1", Success: sitesubmission.SuccessAction{Kind: site.SuccessMessage},
		Booked: &siteforms.Booked{Start: start, End: start.Add(30 * time.Minute), TimeZone: "America/Sao_Paulo", MeetingLink: "https://meet.google.com/abc"},
	}}
	rec := send(publicServer(forms), http.MethodPost, "/public/sites/forms/key-1/bookings", pageOrigin, `{"fields":{}}`)
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != pageOrigin || string(forms.booking) != `{"fields":{}}` {
		t.Fatalf("booking %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	booking, _ := got["booking"].(map[string]any)
	if got["id"] != "s1" || booking["start"] != "2026-10-29T12:00:00Z" || booking["timeZone"] != "America/Sao_Paulo" || booking["meetingLink"] != "https://meet.google.com/abc" {
		t.Fatalf("body %v", got)
	}
	for err, want := range map[error]struct {
		status int
		code   string
	}{
		sitesubmission.ErrSlotTaken:          {http.StatusConflict, sitesubmission.CodeSlotTaken},
		sitesubmission.ErrBookingUnavailable: {http.StatusServiceUnavailable, sitesubmission.CodeBookingUnavailable},
		sitesubmission.ErrFormUnknown:        {http.StatusNotFound, sitesubmission.CodeFormUnknown},
	} {
		forms.bookErr = err
		rec := send(publicServer(forms), http.MethodPost, "/public/sites/forms/key-1/bookings", pageOrigin, `{}`)
		if rec.Code != want.status || responsetest.CodeOf(t, rec) != want.code {
			t.Fatalf("%v: %d %s", err, rec.Code, rec.Body.String())
		}
	}
	rec = send(publicServer(forms), http.MethodOptions, "/public/sites/forms/key-1/bookings", pageOrigin, "")
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != pageOrigin {
		t.Fatalf("preflight %d", rec.Code)
	}
}
