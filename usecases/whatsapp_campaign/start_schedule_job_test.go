package whatsapp_campaign_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/campaign"
	"vozko/domain/campaign/launchcheck"
	"vozko/domain/campaign/scheduledstart"
	"vozko/domain/notification"
	wc "vozko/domain/whatsapp_campaign"
	"vozko/domain/workspace"
	"vozko/usecases/campaignschedule"
)

type dueCampaigns struct {
	wc.Repository
	due []*wc.Campaign
}

func (d dueCampaigns) ListScheduledToStart(time.Time, int) ([]*wc.Campaign, error) {
	return d.due, nil
}

type scheduleLedger struct {
	claimable map[string]bool
	claimed   []string
	failures  map[string]scheduledstart.Failure
	recordErr error
}

func (s *scheduleLedger) ClaimScheduledStart(campaignID string, _ time.Time) (bool, error) {
	if !s.claimable[campaignID] {
		return false, nil
	}
	s.claimable[campaignID] = false
	s.claimed = append(s.claimed, campaignID)
	return true, nil
}

func (s *scheduleLedger) RecordScheduleFailure(_ context.Context, campaignID string, failure scheduledstart.Failure) error {
	if s.recordErr != nil {
		return s.recordErr
	}
	if s.failures == nil {
		s.failures = map[string]scheduledstart.Failure{}
	}
	s.failures[campaignID] = failure
	return nil
}

type recordingDispatch struct {
	started []wc.DispatchCampaignInput
	fail    map[string]error
}

func (r *recordingDispatch) Dispatch(input wc.DispatchCampaignInput) error {
	r.started = append(r.started, input)
	return r.fail[input.CampaignID]
}

type noticeBox struct{ drafts []notification.Draft }

func (n *noticeBox) Publish(_ context.Context, d notification.Draft) (notification.Record, error) {
	n.drafts = append(n.drafts, d)
	return notification.Record{}, nil
}

