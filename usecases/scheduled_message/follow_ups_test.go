package scheduled_message_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	sm "vozko/domain/scheduled_message"
	"vozko/domain/shared"
)

type followUpFixture struct {
	repo      *fakeRepo
	windows   *fakeWindows
	templates *fakeTemplates
	wake      *fakeWake
	gate      *fakeGate
	zones     fakeZones
	clock     *fixedClock
	uc        sm.FollowUpUseCase
}

func newFollowUpFixture(t *testing.T) *followUpFixture {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("zone: %v", err)
	}
	expires := fixedNow.Add(20 * time.Hour)
	f := &followUpFixture{
		repo:      newFakeRepo(),
		windows:   &fakeWindows{open: true, expiresAt: &expires},
		templates: &fakeTemplates{},
		wake:      &fakeWake{},
		gate:      &fakeGate{},
		zones:     fakeZones{loc: loc},
		clock:     &fixedClock{now: fixedNow},
	}
	f.build(t)
	return f
}

func (f *followUpFixture) build(t *testing.T) {
	t.Helper()
	schedule, err := NewScheduleUseCase(f.repo, f.windows, f.templates, f.wake, f.clock)
	if err != nil {
		t.Fatalf("NewScheduleUseCase: %v", err)
	}
	f.uc, err = NewFollowUps(FollowUpsDeps{
		Repo: f.repo, Schedule: schedule, Gate: f.gate, Zones: f.zones, Windows: f.windows, Clock: f.clock,
	})
	if err != nil {
		t.Fatalf("NewFollowUps: %v", err)
	}
}

func receiptReminder() sm.FollowUpInput {
	return sm.FollowUpInput{
		WorkspaceID:   "ws-1",
		EntryID:       "entry-1",
		EntryType:     string(shared.EntryTypeWhatsApp),
		Author:        "ai:agent-1",
		Text:          "Oi, conseguiu enviar o comprovante?",
		When:          sm.FollowUpWhen{InMinutes: 240},
		CancelOnReply: true,
	}
}

func TestAnAgentSchedulesAFollowUpForLater(t *testing.T) {
	f := newFollowUpFixture(t)

	got, err := f.uc.Schedule(context.Background(), receiptReminder())
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	m := got.Message
	if m.CreatedByAutomation != "ai:agent-1" || m.CreatedByUserID != "" || !m.CancelOnReply {
		t.Fatalf("message = %+v", m)
	}
	if m.Kind != sm.KindText || m.Text != "Oi, conseguiu enviar o comprovante?" {
		t.Fatalf("kind %q text %q", m.Kind, m.Text)
	}
	if !m.ScheduledAt.Equal(fixedNow.Add(4 * time.Hour)) {
		t.Fatalf("scheduled at %s", m.ScheduledAt)
	}
	if got.Local.Location().String() != "America/Sao_Paulo" {
		t.Fatalf("local time in %s, want the workspace zone", got.Local.Location())
	}
	if f.repo.capped != 1 {
		t.Fatalf("an automation follow-up must be stored under the pending cap")
	}
	if f.wake.count() != 1 {
		t.Fatalf("the follow-up was not enqueued")
	}
}

func TestTomorrowAtTenIsTheWorkspacesTen(t *testing.T) {
	f := newFollowUpFixture(t)
	f.windows.set(true, nil)
	in := receiptReminder()
	tomorrow := 1
	in.When = sm.FollowUpWhen{DaysFromToday: &tomorrow, Time: "10:00"}

	got, err := f.uc.Schedule(context.Background(), in)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if want := time.Date(2026, 8, 13, 13, 0, 0, 0, time.UTC); !got.Message.ScheduledAt.Equal(want) {
		t.Fatalf("scheduled at %s, want %s (10:00 in Sao Paulo)", got.Message.ScheduledAt, want)
	}
}

func TestAFollowUpPastTheWindowGoesOutAsTheFallbackTemplate(t *testing.T) {
	f := newFollowUpFixture(t)
	in := receiptReminder()
	in.When = sm.FollowUpWhen{InMinutes: 30 * 60}
	in.Fallback = &sm.TemplateContent{ID: "tpl-1", BodyParams: []string{"Ana"}}

	got, err := f.uc.Schedule(context.Background(), in)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if got.Message.Kind != sm.KindTemplate || got.Message.Template == nil || got.Message.Template.ID != "tpl-1" || got.Message.Text != "" {
		t.Fatalf("message = %+v", got.Message)
	}
	if len(f.templates.checks) != 1 {
		t.Fatalf("the template send rules were not checked at schedule time")
	}
}

