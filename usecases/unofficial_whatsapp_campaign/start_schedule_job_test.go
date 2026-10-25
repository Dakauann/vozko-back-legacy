package unofficial_whatsapp_campaign

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/campaign"
	"vozko/domain/campaign/launchcheck"
	"vozko/domain/campaign/scheduledstart"
	"vozko/domain/notification"
	uw "vozko/domain/unofficial_whatsapp"
	uwc "vozko/domain/unofficial_whatsapp_campaign"
	"vozko/domain/workspace"
	"vozko/usecases/campaignschedule"
)

type dueUnofficial struct {
	uwc.Repository
	due []*uwc.Campaign
}

func (d dueUnofficial) ListScheduledToStart(time.Time, int) ([]*uwc.Campaign, error) {
	return d.due, nil
}

type unofficialSchedules struct {
	claimable map[string]bool
	failures  map[string]scheduledstart.Failure
	recordErr error
}

func (s *unofficialSchedules) ClaimScheduledStart(campaignID string, _ time.Time) (bool, error) {
	if !s.claimable[campaignID] {
		return false, nil
	}
	s.claimable[campaignID] = false
	return true, nil
}

func (s *unofficialSchedules) RecordScheduleFailure(_ context.Context, campaignID string, failure scheduledstart.Failure) error {
	if s.recordErr != nil {
		return s.recordErr
	}
	if s.failures == nil {
		s.failures = map[string]scheduledstart.Failure{}
	}
	s.failures[campaignID] = failure
	return nil
}

type scheduledDispatch struct {
	inputs []uwc.DispatchCampaignInput
	fail   map[string]error
}

func (d *scheduledDispatch) Dispatch(_ context.Context, in uwc.DispatchCampaignInput) error {
	d.inputs = append(d.inputs, in)
	return d.fail[in.CampaignID]
}

type unofficialNotices struct{ drafts []notification.Draft }

func (n *unofficialNotices) Publish(_ context.Context, d notification.Draft) (notification.Record, error) {
	n.drafts = append(n.drafts, d)
	return notification.Record{}, nil
}

