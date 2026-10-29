package tutorial_repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/tutorial"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

var _ tutorial.ProgressRepository = (*Progress)(nil)

func TestTutorialProgressIsStoredPerPersonAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	db := repotest.IsolatedDB(t, "tutorial_progress_test", &schema.TutorialProgress{})
	repo := NewProgress(db)
	person, other := uuid.NewString(), uuid.NewString()
	at := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

	empty, err := repo.List(ctx, person)
	if err != nil || len(empty) != 0 {
		t.Fatalf("fresh = %+v, %v", empty, err)
	}
	if _, found, err := repo.Find(ctx, person, "sites.editor"); err != nil || found {
		t.Fatalf("find fresh = %v, %v", found, err)
	}

	started := tutorial.Progress{TutorialID: "sites.editor", Status: tutorial.StatusInProgress, Step: 2, UpdatedAt: at}
	if err := repo.Save(ctx, person, started); err != nil {
		t.Fatal(err)
	}
	done := tutorial.Progress{TutorialID: "sites.editor", Status: tutorial.StatusCompleted, Step: 7, UpdatedAt: at.Add(time.Minute)}
	if err := repo.Save(ctx, person, done); err != nil {
		t.Fatalf("saving again must upsert: %v", err)
	}
	if err := repo.Save(ctx, person, tutorial.Progress{TutorialID: "agents.create", Status: tutorial.StatusDismissed, UpdatedAt: at}); err != nil {
		t.Fatal(err)
	}

	got, found, err := repo.Find(ctx, person, "sites.editor")
	if err != nil || !found || got.Status != tutorial.StatusCompleted || got.Step != 7 || !got.UpdatedAt.Equal(done.UpdatedAt) {
		t.Fatalf("find = %+v, %v, %v", got, found, err)
	}
	listed, err := repo.List(ctx, person)
	if err != nil || len(listed) != 2 || listed[0].TutorialID != "agents.create" {
		t.Fatalf("list = %+v, %v", listed, err)
	}
	theirs, err := repo.List(ctx, other)
	if err != nil || len(theirs) != 0 {
		t.Fatalf("another person sees %+v, %v", theirs, err)
	}

	if err := repo.Delete(ctx, person, "sites.editor"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, person, "sites.editor"); err != nil {
		t.Fatalf("deleting twice must be fine: %v", err)
	}
	if _, found, _ := repo.Find(ctx, person, "sites.editor"); found {
		t.Fatal("reset tutorial is still stored")
	}
}
