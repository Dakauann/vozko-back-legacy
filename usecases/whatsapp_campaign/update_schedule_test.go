package whatsapp_campaign_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/campaign"
	"vozko/domain/campaign/launchcheck"
	"vozko/domain/campaign/scheduledstart"
	wc "vozko/domain/whatsapp_campaign"
)

type scheduleChecks struct {
	report launchcheck.Report
	err    error
	seen   []wc.Campaign
}

func (s *scheduleChecks) RunOn(_ context.Context, camp *wc.Campaign) (launchcheck.Report, error) {
	s.seen = append(s.seen, *camp)
	return s.report, s.err
}

var noExit = launchcheck.Report{Findings: []launchcheck.Finding{{Code: launchcheck.CodeNoExit, Level: launchcheck.Confirm}}}

func scheduleEdit(start time.Time, fingerprint string) *wc.Campaign {
	return &wc.Campaign{Name: "Matrículas", ScheduledStart: start, ScheduleConfirmedFingerprint: fingerprint, ScheduledBy: "u1"}
}

func TestSchedulingAnEditRunsTheLaunchCheckOnTheEditedCampaign(t *testing.T) {
	f := newEditFixture(t)
	start := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)

	in := scheduleEdit(start, "")
	in.Name = "Matrículas 2027"
	if _, err := f.uc.Execute(context.Background(), "camp-1", in); err != nil {
		t.Fatal(err)
	}

	if len(f.checks.seen) != 1 || f.checks.seen[0].Name != "Matrículas 2027" || !f.checks.seen[0].ScheduledStart.Equal(start) {
		t.Fatalf("checked %+v", f.checks.seen)
	}
	if f.current().ScheduledBy != "u1" || f.current().ScheduleConfirmedFingerprint != "" {
		t.Fatalf("saved %+v", f.current())
	}
}

func TestSchedulingWithAnUnconfirmedRiskIsRefused(t *testing.T) {
	f := newEditFixture(t)
	f.checks.report = noExit

	_, err := f.uc.Execute(context.Background(), "camp-1", scheduleEdit(time.Now().Add(2*time.Hour), ""))

	if !errors.Is(err, launchcheck.ErrUnconfirmed) {
		t.Fatalf("got %v", err)
	}
	if f.campaigns.updates != 0 {
		t.Fatal("an unconfirmed schedule was saved")
	}
}

func TestSchedulingWithTheConfirmedRiskStoresTheFingerprint(t *testing.T) {
	f := newEditFixture(t)
	f.checks.report = noExit
	f.current().ScheduleFailure = scheduledstart.FailureBlocked

	if _, err := f.uc.Execute(context.Background(), "camp-1", scheduleEdit(time.Now().Add(2*time.Hour), noExit.Fingerprint())); err != nil {
		t.Fatal(err)
	}

	if f.current().ScheduleConfirmedFingerprint != noExit.Fingerprint() || f.current().ScheduleFailure != "" {
		t.Fatalf("saved %+v", f.current())
	}
}

func TestABlockedCampaignCannotBeScheduled(t *testing.T) {
	f := newEditFixture(t)
	f.checks.report = launchcheck.Blocked(launchcheck.CodePhoneUnavailable, 0)

	_, err := f.uc.Execute(context.Background(), "camp-1", scheduleEdit(time.Now().Add(2*time.Hour), ""))

	if !errors.Is(err, launchcheck.ErrBlocked) || f.campaigns.updates != 0 {
		t.Fatalf("err %v updates %d", err, f.campaigns.updates)
	}
}

func TestAKeptScheduleWithTheSameRisksNeedsNoNewConfirmation(t *testing.T) {
	f := newEditFixture(t)
	f.checks.report = noExit
	kept := time.Now().Add(3 * time.Hour).UTC()
	f.current().ScheduledStart = kept
	f.current().ScheduleConfirmedFingerprint = noExit.Fingerprint()

	if _, err := f.uc.Execute(context.Background(), "camp-1", scheduleEdit(kept, "")); err != nil {
		t.Fatalf("a rename of a confirmed schedule was refused: %v", err)
	}
	if f.current().ScheduleConfirmedFingerprint != noExit.Fingerprint() {
		t.Fatalf("saved %+v", f.current())
	}
}

func TestAKeptScheduleWhoseRisksChangedNeedsANewConfirmation(t *testing.T) {
	f := newEditFixture(t)
	f.checks.report = noExit
	kept := time.Now().Add(3 * time.Hour).UTC()
	f.current().ScheduledStart = kept

	if _, err := f.uc.Execute(context.Background(), "camp-1", scheduleEdit(kept, "")); !errors.Is(err, launchcheck.ErrUnconfirmed) {
		t.Fatalf("got %v", err)
	}
}

func TestALeadSelectionSendCannotBeScheduled(t *testing.T) {
	f := newEditFixture(t)
	f.current().Source = campaign.SourceLeadSelection

	_, err := f.uc.Execute(context.Background(), "camp-1", scheduleEdit(time.Now().Add(2*time.Hour), ""))

	if !errors.Is(err, campaign.ErrSelectionScheduleRefused) || f.campaigns.updates != 0 {
		t.Fatalf("err %v updates %d", err, f.campaigns.updates)
	}
}

func TestACheckThatCannotRunRefusesTheSchedule(t *testing.T) {
	f := newEditFixture(t)
	boom := errors.New("quality unreadable")
	f.checks.err = boom

	if _, err := f.uc.Execute(context.Background(), "camp-1", scheduleEdit(time.Now().Add(2*time.Hour), "")); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestTurningTheScheduleOffForgetsWhoConfirmedIt(t *testing.T) {
	f := newEditFixture(t)
	f.current().ScheduledStart = time.Now().Add(3 * time.Hour).UTC()
	f.current().ScheduledBy = "u1"
	f.current().ScheduleConfirmedFingerprint = noExit.Fingerprint()

	if _, err := f.uc.Execute(context.Background(), "camp-1", &wc.Campaign{Name: "Matrículas"}); err != nil {
		t.Fatal(err)
	}
	if !f.current().ScheduledStart.IsZero() || f.current().ScheduledBy != "" || f.current().ScheduleConfirmedFingerprint != "" {
		t.Fatalf("saved %+v", f.current())
	}
	if len(f.checks.seen) != 0 {
		t.Fatal("clearing a schedule ran the launch check")
	}
}