func scheduleReporter(t *testing.T, ledger *scheduleLedger, box *noticeBox) *campaignschedule.Reporter {
	t.Helper()
	r, err := campaignschedule.New(campaignschedule.Deps{
		Channel:       campaignschedule.Channel{Key: "whatsapp", Label: "Campanha oficial", Path: "/dashboard/whatsapp-campaigns/", Resource: workspace.ResourceWhatsAppCampaigns},
		Failures:      ledger,
		Notifications: box,
		Logf:          func(string, ...any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func dueAt() time.Time { return time.Now().Add(-time.Minute).UTC() }

func TestAScheduleFiresOnlyForTheCampaignsItClaims(t *testing.T) {
	ledger := &scheduleLedger{claimable: map[string]bool{"a": true, "b": false, "c": true}}
	dispatch := &recordingDispatch{}
	job := NewScheduleStartJob(dispatch, dueCampaigns{due: []*wc.Campaign{{ID: "a"}, {ID: "b"}, {ID: "c"}}}, ledger, scheduleReporter(t, ledger, &noticeBox{}))

	if err := job.StartScheduledWhatsappCampaigns(); err != nil {
		t.Fatal(err)
	}
	if err := job.StartScheduledWhatsappCampaigns(); err != nil {
		t.Fatal(err)
	}

	if len(dispatch.started) != 2 || dispatch.started[0].CampaignID != "a" || dispatch.started[1].CampaignID != "c" {
		t.Fatalf("started %v, want a and c once each", dispatch.started)
	}
}

func TestAScheduledStartCarriesTheConfirmationGivenWhenScheduling(t *testing.T) {
	ledger := &scheduleLedger{claimable: map[string]bool{"a": true}}
	dispatch := &recordingDispatch{}
	due := &wc.Campaign{ID: "a", ScheduledStart: dueAt(), ScheduleConfirmedFingerprint: "abc", ScheduledBy: "u1"}
	job := NewScheduleStartJob(dispatch, dueCampaigns{due: []*wc.Campaign{due}}, ledger, scheduleReporter(t, ledger, &noticeBox{}))

	if err := job.StartScheduledWhatsappCampaigns(); err != nil {
		t.Fatal(err)
	}

	got := dispatch.started[0]
	if got.Action != wc.CampaignActionStart || got.ConfirmedFingerprint != "abc" || got.Reviewed {
		t.Fatalf("dispatched %+v", got)
	}
}

func TestAFailedScheduledStartIsRecordedAndToldToWhoScheduledIt(t *testing.T) {
	ledger := &scheduleLedger{claimable: map[string]bool{"a": true, "b": true}}
	box := &noticeBox{}
	changed := &launchcheck.RefusedError{Report: noExit}
	dispatch := &recordingDispatch{fail: map[string]error{"a": changed}}
	due := []*wc.Campaign{
		{ID: "a", WorkspaceID: "ws", Name: "Natal", ScheduledStart: dueAt(), ScheduledBy: "u1"},
		{ID: "b", WorkspaceID: "ws", Name: "Ano novo", ScheduledStart: dueAt()},
	}
	job := NewScheduleStartJob(dispatch, dueCampaigns{due: due}, ledger, scheduleReporter(t, ledger, box))

	if err := job.StartScheduledWhatsappCampaigns(); err != nil {
		t.Fatalf("a refused start is reported, not a job error: %v", err)
	}

	if len(dispatch.started) != 2 {
		t.Fatalf("started %v, want b started too", dispatch.started)
	}
	if ledger.failures["a"] != scheduledstart.FailureUnconfirmed || len(ledger.failures) != 1 {
		t.Fatalf("failures %v", ledger.failures)
	}
	if len(box.drafts) != 1 || box.drafts[0].Audience.UserIDs[0] != "u1" || box.drafts[0].ActionURL != "/dashboard/whatsapp-campaigns/a" {
		t.Fatalf("notices %+v", box.drafts)
	}
}

func TestALeadSelectionScheduleIsRefusedNotBypassed(t *testing.T) {
	ledger := &scheduleLedger{claimable: map[string]bool{"a": true}}
	dispatch := &recordingDispatch{fail: map[string]error{"a": campaign.ErrSelectionStartNeedsReview}}
	job := NewScheduleStartJob(dispatch, dueCampaigns{due: []*wc.Campaign{{ID: "a", Source: campaign.SourceLeadSelection, ScheduledStart: dueAt()}}}, ledger, scheduleReporter(t, ledger, &noticeBox{}))

	if err := job.StartScheduledWhatsappCampaigns(); err != nil {
		t.Fatal(err)
	}

	if dispatch.started[0].Reviewed {
		t.Fatal("the job claimed a review nobody did")
	}
	if ledger.failures["a"] != scheduledstart.FailureNeedsReview {
		t.Fatalf("failures %v", ledger.failures)
	}
}

func TestAFailureThatCannotBeRecordedIsAJobError(t *testing.T) {
	boom := errors.New("db down")
	ledger := &scheduleLedger{claimable: map[string]bool{"a": true}, recordErr: boom}
	dispatch := &recordingDispatch{fail: map[string]error{"a": errors.New("queue down")}}
	job := NewScheduleStartJob(dispatch, dueCampaigns{due: []*wc.Campaign{{ID: "a", ScheduledStart: dueAt()}}}, ledger, scheduleReporter(t, ledger, &noticeBox{}))

	if err := job.StartScheduledWhatsappCampaigns(); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestAnAlreadyRunningCampaignIsNotAnError(t *testing.T) {
	ledger := &scheduleLedger{claimable: map[string]bool{"a": true}}
	dispatch := &recordingDispatch{fail: map[string]error{"a": wc.ErrDispatchCampaignAlreadyRunning}}
	box := &noticeBox{}
	job := NewScheduleStartJob(dispatch, dueCampaigns{due: []*wc.Campaign{{ID: "a"}}}, ledger, scheduleReporter(t, ledger, box))

	if err := job.StartScheduledWhatsappCampaigns(); err != nil {
		t.Fatalf("err %v, want nil", err)
	}
	if len(ledger.failures) != 0 || len(box.drafts) != 0 {
		t.Fatal("a campaign already running is not a failed schedule")
	}
}

func TestWithoutAReporterTheJobStartsNothing(t *testing.T) {
	ledger := &scheduleLedger{claimable: map[string]bool{"a": true}}
	dispatch := &recordingDispatch{}
	job := NewScheduleStartJob(dispatch, dueCampaigns{due: []*wc.Campaign{{ID: "a"}}}, ledger, nil)

	if err := job.StartScheduledWhatsappCampaigns(); !errors.Is(err, ErrScheduleStartUnavailable) {
		t.Fatalf("got %v", err)
	}
	if len(ledger.claimed) != 0 || len(dispatch.started) != 0 {
		t.Fatal("a schedule was consumed with no way to report its failure")
	}
}