func unofficialReporter(t *testing.T, schedules *unofficialSchedules, box *unofficialNotices) *campaignschedule.Reporter {
	t.Helper()
	r, err := campaignschedule.New(campaignschedule.Deps{
		Channel:       campaignschedule.Channel{Key: "unofficial_whatsapp", Label: "Campanha não oficial", Path: "/dashboard/unofficial-whatsapp-campaigns/", Resource: workspace.ResourceUnofficialWhatsAppCampaigns},
		Failures:      schedules,
		Notifications: box,
		Logf:          func(string, ...any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAFailedUnofficialScheduleIsRecordedAndToldToWhoCreatedIt(t *testing.T) {
	schedules := &unofficialSchedules{claimable: map[string]bool{"a": true, "b": true}}
	box := &unofficialNotices{}
	blocked := &launchcheck.RefusedError{Report: launchcheck.Blocked(launchcheck.CodeNumberUnusable, 0)}
	dispatch := &scheduledDispatch{fail: map[string]error{"a": blocked}}
	due := []*uwc.Campaign{
		{ID: "a", WorkspaceID: "ws", Name: "Natal", CreatedByID: "u1", ScheduledStart: time.Now().Add(-time.Minute)},
		{ID: "b", WorkspaceID: "ws", Name: "Ano novo", ScheduledStart: time.Now().Add(-time.Minute)},
	}
	job := NewScheduleStartJob(dueUnofficial{due: due}, dispatch, schedules, unofficialReporter(t, schedules, box))

	if err := job.StartScheduledCampaigns(); err != nil {
		t.Fatalf("a refused start is reported, not a job error: %v", err)
	}

	if len(dispatch.inputs) != 2 || dispatch.inputs[0].Action != campaign.ActionStart || dispatch.inputs[0].Reviewed {
		t.Fatalf("dispatched %+v", dispatch.inputs)
	}
	if schedules.failures["a"] != scheduledstart.FailureBlocked || len(schedules.failures) != 1 {
		t.Fatalf("failures %v", schedules.failures)
	}
	if len(box.drafts) != 1 || box.drafts[0].Audience.UserIDs[0] != "u1" || box.drafts[0].ActionURL != "/dashboard/unofficial-whatsapp-campaigns/a" {
		t.Fatalf("notices %+v", box.drafts)
	}
}

func TestAnUnofficialCampaignAlreadyRunningIsNotAFailedSchedule(t *testing.T) {
	schedules := &unofficialSchedules{claimable: map[string]bool{"a": true}}
	box := &unofficialNotices{}
	dispatch := &scheduledDispatch{fail: map[string]error{"a": campaign.ErrAlreadyRunning}}
	job := NewScheduleStartJob(dueUnofficial{due: []*uwc.Campaign{{ID: "a"}}}, dispatch, schedules, unofficialReporter(t, schedules, box))

	if err := job.StartScheduledCampaigns(); err != nil {
		t.Fatal(err)
	}
	if len(schedules.failures) != 0 || len(box.drafts) != 0 {
		t.Fatal("reported a campaign that is already running")
	}
}

func TestAnUnofficialFailureThatCannotBeRecordedIsAJobError(t *testing.T) {
	boom := errors.New("db down")
	schedules := &unofficialSchedules{claimable: map[string]bool{"a": true}, recordErr: boom}
	dispatch := &scheduledDispatch{fail: map[string]error{"a": errors.New("queue down")}}
	job := NewScheduleStartJob(dueUnofficial{due: []*uwc.Campaign{{ID: "a"}}}, dispatch, schedules, unofficialReporter(t, schedules, &unofficialNotices{}))

	if err := job.StartScheduledCampaigns(); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestWithoutAReporterNoUnofficialScheduleIsConsumed(t *testing.T) {
	schedules := &unofficialSchedules{claimable: map[string]bool{"a": true}}
	dispatch := &scheduledDispatch{}
	job := NewScheduleStartJob(dueUnofficial{due: []*uwc.Campaign{{ID: "a"}}}, dispatch, schedules, nil)

	if err := job.StartScheduledCampaigns(); !errors.Is(err, ErrScheduleStartUnavailable) {
		t.Fatalf("got %v", err)
	}
	if !schedules.claimable["a"] || len(dispatch.inputs) != 0 {
		t.Fatal("a schedule was consumed with no way to report its failure")
	}
}

func TestAManualUnofficialStartClearsTheFailedSchedule(t *testing.T) {
	schedules := &unofficialSchedules{}
	failed := campaignGetStub{"c1": {ID: "c1", WorkspaceID: "ws1", ScheduleFailure: scheduledstart.FailureBlocked}}
	uc := NewCampaignActionUseCase(NewCampaignAccessUseCase(failed), &dispatchRecorder{}, unofficialReporter(t, schedules, &unofficialNotices{}))

	if _, err := uc.Act(context.Background(), "ws1", uw.Unrestricted(), "c1", campaign.ActionStart); err != nil {
		t.Fatal(err)
	}
	if got, ok := schedules.failures["c1"]; !ok || got != "" {
		t.Fatalf("failures %v", schedules.failures)
	}
}

func TestPausingOrARefusedStartKeepsTheFailedSchedule(t *testing.T) {
	schedules := &unofficialSchedules{}
	failed := campaignGetStub{"c1": {ID: "c1", WorkspaceID: "ws1", ScheduleFailure: scheduledstart.FailureBlocked}}
	reporter := unofficialReporter(t, schedules, &unofficialNotices{})

	if _, err := NewCampaignActionUseCase(NewCampaignAccessUseCase(failed), &dispatchRecorder{}, reporter).Act(context.Background(), "ws1", uw.Unrestricted(), "c1", campaign.ActionStop); err != nil {
		t.Fatal(err)
	}
	refused := &dispatchRecorder{err: errors.New("number banned")}
	if _, err := NewCampaignActionUseCase(NewCampaignAccessUseCase(failed), refused, reporter).Act(context.Background(), "ws1", uw.Unrestricted(), "c1", campaign.ActionStart); err == nil {
		t.Fatal("the start should fail")
	}
	if len(schedules.failures) != 0 {
		t.Fatalf("failures %v", schedules.failures)
	}
}
