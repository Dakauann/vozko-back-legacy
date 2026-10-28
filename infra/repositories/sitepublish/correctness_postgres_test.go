package sitepublish_repository

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/sitepublish"
	"vozko/infra/database/schema"
)

func TestPostgresOnlyOneDeploymentOfAPageIsActive(t *testing.T) {
	db := postgresDB(t)
	repo := NewDeployments(db)
	ctx := context.Background()
	base := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	first, second := uuid.NewString(), uuid.NewString()
	if err := repo.Create(ctx, deployment(first, sitepublish.DeploymentActive, base)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, deployment(second, sitepublish.DeploymentActive, base.Add(time.Minute))); err == nil {
		t.Fatal("a second active deployment of the same page is refused")
	}
	ready := deployment(second, sitepublish.DeploymentReady, base.Add(time.Minute))
	ready.Outbound = []string{"https://wa.me/5511999999999"}
	if err := repo.Create(ctx, ready); err != nil {
		t.Fatal(err)
	}
	if err := repo.Activate(ctx, projectID, second, "person:u1", base); err != nil {
		t.Fatalf("activation supersedes first: %v", err)
	}
	if err := repo.RecordPublished(ctx, projectID, second, "rev-mark"); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, workspaceID, second)
	if err != nil || got.RevisionID != "rev-mark" || got.Status != sitepublish.DeploymentActive || !slices.Equal(got.Outbound, ready.Outbound) {
		t.Fatalf("got %+v %v", got, err)
	}
	if err := repo.RecordPublished(ctx, projectID, uuid.NewString(), "rev"); !errors.Is(err, sitepublish.ErrDeploymentNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestPostgresLivePagesCarryTheirPublicationAndSuspension(t *testing.T) {
	db := postgresDB(t)
	if err := db.AutoMigrate(&schema.SiteProject{}); err != nil {
		t.Fatal(err)
	}
	repo := NewDeployments(db)
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	if err := repo.Create(ctx, deployment(uuid.NewString(), sitepublish.DeploymentActive, now)); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&schema.SiteProject{ID: projectID, WorkspaceID: workspaceID, Name: "Página", Slug: "pagina", Stack: "html", Status: "published", WorkingRevisionID: uuid.NewString(), Version: 1, CreatedBy: workspaceID, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewAbuseCases(db).Create(ctx, sitepublish.AbuseCase{ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID, Source: sitepublish.AbuseAdmin, Status: sitepublish.AbuseSuspended, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	live, err := repo.LiveProjects(ctx, "", 10)
	if err != nil || len(live) != 1 || live[0].ProjectStatus != "published" || !live[0].AbuseSuspended || live[0].WorkspaceID != workspaceID {
		t.Fatalf("got %+v %v", live, err)
	}
	if after, err := repo.LiveProjects(ctx, projectID, 10); err != nil || len(after) != 0 {
		t.Fatalf("pages after the cursor: %+v %v", after, err)
	}
}

func TestPostgresTheDomainQuotaHoldsUnderConcurrentAdds(t *testing.T) {
	db := postgresDB(t)
	repo := NewHostnames(db)
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	results := make([]error, 6)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := sitepublish.HostnameRecord{ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID, Hostname: uuid.NewString()[:8] + ".cliente.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnamePendingDNS, CreatedAt: now.Add(time.Duration(i) * time.Minute), UpdatedAt: now}
			results[i] = repo.CreateCustom(ctx, rec, 2)
		}(i)
	}
	wg.Wait()
	created := 0
	for _, err := range results {
		switch {
		case err == nil:
			created++
		case !errors.Is(err, sitepublish.ErrDomainQuota):
			t.Fatalf("got %v", err)
		}
	}
	if created != 2 {
		t.Fatalf("the quota allows two, created %d", created)
	}
	list, err := repo.ListCustomByWorkspace(ctx, workspaceID)
	if err != nil || len(list) != 2 || list[0].CreatedAt.After(list[1].CreatedAt) {
		t.Fatalf("oldest first: %+v %v", list, err)
	}
	workspaces, err := repo.CustomWorkspaces(ctx, "", 10)
	if err != nil || !slices.Equal(workspaces, []string{workspaceID}) {
		t.Fatalf("got %v %v", workspaces, err)
	}
	if rest, err := repo.CustomWorkspaces(ctx, workspaceID, 10); err != nil || len(rest) != 0 {
		t.Fatalf("got %v %v", rest, err)
	}
}
