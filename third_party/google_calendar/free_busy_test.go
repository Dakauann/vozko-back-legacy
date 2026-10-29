package google_calendar

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func serviceAnswering(t *testing.T, status int, body string, seen *map[string]any) *Service {
	t.Helper()
	return &Service{httpClient: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != calendarAPIURL+"/freeBusy" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("request %s %s %q", r.Method, r.URL, r.Header.Get("Authorization"))
		}
		raw, _ := io.ReadAll(r.Body)
		if seen != nil {
			_ = json.Unmarshal(raw, seen)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}}
}

func TestFreeBusyReturnsOnlyBusyIntervalsOfThePrimaryCalendar(t *testing.T) {
	from := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	to := from.Add(48 * time.Hour)
	var sent map[string]any
	s := serviceAnswering(t, http.StatusOK, `{"calendars":{"primary":{"busy":[{"start":"2026-10-28T12:00:00Z","end":"2026-10-28T13:00:00Z"}]}}}`, &sent)
	busy, err := s.FreeBusy("tok", from, to)
	if err != nil || len(busy) != 1 || !busy[0].Start.Equal(from.Add(12*time.Hour)) || !busy[0].End.Equal(from.Add(13*time.Hour)) {
		t.Fatalf("busy = %+v, %v", busy, err)
	}
	items, _ := sent["items"].([]any)
	if sent["timeMin"] != "2026-10-28T00:00:00Z" || sent["timeMax"] != "2026-10-30T00:00:00Z" || len(items) != 1 {
		t.Fatalf("sent %v", sent)
	}
}

func TestFreeBusyFailsClosedWhenGoogleCannotAnswerForTheCalendar(t *testing.T) {
	from := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	for name, reply := range map[string]struct {
		status int
		body   string
	}{
		"http error":     {http.StatusForbidden, `{"error":{"message":"no"}}`},
		"calendar error": {http.StatusOK, `{"calendars":{"primary":{"errors":[{"reason":"backendError"}],"busy":[]}}}`},
		"missing":        {http.StatusOK, `{"calendars":{}}`},
		"bad time":       {http.StatusOK, `{"calendars":{"primary":{"busy":[{"start":"amanhã","end":"2026-10-28T13:00:00Z"}]}}}`},
	} {
		if _, err := serviceAnswering(t, reply.status, reply.body, nil).FreeBusy("tok", from, from.Add(time.Hour)); err == nil {
			t.Fatalf("%s: busy time was read as free", name)
		}
	}
}
