package whatsapp_campaign

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"vozko/domain/campaign"
	"vozko/domain/campaign/launchcheck"
	"vozko/domain/campaign/scheduledstart"
)

var risky = launchcheck.Report{Findings: []launchcheck.Finding{{Code: launchcheck.CodeNoExit, Level: launchcheck.Confirm}}}

func scheduledIn(d time.Duration) *Campaign {
	return &Campaign{ID: "c1", ScheduledStart: time.Now().Add(d).UTC(), ScheduleFailure: scheduledstart.FailureBlocked}
}

func TestAnUnscheduledCampaignKeepsNoScheduleCompanions(t *testing.T) {
	c := Campaign{ScheduledBy: "u1", ScheduleConfirmedFingerprint: "abc"}
	c.Normalize()
	if c.ScheduledBy != "" || c.ScheduleConfirmedFingerprint != "" {
		t.Fatalf("got %+v", c)
	}
}

func TestSchedulingStoresWhoConfirmedWhichRisks(t *testing.T) {
	c := scheduledIn(time.Hour)
	if err := c.AdmitSchedule(risky, risky.Fingerprint(), " u1 "); err != nil {
		t.Fatal(err)
	}
	if c.ScheduledBy != "u1" || c.ScheduleConfirmedFingerprint != risky.Fingerprint() || c.ScheduleFailure != "" {
		t.Fatalf("got %+v", c)
	}
}

func TestSchedulingWithUnconfirmedRisksIsRefused(t *testing.T) {
	c := scheduledIn(time.Hour)
	if err := c.AdmitSchedule(risky, "", "u1"); !errors.Is(err, launchcheck.ErrUnconfirmed) {
		t.Fatalf("got %v", err)
	}
	if c.ScheduledBy != "" || c.ScheduleFailure != scheduledstart.FailureBlocked {
		t.Fatalf("a refused schedule changed the campaign: %+v", c)
	}
}

func TestSchedulingABlockedCampaignIsRefused(t *testing.T) {
	c := scheduledIn(time.Hour)
	if err := c.AdmitSchedule(launchcheck.Blocked(launchcheck.CodePhoneUnavailable, 0), "", "u1"); !errors.Is(err, launchcheck.ErrBlocked) {
		t.Fatalf("got %v", err)
	}
}

func TestALeadSelectionSendIsNeverScheduled(t *testing.T) {
	c := scheduledIn(time.Hour)
	c.Source = campaign.SourceLeadSelection
	if err := c.AdmitSchedule(launchcheck.Report{}, "", "u1"); !errors.Is(err, campaign.ErrSelectionScheduleRefused) {
		t.Fatalf("got %v", err)
	}
}

func TestClearingAScheduleNeedsNoCheck(t *testing.T) {
	c := &Campaign{Source: campaign.SourceLeadSelection, ScheduledBy: "u1", ScheduleConfirmedFingerprint: "abc"}
	if err := c.AdmitSchedule(launchcheck.Blocked(launchcheck.CodePhoneUnavailable, 0), "", "u2"); err != nil {
		t.Fatal(err)
	}
	if c.ScheduledBy != "" || c.ScheduleConfirmedFingerprint != "" {
		t.Fatalf("got %+v", c)
	}
}

func TestACreatedScheduleTakesTheAdmittedFingerprint(t *testing.T) {
	at := time.Now().Add(2 * time.Hour).UTC()
	c := &Campaign{ScheduleFailure: scheduledstart.FailureBlocked}
	c.ApplySchedule(scheduledstart.Schedule{At: at, ConfirmedFingerprint: "abc"}, "u1")
	if !c.ScheduledStart.Equal(at) || c.ScheduleConfirmedFingerprint != "abc" || c.ScheduledBy != "u1" || c.ScheduleFailure != "" {
		t.Fatalf("got %+v", c)
	}
}

func TestTheScheduleSurvivesTheCampaignCache(t *testing.T) {
	c := scheduledIn(time.Hour)
	c.ScheduledBy, c.ScheduleConfirmedFingerprint = "u1", "abc"
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var back Campaign
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.ScheduledBy != "u1" || back.ScheduleConfirmedFingerprint != "abc" || back.ScheduleFailure != scheduledstart.FailureBlocked {
		t.Fatalf("round trip lost the schedule: %s", raw)
	}
}
