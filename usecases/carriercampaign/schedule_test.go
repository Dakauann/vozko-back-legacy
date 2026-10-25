package carriercampaign_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/campaign"
	"vozko/domain/campaign/scheduledstart"
	cc "vozko/domain/carriercampaign"
	"vozko/domain/carriermessaging"
	"vozko/domain/notification"
)

func scheduledCampaign(id string, at time.Time) cc.Campaign {
	c := rcsCampaign(id)
	c.Status = cc.StatusScheduled
	c.ScheduledAt = &at
	return c
}

func TestRunSchedules(t *testing.T) {
	h := newHarness(t)
	h.addLead("lead-a", 1)
	due := testNow.Add(-time.Minute)
	h.campaigns.put(scheduledCampaign("c1", due))
	h.campaigns.put(scheduledCampaign("c2", testNow.Add(time.Hour)))

	report, err := h.svc.RunSchedules(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Started) != 1 || report.Started[0] != "c1" || len(report.Failed) != 0 {
		t.Fatalf("report %+v", report)
	}
	if h.campaigns.get("c1").Status != cc.StatusRunning || h.campaigns.get("c2").Status != cc.StatusScheduled {
		t.Fatal("only the due campaign starts")
	}
	runs := h.runs.all()
	if len(runs) != 1 || !runs[0].Occurrence.Equal(due) || runs[0].Status != campaign.StatusRunning {
		t.Fatalf("runs %+v", runs)
	}
	if len(h.enqueuer.messages) != 1 {
		t.Fatalf("enqueued %d", len(h.enqueuer.messages))
	}
	if h.selections.scopes[0].ActorID != owner || h.selections.scopes[0].WorkspaceID != wsID {
		t.Fatalf("scope %+v", h.selections.scopes[0])
	}
}

func TestRunSchedulesFailureFallsBackToDraft(t *testing.T) {
	h := newHarness(t)
	h.addLead("lead-a", 1)
	h.balance.micros = 0
	h.campaigns.put(scheduledCampaign("c1", testNow.Add(-time.Minute)))

	report, err := h.svc.RunSchedules(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Started) != 0 || !errors.Is(report.Failed["c1"], carriermessaging.ErrInsufficientBalance) {
		t.Fatalf("report %+v", report)
	}
	if stored := h.campaigns.get("c1"); stored.Status != cc.StatusDraft || stored.ScheduledAt != nil {
		t.Fatalf("stored %+v", stored)
	}
	if len(h.runs.all()) != 0 {
		t.Fatal("no run on failure")
	}
}

func TestRunSchedulesEmptyRecurringOccurrenceMovesOn(t *testing.T) {
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	h := newHarness(t)
	due := time.Date(2026, 10, 21, 9, 0, 0, 0, sp)
	c := scheduledCampaign("c1", due)
	c.Recurrence = &cc.Recurrence{Frequency: cc.FrequencyDaily, Time: "09:00", Timezone: "America/Sao_Paulo"}
	h.campaigns.put(c)

	report, err := h.svc.RunSchedules(context.Background(), 10)
	if err != nil || len(report.Started) != 1 {
		t.Fatalf("report %+v err %v", report, err)
	}
	stored := h.campaigns.get("c1")
	if stored.Status != cc.StatusScheduled || stored.ScheduledAt == nil || !stored.ScheduledAt.Equal(time.Date(2026, 10, 22, 9, 0, 0, 0, sp)) {
		t.Fatalf("stored %+v", stored)
	}
	runs := h.runs.all()
	if len(runs) != 1 || runs[0].Status != campaign.StatusCompleted {
		t.Fatalf("runs %+v", runs)
	}
}

func TestRunSchedulesSkipsUnclaimed(t *testing.T) {
	h := newHarness(t)
	h.addLead("lead-a", 1)
	c := scheduledCampaign("c1", testNow.Add(-time.Minute))
	h.campaigns.put(c)
	if _, err := h.campaigns.ClaimScheduledStart("c1", testNow); err != nil {
		t.Fatal(err)
	}
	c.ScheduledAt = ptr(testNow.Add(-time.Minute))
	report, err := h.svc.startDue(context.Background(), []*cc.Campaign{&c})
	if err != nil || len(report.Started) != 0 || len(report.Failed) != 0 {
		t.Fatalf("report %+v err %v", report, err)
	}
}

func TestAFailedScheduleIsRecordedAndToldToTheCreator(t *testing.T) {
	h := newHarness(t)
	h.addLead("lead-a", 1)
	h.balance.micros = 0
	due := testNow.Add(-time.Minute)
	h.campaigns.put(scheduledCampaign("c1", due))

	if _, err := h.svc.RunSchedules(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if got := h.campaigns.get("c1").ScheduleFailure; got != scheduledstart.FailureStartFailed {
		t.Fatalf("failure = %q", got)
	}
	if len(h.notices.drafts) != 1 {
		t.Fatalf("notices %+v", h.notices.drafts)
	}
	d := h.notices.drafts[0]
	if d.Audience.Kind != notification.AudienceUsers || d.Audience.UserIDs[0] != owner || d.ActionURL != "/dashboard/rcs-sms/campaigns/c1" || d.WorkspaceID != wsID {
		t.Fatalf("notice %+v", d)
	}
}

func TestAFailedScheduleThatCannotBeRecordedIsReturned(t *testing.T) {
	h := newHarness(t)
	h.addLead("lead-a", 1)
	h.balance.micros = 0
	h.campaigns.recordErr = errBoom
	h.campaigns.put(scheduledCampaign("c1", testNow.Add(-time.Minute)))

	if _, err := h.svc.RunSchedules(context.Background(), 10); !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
}

func TestAStartedScheduleIsNotAFailure(t *testing.T) {
	h := newHarness(t)
	h.addLead("lead-a", 1)
	h.campaigns.put(scheduledCampaign("c1", testNow.Add(-time.Minute)))

	if _, err := h.svc.RunSchedules(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if h.campaigns.get("c1").ScheduleFailure != "" || len(h.notices.drafts) != 0 {
		t.Fatal("a started schedule was reported as failed")
	}
}

func TestALaunchClearsTheFailedSchedule(t *testing.T) {
	h := newHarness(t)
	h.addLead("lead-a", 1)
	c := rcsCampaign("c1")
	c.ScheduleFailure = scheduledstart.FailureStartFailed
	h.campaigns.put(c)

	if _, err := h.svc.Launch(context.Background(), wsID, "c1", Actor{UserID: owner}); err != nil {
		t.Fatal(err)
	}
	if got := h.campaigns.get("c1").ScheduleFailure; got != "" {
		t.Fatalf("failure = %q", got)
	}
}

func TestTheCarrierServiceNeedsNotifications(t *testing.T) {
	h := newHarness(t)
	deps := h.svc.deps
	deps.Notifications = nil
	if _, err := New(deps); !errors.Is(err, ErrDependencyMissing) {
		t.Fatalf("got %v", err)
	}
}