func TestAFollowUpPastTheWindowWithoutATemplateIsRefused(t *testing.T) {
	f := newFollowUpFixture(t)
	in := receiptReminder()
	in.When = sm.FollowUpWhen{InMinutes: 30 * 60}

	if _, err := f.uc.Schedule(context.Background(), in); !errors.Is(err, sm.ErrScheduledAtPastWindow) {
		t.Fatalf("err = %v, want ErrScheduledAtPastWindow", err)
	}
	if len(f.repo.messages) != 0 {
		t.Fatal("a refused follow-up was stored")
	}
}

func TestAFollowUpTooSoonOrTooFarIsRefused(t *testing.T) {
	f := newFollowUpFixture(t)
	f.windows.set(true, nil)
	soon := receiptReminder()
	soon.When = sm.FollowUpWhen{Time: "08:00"}
	if _, err := f.uc.Schedule(context.Background(), soon); !errors.Is(err, sm.ErrScheduledAtTooSoon) {
		t.Fatalf("err = %v, want ErrScheduledAtTooSoon for a time already gone today", err)
	}
	missing := receiptReminder()
	missing.When = sm.FollowUpWhen{}
	if _, err := f.uc.Schedule(context.Background(), missing); !errors.Is(err, sm.ErrFollowUpWhenRequired) {
		t.Fatalf("err = %v, want ErrFollowUpWhenRequired", err)
	}
	far := receiptReminder()
	far.When = sm.FollowUpWhen{InMinutes: int(sm.MaxScheduleHorizon/time.Minute) + 60}
	if _, err := f.uc.Schedule(context.Background(), far); !errors.Is(err, sm.ErrScheduledAtTooFar) {
		t.Fatalf("err = %v, want ErrScheduledAtTooFar", err)
	}
}

func TestAConversationKeepsAtMostThreePendingFollowUps(t *testing.T) {
	f := newFollowUpFixture(t)
	for i := 0; i < sm.MaxPendingFollowUps; i++ {
		if _, err := f.uc.Schedule(context.Background(), receiptReminder()); err != nil {
			t.Fatalf("follow-up %d: %v", i+1, err)
		}
	}
	if _, err := f.uc.Schedule(context.Background(), receiptReminder()); !errors.Is(err, sm.ErrFollowUpLimitReached) {
		t.Fatalf("err = %v, want ErrFollowUpLimitReached", err)
	}
	if got := sm.FollowUpErrorCode(sm.ErrFollowUpLimitReached); got != "follow_up_limit_reached" {
		t.Fatalf("code = %q", got)
	}
}

func TestAutomationCannotScheduleOnAPersonsConversation(t *testing.T) {
	f := newFollowUpFixture(t)
	f.gate.refuse = true

	if _, err := f.uc.Schedule(context.Background(), receiptReminder()); !errors.Is(err, sm.ErrFollowUpNotAllowed) {
		t.Fatalf("err = %v, want ErrFollowUpNotAllowed", err)
	}
	if len(f.repo.messages) != 0 {
		t.Fatal("a follow-up was stored on a conversation automation may not touch")
	}
}

func TestAnUnreadableOwnerOrZoneSchedulesNothing(t *testing.T) {
	f := newFollowUpFixture(t)
	f.gate.err = errors.New("owners table unreachable")
	if _, err := f.uc.Schedule(context.Background(), receiptReminder()); err == nil {
		t.Fatal("an unreadable owner must refuse")
	}

	g := newFollowUpFixture(t)
	g.zones = fakeZones{err: errors.New("config unreachable")}
	g.build(t)
	if _, err := g.uc.Schedule(context.Background(), receiptReminder()); err == nil {
		t.Fatal("an unreadable zone must refuse")
	}
	if len(f.repo.messages)+len(g.repo.messages) != 0 {
		t.Fatal("something was stored without the checks")
	}
}

func TestARetriedToolCallDoesNotScheduleTwice(t *testing.T) {
	f := newFollowUpFixture(t)
	in := receiptReminder()
	in.IdempotencyKey = "follow-up:call-1"

	first, err := f.uc.Schedule(context.Background(), in)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := f.uc.Schedule(context.Background(), in)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !second.Replayed || second.Message.ID != first.Message.ID || len(f.repo.messages) != 1 {
		t.Fatalf("replay = %+v, stored %d", second, len(f.repo.messages))
	}
}

