package campaignschedule

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"vozko/domain/campaign/launchcheck"
	"vozko/domain/campaign/scheduledstart"
	"vozko/domain/notification"
	"vozko/domain/workspace"
)

type failureLedger struct {
	recorded map[string]scheduledstart.Failure
	err      error
}

func (l *failureLedger) RecordScheduleFailure(_ context.Context, campaignID string, failure scheduledstart.Failure) error {
	if l.err != nil {
		return l.err
	}
	if l.recorded == nil {
		l.recorded = map[string]scheduledstart.Failure{}
	}
	l.recorded[campaignID] = failure
	return nil
}

type inbox struct {
	drafts []notification.Draft
	err    error
}

func (i *inbox) Publish(_ context.Context, d notification.Draft) (notification.Record, error) {
	i.drafts = append(i.drafts, d)
	return notification.Record{}, i.err
}

type logLines struct{ lines []string }

func (l *logLines) logf(format string, args ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

var official = Channel{Key: "whatsapp", Label: "Campanha oficial", Path: "/dashboard/whatsapp-campaigns/", Resource: workspace.ResourceWhatsAppCampaigns}

var due = time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)

func newReporter(t *testing.T, ledger *failureLedger, box *inbox, logs *logLines) *Reporter {
	t.Helper()
	r, err := New(Deps{Channel: official, Failures: ledger, Notifications: box, Logf: logs.logf})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAFailedScheduledStartIsRecordedNotifiedAndLogged(t *testing.T) {
	ledger, box, logs := &failureLedger{}, &inbox{}, &logLines{}
	r := newReporter(t, ledger, box, logs)
	cause := &launchcheck.RefusedError{Report: launchcheck.Report{Findings: []launchcheck.Finding{{Code: launchcheck.CodeNoExit, Level: launchcheck.Confirm}}}}

	err := r.Failed(context.Background(), Attempt{WorkspaceID: "ws", CampaignID: "c1", CampaignName: "Black Friday", ScheduledBy: "u1", ScheduledAt: due}, cause)

	if err != nil {
		t.Fatal(err)
	}
	if ledger.recorded["c1"] != scheduledstart.FailureUnconfirmed {
		t.Fatalf("recorded %v", ledger.recorded)
	}
	if len(box.drafts) != 1 {
		t.Fatalf("published %d notifications", len(box.drafts))
	}
	d := box.drafts[0]
	if d.WorkspaceID != "ws" || d.Category != notification.CategoryCampaigns || d.Severity != notification.SeverityCritical || d.Source != notification.SourceCampaignSchedule {
		t.Fatalf("draft %+v", d)
	}
	if d.Audience.Kind != notification.AudienceUsers || len(d.Audience.UserIDs) != 1 || d.Audience.UserIDs[0] != "u1" {
		t.Fatalf("the notice goes to whoever scheduled it, got %+v", d.Audience)
	}
	if d.ActionURL != "/dashboard/whatsapp-campaigns/c1" || !strings.Contains(d.Title, "Black Friday") || d.Body == "" {
		t.Fatalf("draft %+v", d)
	}
	if d.DedupKey != "campaign_schedule_failed:whatsapp:c1:1792746000" {
		t.Fatalf("dedup %q", d.DedupKey)
	}
	if err := d.Normalize().Validate(); err != nil {
		t.Fatalf("the notice is invalid: %v", err)
	}
	if len(logs.lines) != 1 || !strings.Contains(logs.lines[0], "c1") || !strings.Contains(logs.lines[0], string(scheduledstart.FailureUnconfirmed)) {
		t.Fatalf("logs %v", logs.lines)
	}
}

func TestAScheduleWithoutAKnownAuthorNotifiesWhoMayStartIt(t *testing.T) {
	box := &inbox{}
	r := newReporter(t, &failureLedger{}, box, &logLines{})

	if err := r.Failed(context.Background(), Attempt{WorkspaceID: "ws", CampaignID: "c1", CampaignName: "x", ScheduledAt: due}, errors.New("queue down")); err != nil {
		t.Fatal(err)
	}

	a := box.drafts[0].Audience
	if a.Kind != notification.AudiencePermission || a.Permission == nil || a.Permission.Resource != workspace.ResourceWhatsAppCampaigns || a.Permission.Action != workspace.ActionStart {
		t.Fatalf("audience %+v", a)
	}
}

func TestEveryReasonHasItsOwnCopy(t *testing.T) {
	seen := map[string]scheduledstart.Failure{}
	for _, failure := range scheduledstart.Failures() {
		body := bodyOf(failure)
		if body == "" {
			t.Fatalf("%q has no copy", failure)
		}
		if strings.ContainsAny(body, "\u2013\u2014") {
			t.Fatalf("%q copy uses a dash", failure)
		}
		if other, dup := seen[body]; dup {
			t.Fatalf("%q and %q share the copy", failure, other)
		}
		seen[body] = failure
	}
}

func TestARecordingFailureIsReturnedAndTheNoticeStillGoesOut(t *testing.T) {
	boom := errors.New("db down")
	box := &inbox{}
	r := newReporter(t, &failureLedger{err: boom}, box, &logLines{})

	err := r.Failed(context.Background(), Attempt{WorkspaceID: "ws", CampaignID: "c1", CampaignName: "x", ScheduledAt: due}, errors.New("queue down"))

	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if len(box.drafts) != 1 {
		t.Fatal("the person was not told")
	}
}

func TestAPublishFailureIsLoggedNotReturned(t *testing.T) {
	logs := &logLines{}
	r := newReporter(t, &failureLedger{}, &inbox{err: errors.New("queue down")}, logs)

	if err := r.Failed(context.Background(), Attempt{WorkspaceID: "ws", CampaignID: "c1", CampaignName: "x", ScheduledAt: due}, errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	if len(logs.lines) != 2 {
		t.Fatalf("logs %v", logs.lines)
	}
}

func TestAStartClearsAnOldFailure(t *testing.T) {
	ledger := &failureLedger{recorded: map[string]scheduledstart.Failure{"c1": scheduledstart.FailureBlocked}}
	r := newReporter(t, ledger, &inbox{}, &logLines{})

	r.Cleared(context.Background(), "c1", scheduledstart.FailureBlocked)

	if got, ok := ledger.recorded["c1"]; !ok || got != "" {
		t.Fatalf("recorded %v", ledger.recorded)
	}
}

func TestAStartWithoutAFailureWritesNothing(t *testing.T) {
	ledger := &failureLedger{}
	r := newReporter(t, ledger, &inbox{}, &logLines{})

	r.Cleared(context.Background(), "c1", "")

	if len(ledger.recorded) != 0 {
		t.Fatalf("recorded %v", ledger.recorded)
	}
}

func TestANilReporterClearsNothing(t *testing.T) {
	var r *Reporter
	r.Cleared(context.Background(), "c1", scheduledstart.FailureBlocked)
}

func TestTheReporterNeedsEveryDependency(t *testing.T) {
	logs := &logLines{}
	cases := []Deps{
		{Failures: &failureLedger{}, Notifications: &inbox{}, Logf: logs.logf},
		{Channel: official, Notifications: &inbox{}, Logf: logs.logf},
		{Channel: official, Failures: &failureLedger{}, Logf: logs.logf},
		{Channel: official, Failures: &failureLedger{}, Notifications: &inbox{}},
	}
	for i, d := range cases {
		if _, err := New(d); !errors.Is(err, ErrIncomplete) {
			t.Errorf("case %d: got %v", i, err)
		}
	}
}
