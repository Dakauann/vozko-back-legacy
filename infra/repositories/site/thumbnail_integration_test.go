package site_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	site_usecase "vozko/usecases/site"
)

func TestThumbnailSwapsAgainstPostgres(t *testing.T) {
	repo, _ := isolated(t, "site_projects_thumbnail")
	ctx := context.Background()
	ws := uuid.NewString()
	created, err := repo.CreateProject(ctx, newRecord(ws, "miniatura", time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	if previous, err := repo.SetThumbnail(ctx, ws, created.ID, "media-1"); err != nil || previous != "" {
		t.Fatalf("first thumbnail: %q %v", previous, err)
	}
	if previous, err := repo.SetThumbnail(ctx, ws, created.ID, "media-2"); err != nil || previous != "media-1" {
		t.Fatalf("second thumbnail: %q %v", previous, err)
	}
	got, err := repo.GetProject(ctx, ws, created.ID)
	if err != nil || got.ThumbnailMediaID != "media-2" || got.Version != created.Version || got.UpdatedAt.UnixMicro() != created.UpdatedAt.UnixMicro() {
		t.Fatalf("a thumbnail never bumps the version or the edit time: %+v %v", got, err)
	}
	if _, err := repo.SetThumbnail(ctx, uuid.NewString(), created.ID, "media-3"); !errors.Is(err, site_usecase.ErrProjectNotFound) {
		t.Fatalf("another workspace: %v", err)
	}
}
