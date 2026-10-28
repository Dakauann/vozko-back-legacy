package site_repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	site_usecase "vozko/usecases/site"
)

func TestPostgresAPublishMarkNeverBumpsTheVersionAndAPublicationIsNotOverwritten(t *testing.T) {
	repo, _ := isolated(t, "site_publish_mark")
	ctx := context.Background()
	ws := uuid.NewString()
	now := time.Now().UTC()
	in := newRecord(ws, "home", now)
	if _, err := repo.CreateProject(ctx, in); err != nil {
		t.Fatal(err)
	}
	mark := site_usecase.PublishMark{
		WorkspaceID: ws, ProjectID: in.Project.ID, SourceRevisionID: in.Revision.ID, SourceVersion: 1,
		Revision: site_usecase.Revision{ID: uuid.NewString(), WorkspaceID: ws, ProjectID: in.Project.ID, ParentID: in.Revision.ID, Manifest: in.Revision.Manifest, Author: "person:u1", Kind: site_usecase.RevisionPublish, CreatedAt: now, UpdatedAt: now},
	}
	marked, err := repo.MarkPublished(ctx, mark)
	if err != nil || !marked.Current || marked.Project.Version != 1 || marked.Project.WorkingRevisionID != mark.Revision.ID {
		t.Fatalf("got %+v %v", marked, err)
	}
	again := mark
	again.Revision.ID = uuid.NewString()
	if marked, err = repo.MarkPublished(ctx, again); err != nil || marked.Current {
		t.Fatalf("a stale source leaves the working revision alone: %+v %v", marked, err)
	}
	stale, _ := repo.GetProject(ctx, ws, in.Project.ID)
	if _, err := repo.SetPublication(ctx, ws, in.Project.ID, site_usecase.StatusPublished, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	stale.Name = "Renomeada"
	if err := repo.UpdateProject(ctx, stale, 1); err != nil {
		t.Fatal(err)
	}
	if stale.Status != site_usecase.StatusPublished || stale.Name != "Renomeada" {
		t.Fatalf("a rename keeps the publication: %+v", stale)
	}
}
