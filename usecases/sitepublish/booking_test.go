package sitepublish_usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"vozko/domain/site"
	"vozko/domain/working_hours"
)

type fakeCalendars struct {
	connected bool
	err       error
	asked     []string
}

func (f *fakeCalendars) Connected(_ context.Context, workspaceID string) (bool, error) {
	f.asked = append(f.asked, workspaceID)
	return f.connected, f.err
}

const bookingHTML = `<!doctype html><html lang="pt-BR"><head><title>Agenda</title><script type="module" src="/assets/vz-runtime-ABCD1234.js"></script></head><body><h1>Agende</h1><a href="/privacidade">Privacidade</a><vz-booking booking="agenda"></vz-booking></body></html>`

func bookingRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	r.builder.candidate = htmlCandidate(bookingHTML)
	r.forms.keys["agenda"] = "fk_agenda"
	r.projects.snap.Settings = site.Settings{
		Bookings: map[string]site.Booking{"agenda": {
			DurationMinutes: 30, HorizonDays: 7, Title: "Avaliação com {nome}", ConsentText: "Autorizo o contato.",
			Hours: working_hours.Spec{Timezone: "America/Sao_Paulo", Days: map[string][]working_hours.Window{"mon": {{Start: "09:00", End: "12:00"}}}},
		}},
		Forms: map[string]site.FormSettings{"agenda": {Notify: true}},
	}
	return r
}

func TestPublishActivatesABookingAsAFormWithItsSchedule(t *testing.T) {
	r := bookingRig(t)
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err != nil {
		t.Fatal(err)
	}
	activated := r.forms.activated[0]
	if len(activated.Manifests) != 1 || activated.Manifests[0].ID != "agenda" || activated.Manifests[0].ConsentText != "Autorizo o contato." {
		t.Fatalf("manifests %+v", activated.Manifests)
	}
	if b, ok := activated.Bookings["agenda"]; !ok || b.DurationMinutes != 30 || !activated.Settings["agenda"].Notify {
		t.Fatalf("activation %+v", activated)
	}
	if len(r.calendars.asked) != 1 || r.calendars.asked[0] != wsID {
		t.Fatalf("calendar asked %v", r.calendars.asked)
	}
	if req := r.builder.requests; len(req) == 0 || req[len(req)-1].FormKeys["agenda"] != "fk_agenda" {
		t.Fatalf("the build never received the booking key: %+v", req)
	}
}

func TestADisconnectedCalendarIsAPublishGapAndAnOutageRefuses(t *testing.T) {
	r := bookingRig(t)
	r.calendars.connected = false
	_, err := r.svc.Publish(context.Background(), actor, projectID)
	var issues *IssuesFailure
	if !errors.As(err, &issues) || !errors.Is(err, ErrGapsOpen) || issues.Issues[0].Code != site.CodeBookingCalendarDisconnected || issues.Issues[0].Detail != "agenda" {
		t.Fatalf("err = %v", err)
	}
	r.calendars.err = errors.New("google auth: timeout")
	if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, ErrFormsUnavailable) {
		t.Fatalf("an unknown calendar state must refuse: %v", err)
	}
}

func TestAnUnconfiguredBookingIsAPublishGap(t *testing.T) {
	r := bookingRig(t)
	r.projects.snap.Settings.Bookings = nil
	_, err := r.svc.Publish(context.Background(), actor, projectID)
	var issues *IssuesFailure
	if !errors.As(err, &issues) || !strings.Contains(issuesText(issues), site.CodeBookingUnconfigured) {
		t.Fatalf("err = %v", err)
	}
	if len(r.calendars.asked) != 0 {
		t.Fatal("the calendar was read for a page that cannot book")
	}
}

func issuesText(f *IssuesFailure) string {
	var out []string
	for _, i := range f.Issues {
		out = append(out, i.Code)
	}
	return strings.Join(out, " ")
}
