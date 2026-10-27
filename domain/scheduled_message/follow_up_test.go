package scheduled_message

import (
	"errors"
	"testing"
	"time"

	"vozko/domain/shared"
)

func saoPaulo(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	return loc
}

func days(n int) *int { return &n }

func TestAFollowUpTimeIsReadInTheWorkspaceZone(t *testing.T) {
	loc := saoPaulo(t)
	evening := time.Date(2026, 10, 26, 22, 30, 0, 0, time.UTC)

	cases := []struct {
		name string
		when FollowUpWhen
		want time.Time
	}{
		{name: "minutes from now", when: FollowUpWhen{InMinutes: 240}, want: evening.Add(4 * time.Hour)},
		{name: "tomorrow at ten", when: FollowUpWhen{DaysFromToday: days(1), Time: "10:00"}, want: time.Date(2026, 10, 27, 10, 0, 0, 0, loc)},
		{name: "today later", when: FollowUpWhen{Time: "21:00"}, want: time.Date(2026, 10, 26, 21, 0, 0, 0, loc)},
		{name: "a calendar date", when: FollowUpWhen{Date: "2026-10-30", Time: "09:15"}, want: time.Date(2026, 10, 30, 9, 15, 0, 0, loc)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.when.Resolve(evening, loc)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("at = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestTodayIsTheWorkspaceDayNotTheServerDay(t *testing.T) {
	loc := saoPaulo(t)
	utcAlreadyTomorrow := time.Date(2026, 10, 27, 1, 0, 0, 0, time.UTC)

	got, err := FollowUpWhen{DaysFromToday: days(1), Time: "10:00"}.Resolve(utcAlreadyTomorrow, loc)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := time.Date(2026, 10, 27, 10, 0, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("at = %s, want %s: in Sao Paulo it is still the 26th", got, want)
	}
}

func TestAnUnclearFollowUpTimeIsRefused(t *testing.T) {
	loc := saoPaulo(t)
	cases := []struct {
		name string
		when FollowUpWhen
		want error
	}{
		{name: "nothing given", when: FollowUpWhen{}, want: ErrFollowUpWhenRequired},
		{name: "a day without a time", when: FollowUpWhen{DaysFromToday: days(1)}, want: ErrFollowUpWhenRequired},
		{name: "minutes and a clock time", when: FollowUpWhen{InMinutes: 30, Time: "10:00"}, want: ErrFollowUpWhenAmbiguous},
		{name: "a relative and a calendar day", when: FollowUpWhen{DaysFromToday: days(1), Date: "2026-10-30", Time: "10:00"}, want: ErrFollowUpWhenAmbiguous},
		{name: "negative minutes", when: FollowUpWhen{InMinutes: -5}, want: ErrFollowUpWhenInvalid},
		{name: "a day in the past", when: FollowUpWhen{DaysFromToday: days(-1), Time: "10:00"}, want: ErrFollowUpWhenInvalid},
		{name: "a clock time that does not exist", when: FollowUpWhen{Time: "25:10"}, want: ErrFollowUpWhenInvalid},
		{name: "a date that does not parse", when: FollowUpWhen{Date: "30/10/2026", Time: "10:00"}, want: ErrFollowUpWhenInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.when.Resolve(now, loc); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAFollowUpNeedsTheWorkspaceZone(t *testing.T) {
	if _, err := (FollowUpWhen{InMinutes: 10}).Resolve(now, nil); !errors.Is(err, ErrFollowUpZoneRequired) {
		t.Fatalf("err = %v, want ErrFollowUpZoneRequired", err)
	}
}

func TestAFollowUpInsideTheWindowIsText(t *testing.T) {
	expires := now.Add(20 * time.Hour)
	kind, err := FollowUpKind(now.Add(4*time.Hour), true, &expires, now, shared.EntryTypeWhatsApp, true)
	if err != nil || kind != KindText {
		t.Fatalf("kind = %q, err = %v, want text", kind, err)
	}
}

func TestAFollowUpPastTheWindowUsesTheFallbackTemplate(t *testing.T) {
	expires := now.Add(20 * time.Hour)
	kind, err := FollowUpKind(now.Add(30*time.Hour), true, &expires, now, shared.EntryTypeWhatsApp, true)
	if err != nil || kind != KindTemplate {
		t.Fatalf("kind = %q, err = %v, want template", kind, err)
	}
	kind, err = FollowUpKind(now.Add(30*time.Hour), false, nil, now, shared.EntryTypeWhatsApp, true)
	if err != nil || kind != KindTemplate {
		t.Fatalf("closed window: kind = %q, err = %v, want template", kind, err)
	}
}

func TestAFollowUpPastTheWindowWithoutATemplateIsRefused(t *testing.T) {
	expires := now.Add(20 * time.Hour)
	if _, err := FollowUpKind(now.Add(30*time.Hour), true, &expires, now, shared.EntryTypeWhatsApp, false); !errors.Is(err, ErrScheduledAtPastWindow) {
		t.Fatalf("err = %v, want ErrScheduledAtPastWindow", err)
	}
	if _, err := FollowUpKind(now.Add(30*time.Hour), true, &expires, now, shared.EntryTypeInstagram, true); !errors.Is(err, ErrScheduledAtPastWindow) {
		t.Fatalf("a channel without templates: err = %v, want ErrScheduledAtPastWindow", err)
	}
}

func TestAFollowUpKeepsTheScheduleLimits(t *testing.T) {
	if _, err := FollowUpKind(now.Add(10*time.Second), true, nil, now, shared.EntryTypeWhatsApp, true); !errors.Is(err, ErrScheduledAtTooSoon) {
		t.Fatalf("err = %v, want ErrScheduledAtTooSoon", err)
	}
	if _, err := FollowUpKind(now.Add(MaxScheduleHorizon+time.Hour), false, nil, now, shared.EntryTypeWhatsApp, true); !errors.Is(err, ErrScheduledAtTooFar) {
		t.Fatalf("err = %v, want ErrScheduledAtTooFar", err)
	}
}

func automationMessage(author string) *ScheduledMessage {
	m := &ScheduledMessage{
		WorkspaceID:         "ws-1",
		EntryID:             "entry-1",
		EntryType:           shared.EntryTypeWhatsApp,
		CreatedByAutomation: author,
		Text:                "Oi, conseguiu enviar o comprovante?",
		ScheduledAt:         now.Add(time.Hour),
	}
	m.Normalize()
	return m
}

func TestAnAgentOrAWorkflowCanAuthorAScheduledMessage(t *testing.T) {
	for _, author := range []string{"ai:agent-1", "workflow:wf-1"} {
		m := automationMessage(author)
		if err := m.Validate(); err != nil {
			t.Fatalf("%s: %v", author, err)
		}
		if !m.ByAutomation() || m.Author() != author {
			t.Fatalf("%s: by automation %v, author %q", author, m.ByAutomation(), m.Author())
		}
		if limit, capped := m.PendingLimit(); !capped || limit != MaxPendingFollowUps {
			t.Fatalf("%s: limit %d capped %v", author, limit, capped)
		}
	}
}

func TestAScheduledMessageHasExactlyOneAuthor(t *testing.T) {
	both := automationMessage("ai:agent-1")
	both.CreatedByUserID = "user-1"
	if err := both.Validate(); !errors.Is(err, ErrAuthorAmbiguous) {
		t.Fatalf("err = %v, want ErrAuthorAmbiguous", err)
	}
	for _, author := range []string{"ai:", "workflow:", "campaign:c-1", "system"} {
		if err := automationMessage(author).Validate(); !errors.Is(err, ErrAutomationAuthorInvalid) {
			t.Fatalf("%q: err = %v, want ErrAutomationAuthorInvalid", author, err)
		}
	}
	person := &ScheduledMessage{WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: shared.EntryTypeWhatsApp, CreatedByUserID: "user-1", Text: "oi"}
	person.Normalize()
	if limit, capped := person.PendingLimit(); capped || limit != 0 {
		t.Fatalf("a person's message is not capped, got %d %v", limit, capped)
	}
}

func TestOnlyAPendingOrClaimedMessageCanBeCancelled(t *testing.T) {
	if !StatusSending.CanTransitionTo(StatusCanceled) {
		t.Fatal("a claimed follow-up must be cancellable when the automation stood down")
	}
	if StatusSent.CanTransitionTo(StatusCanceled) || StatusFailed.CanTransitionTo(StatusCanceled) {
		t.Fatal("a finished message cannot be cancelled")
	}
}

func TestEveryFollowUpRefusalHasACode(t *testing.T) {
	cases := map[error]string{
		ErrFollowUpWhenRequired:      "follow_up_when_required",
		ErrFollowUpWhenAmbiguous:     "follow_up_when_ambiguous",
		ErrFollowUpWhenInvalid:       "follow_up_when_invalid",
		ErrScheduledAtTooSoon:        "follow_up_too_soon",
		ErrScheduledAtTooFar:         "follow_up_too_far",
		ErrScheduledAtPastWindow:     "follow_up_past_window",
		ErrWindowClosed:              "follow_up_window_closed",
		ErrFollowUpLimitReached:      "follow_up_limit_reached",
		ErrFollowUpNotAllowed:        "follow_up_not_allowed",
		ErrFollowUpNeedsConversation: "follow_up_needs_conversation",
		ErrFollowUpVariableMissing:   "follow_up_variable_missing",
		ErrNotFound:                  "follow_up_not_found",
		ErrNotPending:                "follow_up_not_pending",
		ErrContentRequired:           "follow_up_text_required",
		ErrTemplatesUnsupported:      "follow_up_template_unavailable",
		errors.New("database down"):  "follow_up_unavailable",
	}
	for err, want := range cases {
		if got := FollowUpErrorCode(err); got != want {
			t.Errorf("%v: code %q, want %q", err, got, want)
		}
	}
	for _, code := range FollowUpErrorCodes() {
		if code == "" {
			t.Fatal("an empty code")
		}
	}
}
