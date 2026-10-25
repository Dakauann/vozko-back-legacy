package whatsapp_campaign_repository

import (
	"context"
	"testing"

	"vozko/domain/campaign/scheduledstart"
	wc "vozko/domain/whatsapp_campaign"
)

type failureRecordingRepo struct {
	*fakeRepo
	recorded map[string]scheduledstart.Failure
}

func (f *failureRecordingRepo) RecordScheduleFailure(_ context.Context, campaignID string, failure scheduledstart.Failure) error {
	f.recorded[campaignID] = failure
	if c, ok := f.campaigns[campaignID]; ok {
		c.ScheduleFailure = failure
	}
	return nil
}

func TestARecordedScheduleFailureIsNotHiddenByTheCache(t *testing.T) {
	inner := &failureRecordingRepo{fakeRepo: newFakeRepo(), recorded: map[string]scheduledstart.Failure{}}
	inner.put(&wc.Campaign{ID: "c-1", Status: wc.CampaignStatusStopped})
	repo := NewCachedRepository(inner, newFakeSharedState())
	if _, err := repo.FindByID("c-1"); err != nil {
		t.Fatal(err)
	}

	if err := repo.RecordScheduleFailure(context.Background(), "c-1", scheduledstart.FailureBlocked); err != nil {
		t.Fatal(err)
	}

	got, err := repo.FindByID("c-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ScheduleFailure != scheduledstart.FailureBlocked {
		t.Fatalf("the cache still shows %q", got.ScheduleFailure)
	}
}

func TestARepositoryThatCannotRecordFailuresRefuses(t *testing.T) {
	repo := NewCachedRepository(newFakeRepo(), newFakeSharedState())
	if err := repo.RecordScheduleFailure(context.Background(), "c-1", scheduledstart.FailureBlocked); err == nil {
		t.Fatal("a failure was silently dropped")
	}
}
