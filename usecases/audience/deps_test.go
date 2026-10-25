package audience_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	ca "vozko/domain/audience"
	"vozko/domain/shared/deps"
)

type quietAudienceMetrics struct{}

func (quietAudienceMetrics) IncCommentEnqueued(string)                {}
func (quietAudienceMetrics) IncCommentBatches(string, string)         {}
func (quietAudienceMetrics) AddCommentItems(string, int)              {}
func (quietAudienceMetrics) AddCommentTokens(string, string, int)     {}
func (quietAudienceMetrics) ObserveCommentBatchLatency(time.Duration) {}
func (quietAudienceMetrics) SetCommentPending(string, int)            {}
func (quietAudienceMetrics) AddCommentTruncated(int)                  {}
func (quietAudienceMetrics) IncCommentCapHit(string)                  {}

type quietLive struct{}

func (quietLive) AnalysisStateChanged(ca.ConversationAnalysisState) {}

type countingRoles struct{ accounts []string }

func (r *countingRoles) RunAccount(_ context.Context, _ ca.Source, accountID string) int {
	r.accounts = append(r.accounts, accountID)
	return 0
}

func ingestFor(repo ca.Repository, settings ca.SettingsResolver, scheduler ca.Scheduler, live ca.ConversationAnalysisLive, clock ca.Clock) ca.Ingestor {
	if live == nil {
		live = quietLive{}
	}
	ingest, err := NewIngestUseCase(IngestDeps{Repo: repo, Settings: settings, Scheduler: scheduler, Metrics: quietAudienceMetrics{}, Live: live, Clock: clock})
	if err != nil {
		panic(err)
	}
	return ingest
}

func TestNewIngestUseCaseRefusesMissingDependencies(t *testing.T) {
	ingest, err := NewIngestUseCase(IngestDeps{Metrics: quietAudienceMetrics{}})
	var missing *deps.MissingError
	if ingest != nil || !errors.As(err, &missing) || len(missing.Missing) != 4 {
		t.Fatalf("ingest = %v err = %v, want repository, settings, scheduler and live analysis refused", ingest, err)
	}
}

func TestNewRollupJobRefusesAMissingRoleInference(t *testing.T) {
	job, err := NewRollupJob(RollupDeps{Repo: newFakeRepo(), Settings: newFakeSettings(enabledSettings()), Authors: &fakeAuthors{}, Rollups: &fakeRollups{}, State: newFakeState(), Clock: fixedClock{now}})
	var missing *deps.MissingError
	if job != nil || !errors.As(err, &missing) || missing.Missing[0] != "role inference" {
		t.Fatalf("job = %v err = %v, want the role inference refused", job, err)
	}
}