func TestOnlyAnAgentOrAWorkflowSchedulesFollowUps(t *testing.T) {
	f := newFollowUpFixture(t)
	in := receiptReminder()
	in.Author = "user-1"
	if _, err := f.uc.Schedule(context.Background(), in); !errors.Is(err, sm.ErrAutomationAuthorInvalid) {
		t.Fatalf("err = %v, want ErrAutomationAuthorInvalid", err)
	}
}

func (f *followUpFixture) stored(id, author string, status sm.Status) *sm.ScheduledMessage {
	m := &sm.ScheduledMessage{
		ID: id, WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: shared.EntryTypeWhatsApp,
		Kind: sm.KindText, Text: "oi", ScheduledAt: fixedNow.Add(time.Hour), Status: status,
	}
	if author == "person" {
		m.CreatedByUserID = "user-1"
	} else {
		m.CreatedByAutomation = author
	}
	return f.repo.put(m)
}

func cancelOf(author, id string) sm.FollowUpCancel {
	return sm.FollowUpCancel{WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: string(shared.EntryTypeWhatsApp), Author: author, ID: id}
}

func TestAnAgentCancelsItsFollowUpByID(t *testing.T) {
	f := newFollowUpFixture(t)
	f.stored("f-1", "ai:agent-1", sm.StatusPending)

	got, err := f.uc.Cancel(context.Background(), cancelOf("ai:agent-1", "f-1"))
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if len(got) != 1 || got[0].ID != "f-1" {
		t.Fatalf("cancelled = %+v", got)
	}
	stored := f.repo.get("f-1")
	if stored.Status != sm.StatusCanceled || stored.CancelReason == nil || *stored.CancelReason != sm.CancelByAutomation {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestAnAgentCannotCancelWhatItDidNotSchedule(t *testing.T) {
	f := newFollowUpFixture(t)
	f.stored("other-agent", "ai:agent-2", sm.StatusPending)
	f.stored("operator", "person", sm.StatusPending)
	elsewhere := f.stored("elsewhere", "ai:agent-1", sm.StatusPending)
	elsewhere.EntryID = "entry-2"

	for _, id := range []string{"other-agent", "operator", "elsewhere", "missing"} {
		if _, err := f.uc.Cancel(context.Background(), cancelOf("ai:agent-1", id)); !errors.Is(err, sm.ErrNotFound) {
			t.Fatalf("%s: err = %v, want ErrNotFound", id, err)
		}
	}
	for _, id := range []string{"other-agent", "operator", "elsewhere"} {
		if f.repo.get(id).Status != sm.StatusPending {
			t.Fatalf("%s was cancelled", id)
		}
	}
}

func TestASentFollowUpCannotBeCancelled(t *testing.T) {
	f := newFollowUpFixture(t)
	f.stored("f-1", "ai:agent-1", sm.StatusSent)
	if _, err := f.uc.Cancel(context.Background(), cancelOf("ai:agent-1", "f-1")); !errors.Is(err, sm.ErrNotPending) {
		t.Fatalf("err = %v, want ErrNotPending", err)
	}
}

func TestCancelAllTakesOnlyThisAuthorsPendingFollowUps(t *testing.T) {
	f := newFollowUpFixture(t)
	f.stored("mine-1", "ai:agent-1", sm.StatusPending)
	f.stored("mine-2", "ai:agent-1", sm.StatusPending)
	f.stored("mine-sent", "ai:agent-1", sm.StatusSent)
	f.stored("workflow", "workflow:wf-1", sm.StatusPending)
	f.stored("operator", "person", sm.StatusPending)

	got, err := f.uc.Cancel(context.Background(), cancelOf("ai:agent-1", ""))
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("cancelled %d, want 2", len(got))
	}
	for id, want := range map[string]sm.Status{"mine-1": sm.StatusCanceled, "mine-2": sm.StatusCanceled, "mine-sent": sm.StatusSent, "workflow": sm.StatusPending, "operator": sm.StatusPending} {
		if got := f.repo.get(id).Status; got != want {
			t.Errorf("%s: %s, want %s", id, got, want)
		}
	}
}

func TestNewFollowUpsNamesEveryMissingDependency(t *testing.T) {
	if _, err := NewFollowUps(FollowUpsDeps{}); err == nil {
		t.Fatal("missing dependencies were accepted")
	}
}
