package sitepublish_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vozko/domain/site"
	"vozko/domain/siteform"
	"vozko/domain/sitepublish"
	"vozko/infra/database/schema"
	"vozko/infra/testenv"
)

func postgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testenv.Postgres(t).DSN()
	silent := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(postgres.Open(dsn), silent)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	schemaName := "site_publish_test_" + uuid.New().String()[:8]
	if err := admin.Exec("CREATE SCHEMA " + schemaName).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schemaName), silent)
	if err != nil {
		t.Fatalf("open schema: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		_ = admin.Exec("DROP SCHEMA " + schemaName + " CASCADE").Error
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(schema.SitePublishModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

const workspaceID = "2f0e4c1a-0000-4000-8000-000000000001"

func deployment(id string, status sitepublish.DeploymentStatus, at time.Time) sitepublish.DeploymentRecord {
	return sitepublish.DeploymentRecord{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, RevisionID: "rev-1", Status: status, CreatedAt: at,
		Files:     map[string]sitepublish.FileEntry{"/index.html": {SHA256: "ab", ContentType: "text/html", Size: 2}},
		Checks:    []site.Check{{Code: site.CheckH1Missing, Route: "/"}},
		Forms:     sitepublish.FormsSnapshot{Manifests: []siteform.Manifest{{ID: "contato"}}},
		Redirects: []site.Redirect{{From: "/velho", To: "/", Status: 301}},
	}
}

func TestPostgresDeploymentsFlipTheActiveOneAtomically(t *testing.T) {
	db := postgresDB(t)
	repo := NewDeployments(db)
	ctx := context.Background()
	first, second := uuid.NewString(), uuid.NewString()
	base := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	for _, rec := range []sitepublish.DeploymentRecord{deployment(first, sitepublish.DeploymentReady, base), deployment(second, sitepublish.DeploymentBuilding, base.Add(time.Minute))} {
		if err := repo.Create(ctx, rec); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Activate(ctx, projectID, first, "person:u1", base); err != nil {
		t.Fatal(err)
	}
	if err := repo.Activate(ctx, projectID, second, "person:u1", base); !errors.Is(err, sitepublish.ErrDeploymentTransition) {
		t.Fatalf("a building deployment cannot go live, got %v", err)
	}
	ready := deployment(second, sitepublish.DeploymentReady, base)
	if err := repo.Update(ctx, ready); err != nil {
		t.Fatal(err)
	}
	if err := repo.Activate(ctx, projectID, second, "person:u2", base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	active, ok, err := repo.Active(ctx, projectID)
	if err != nil || !ok || active.ID != second || active.PublishedBy != "person:u2" || active.Redirects[0].From != "/velho" || active.Forms.Manifests[0].ID != "contato" {
		t.Fatalf("got %+v %v %v", active, ok, err)
	}
	old, err := repo.Get(ctx, workspaceID, first)
	if err != nil || old.Status != sitepublish.DeploymentSuperseded {
		t.Fatalf("got %+v %v", old, err)
	}
	if err := repo.Activate(ctx, projectID, first, "person:u1", base.Add(2*time.Hour)); err != nil {
		t.Fatalf("rollback to a superseded deployment: %v", err)
	}
	list, err := repo.ListByProject(ctx, workspaceID, projectID, 10)
	if err != nil || len(list) != 2 || list[0].ID != second {
		t.Fatalf("got %+v %v", list, err)
	}
	if _, err := repo.Get(ctx, uuid.NewString(), first); !errors.Is(err, sitepublish.ErrDeploymentNotFound) {
		t.Fatalf("another workspace must not read it, got %v", err)
	}
}

func TestPostgresHostnamesAreUniqueAndServeOnlyWhenActive(t *testing.T) {
	db := postgresDB(t)
	repo := NewHostnames(db)
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	platform := sitepublish.HostnameRecord{ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID, Hostname: "loja.paginas.example", Kind: sitepublish.HostnamePlatform, Status: sitepublish.HostnameActive, CreatedAt: now, UpdatedAt: now}
	custom := sitepublish.HostnameRecord{ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID, Hostname: "www.cliente.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnamePendingDNS, CloudflareRef: "cf-1", CreatedAt: now, UpdatedAt: now,
		Verification: sitepublish.Verification{CNAMETarget: "sites.paginas.example", Records: []sitepublish.DNSRecord{{Type: "TXT", Name: "_cf", Value: "x"}}}}
	for _, rec := range []sitepublish.HostnameRecord{platform, custom} {
		if err := repo.Create(ctx, rec); err != nil {
			t.Fatal(err)
		}
	}
	dup := platform
	dup.ID = uuid.NewString()
	if err := repo.Create(ctx, dup); !errors.Is(err, sitepublish.ErrHostnameTaken) {
		t.Fatalf("got %v", err)
	}
	hosts, err := repo.ActiveHostnames(ctx, projectID)
	if err != nil || len(hosts) != 1 || hosts[0] != "loja.paginas.example" {
		t.Fatalf("got %v %v", hosts, err)
	}
	if n, err := repo.CountCustom(ctx, workspaceID); err != nil || n != 1 {
		t.Fatalf("got %d %v", n, err)
	}
	pending, err := repo.ListByStatus(ctx, []sitepublish.HostnameStatus{sitepublish.HostnamePendingDNS, sitepublish.HostnamePendingSSL}, sitepublish.HostnameCustom, 10)
	if err != nil || len(pending) != 1 || pending[0].Verification.Records[0].Value != "x" {
		t.Fatalf("got %+v %v", pending, err)
	}
	custom.Status = sitepublish.HostnameRemoving
	if err := repo.Update(ctx, custom); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.CountCustom(ctx, workspaceID); n != 0 {
		t.Fatalf("a removing hostname no longer counts, got %d", n)
	}
	if err := repo.Delete(ctx, custom.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, workspaceID, custom.ID); !errors.Is(err, sitepublish.ErrHostnameNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestPostgresAnalyticsUpsertIsIdempotent(t *testing.T) {
	db := postgresDB(t)
	repo := NewAnalytics(db)
	ctx := context.Background()
	day := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	stat := sitepublish.DailyStat{ProjectID: projectID, Day: day, Route: "/", Variant: "control", Views: 3, Visitors: 2}
	if err := repo.Upsert(ctx, []sitepublish.DailyStat{stat}); err != nil {
		t.Fatal(err)
	}
	stat.Views, stat.FormSubmits = 9, 1
	if err := repo.Upsert(ctx, []sitepublish.DailyStat{stat}); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.List(ctx, projectID, day, day)
	if err != nil || len(rows) != 1 || rows[0].Views != 9 || rows[0].FormSubmits != 1 {
		t.Fatalf("got %+v %v", rows, err)
	}
}

func TestPostgresAbuseCasesTrackSuspension(t *testing.T) {
	db := postgresDB(t)
	repo := NewAbuseCases(db)
	ctx := context.Background()
	c := sitepublish.AbuseCase{ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID, Source: sitepublish.AbuseAdmin, Status: sitepublish.AbuseSuspended, Reason: "phishing", CreatedAt: time.Now()}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if suspended, err := repo.Suspended(ctx, projectID); err != nil || !suspended {
		t.Fatalf("got %v %v", suspended, err)
	}
	c.Status = sitepublish.AbuseRestored
	if err := repo.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	if suspended, _ := repo.Suspended(ctx, projectID); suspended {
		t.Fatal("restored")
	}
	open, err := repo.OpenForProject(ctx, projectID)
	if err != nil || len(open) != 0 {
		t.Fatalf("got %v %v", open, err)
	}
	list, err := repo.List(ctx, nil, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("got %v %v", list, err)
	}
}

func TestPostgresLatestAndServableDeployments(t *testing.T) {
	db := postgresDB(t)
	repo := NewDeployments(db)
	ctx := context.Background()
	base := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	if _, err := repo.Latest(ctx, projectID); !errors.Is(err, sitepublish.ErrDeploymentNotFound) {
		t.Fatalf("got %v", err)
	}
	ready, failed := uuid.NewString(), uuid.NewString()
	_ = repo.Create(ctx, deployment(ready, sitepublish.DeploymentReady, base))
	_ = repo.Create(ctx, deployment(failed, sitepublish.DeploymentFailed, base.Add(time.Minute)))
	latest, err := repo.Latest(ctx, projectID)
	if err != nil || latest.ID != failed {
		t.Fatalf("got %+v %v", latest, err)
	}
	servable, err := repo.ListServable(ctx, projectID)
	if err != nil || len(servable) != 1 || servable[0].ID != ready {
		t.Fatalf("got %+v %v", servable, err)
	}
}
