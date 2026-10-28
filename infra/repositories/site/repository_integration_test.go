package site_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
	site_usecase "vozko/usecases/site"
)

func isolated(t *testing.T, prefix string) (*Repository, *gorm.DB) {
	t.Helper()
	db := repotest.IsolatedDB(t, prefix, schema.SiteStorageModels()...)
	return New(db), db
}

func newRecord(ws, slug string, at time.Time) site_usecase.NewProject {
	projectID, revisionID := uuid.NewString(), uuid.NewString()
	return site_usecase.NewProject{
		Project: site_usecase.Project{
			ID: projectID, WorkspaceID: ws, Name: "Página " + slug, Slug: slug, Stack: site.StackHTML, Status: site_usecase.StatusDraft,
			WorkingRevisionID: revisionID, Version: 1, CreatedBy: uuid.NewString(), CreatedAt: at, UpdatedAt: at,
		},
		Revision: site_usecase.Revision{
			ID: revisionID, WorkspaceID: ws, ProjectID: projectID, Manifest: map[string]site_usecase.ManifestEntry{"pages/index.html": {SHA256: "aa", Rev: 1}},
			Author: site_usecase.PersonAuthor("u1"), Kind: site_usecase.RevisionCheckpoint, CreatedAt: at, UpdatedAt: at,
		},
		Blobs:    []site_usecase.Blob{{SHA256: "aa", Bytes: []byte("<h1>Oi</h1>")}},
		Settings: site_usecase.StoredSettings{ProjectID: projectID, WorkspaceID: ws, Settings: site.Settings{}, Version: 1, UpdatedAt: at},
	}
}

func TestAPageIsCreatedReadAndScopedToItsWorkspace(t *testing.T) {
	repo, _ := isolated(t, "site_projects_create")
	ctx := context.Background()
	ws := uuid.NewString()
	in := newRecord(ws, "clinica", time.Now().UTC())
	created, err := repo.CreateProject(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetProject(ctx, ws, created.ID)
	if err != nil || got.Slug != "clinica" || got.WorkingRevisionID != in.Revision.ID || got.Version != 1 {
		t.Fatalf("got %+v %v", got, err)
	}
	if _, err := repo.GetProject(ctx, uuid.NewString(), created.ID); !errors.Is(err, site_usecase.ErrProjectNotFound) {
		t.Fatalf("another workspace read it: %v", err)
	}
	rev, err := repo.GetRevision(ctx, ws, created.ID, in.Revision.ID)
	if err != nil || rev.Manifest["pages/index.html"].SHA256 != "aa" || rev.Kind != site_usecase.RevisionCheckpoint {
		t.Fatalf("got %+v %v", rev, err)
	}
	blobs, err := repo.Blobs(ctx, ws, []string{"aa", "missing"})
	if err != nil || string(blobs["aa"]) != "<h1>Oi</h1>" || len(blobs) != 1 {
		t.Fatalf("got %v %v", blobs, err)
	}
	if other, _ := repo.Blobs(ctx, uuid.NewString(), []string{"aa"}); len(other) != 0 {
		t.Fatal("blobs are per workspace")
	}
	settings, err := repo.GetSettings(ctx, ws, created.ID)
	if err != nil || settings.Version != 1 {
		t.Fatalf("got %+v %v", settings, err)
	}
	if _, err := repo.CreateProject(ctx, newRecord(ws, "clinica", time.Now().UTC())); !errors.Is(err, site_usecase.ErrSlugTaken) {
		t.Fatalf("slug: %v", err)
	}
	if _, err := repo.CreateProject(ctx, newRecord(uuid.NewString(), "clinica", time.Now().UTC())); err != nil {
		t.Fatalf("slugs are unique per workspace only: %v", err)
	}
	taken, err := repo.TakenSlugs(ctx, ws, "clin")
	if err != nil || len(taken) != 1 || taken[0] != "clinica" {
		t.Fatalf("got %v %v", taken, err)
	}
}

func TestAnEloOriginCreatesOnePage(t *testing.T) {
	repo, _ := isolated(t, "site_projects_origin")
	ctx := context.Background()
	ws := uuid.NewString()
	in := newRecord(ws, "elo", time.Now().UTC())
	in.Project.Origin = site_usecase.Origin{ThreadID: "thread_1", CommandID: uuid.NewString()}
	first, err := repo.CreateProject(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	again := newRecord(ws, "elo-2", time.Now().UTC())
	again.Project.Origin, again.Project.CreatedBy = in.Project.Origin, in.Project.CreatedBy
	second, err := repo.CreateProject(ctx, again)
	if err != nil || second.ID != first.ID {
		t.Fatalf("got %+v %v", second, err)
	}
	stranger := newRecord(ws, "elo-3", time.Now().UTC())
	stranger.Project.Origin = in.Project.Origin
	if _, err := repo.CreateProject(ctx, stranger); !errors.Is(err, site_usecase.ErrOriginTaken) {
		t.Fatalf("got %v", err)
	}
}

func TestCommitsMoveTheWorkingVersionOnlyFromTheVersionRead(t *testing.T) {
	repo, _ := isolated(t, "site_commits")
	ctx := context.Background()
	ws := uuid.NewString()
	at := time.Now().UTC().Truncate(time.Microsecond)
	in := newRecord(ws, "home", at)
	if _, err := repo.CreateProject(ctx, in); err != nil {
		t.Fatal(err)
	}
	auto := site_usecase.Revision{
		ID: uuid.NewString(), WorkspaceID: ws, ProjectID: in.Project.ID, ParentID: in.Revision.ID, Author: site_usecase.PersonAuthor("u1"),
		Kind: site_usecase.RevisionAutosave, Manifest: map[string]site_usecase.ManifestEntry{"pages/index.html": {SHA256: "bb", Rev: 2}}, CreatedAt: at.Add(time.Second), UpdatedAt: at.Add(time.Second),
	}
	p, err := repo.Commit(ctx, site_usecase.Commit{WorkspaceID: ws, ProjectID: in.Project.ID, ExpectedVersion: 1, Revision: auto, Blobs: []site_usecase.Blob{{SHA256: "bb", Bytes: []byte("b")}, {SHA256: "aa", Bytes: []byte("<h1>Oi</h1>")}}})
	if err != nil || p.Version != 2 || p.WorkingRevisionID != auto.ID {
		t.Fatalf("got %+v %v", p, err)
	}
	if _, err := repo.Commit(ctx, site_usecase.Commit{WorkspaceID: ws, ProjectID: in.Project.ID, ExpectedVersion: 1, Revision: auto}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("stale: %v", err)
	}
	auto.Manifest = map[string]site_usecase.ManifestEntry{"pages/index.html": {SHA256: "cc", Rev: 3}}
	auto.UpdatedAt = at.Add(time.Minute)
	p, err = repo.Commit(ctx, site_usecase.Commit{WorkspaceID: ws, ProjectID: in.Project.ID, ExpectedVersion: 2, Revision: auto, Replace: true, Blobs: []site_usecase.Blob{{SHA256: "cc", Bytes: []byte("c")}}})
	if err != nil || p.Version != 3 {
		t.Fatalf("replace: %+v %v", p, err)
	}
	stored, _ := repo.GetRevision(ctx, ws, in.Project.ID, auto.ID)
	if stored.Manifest["pages/index.html"].SHA256 != "cc" {
		t.Fatalf("the autosave is updated in place: %+v", stored)
	}
	list, total, err := repo.ListRevisions(ctx, ws, in.Project.ID, 10, 0)
	if err != nil || total != 2 || len(list) != 2 || list[0].ID != auto.ID || list[0].Manifest != nil {
		t.Fatalf("got %+v %d %v", list, total, err)
	}
	checkpoint := site_usecase.Revision{ID: in.Revision.ID, WorkspaceID: ws, ProjectID: in.Project.ID, Kind: site_usecase.RevisionCheckpoint, Manifest: map[string]site_usecase.ManifestEntry{}, Author: "person:u1", UpdatedAt: at}
	if _, err := repo.Commit(ctx, site_usecase.Commit{WorkspaceID: ws, ProjectID: in.Project.ID, ExpectedVersion: 3, Revision: checkpoint, Replace: true}); !errors.Is(err, site_usecase.ErrRevisionNotFound) {
		t.Fatalf("only an autosave is replaced: %v", err)
	}
	if again, _ := repo.GetProject(ctx, ws, in.Project.ID); again.Version != 3 {
		t.Fatalf("a refused replace rolls back: %+v", again)
	}
	var blobs int64
	repo.db.Model(&schema.SiteBlob{}).Where("workspace_id = ?", ws).Count(&blobs)
	if blobs != 3 {
		t.Fatalf("each content is stored once: %d", blobs)
	}
}

func TestArchivedPagesRefuseCommitsAndPagesUpdateWithTheirVersion(t *testing.T) {
	repo, _ := isolated(t, "site_projects_update")
	ctx := context.Background()
	ws := uuid.NewString()
	in := newRecord(ws, "home", time.Now().UTC())
	if _, err := repo.CreateProject(ctx, in); err != nil {
		t.Fatal(err)
	}
	other := newRecord(ws, "outra", time.Now().UTC())
	if _, err := repo.CreateProject(ctx, other); err != nil {
		t.Fatal(err)
	}
	p, _ := repo.GetProject(ctx, ws, in.Project.ID)
	p.Name, p.Status = "Arquivada", site_usecase.StatusArchived
	if err := repo.UpdateProject(ctx, p, 1); err != nil || p.Version != 2 || p.Name != "Arquivada" {
		t.Fatalf("got %+v %v", p, err)
	}
	stale := *p
	if err := repo.UpdateProject(ctx, &stale, 1); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("got %v", err)
	}
	p.Slug = "outra"
	if err := repo.UpdateProject(ctx, p, 2); !errors.Is(err, site_usecase.ErrSlugTaken) {
		t.Fatalf("got %v", err)
	}
	rev := in.Revision
	rev.ID = uuid.NewString()
	if _, err := repo.Commit(ctx, site_usecase.Commit{WorkspaceID: ws, ProjectID: in.Project.ID, ExpectedVersion: 2, Revision: rev}); !errors.Is(err, site_usecase.ErrProjectArchived) {
		t.Fatalf("got %v", err)
	}
	list, total, err := repo.ListProjects(ctx, site_usecase.ListQuery{WorkspaceID: ws, Limit: 10})
	if err != nil || total != 1 || list[0].ID != other.Project.ID {
		t.Fatalf("archived pages are hidden: %+v %v", list, err)
	}
	list, _, _ = repo.ListProjects(ctx, site_usecase.ListQuery{WorkspaceID: ws, Status: site_usecase.StatusArchived, Limit: 10})
	if len(list) != 1 || list[0].ID != in.Project.ID {
		t.Fatalf("got %+v", list)
	}
	list, _, _ = repo.ListProjects(ctx, site_usecase.ListQuery{WorkspaceID: ws, Search: "OUTRA", Limit: 10})
	if len(list) != 1 {
		t.Fatalf("search: %+v", list)
	}
	if err := repo.DeleteProject(ctx, ws, in.Project.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetSettings(ctx, ws, in.Project.ID); !errors.Is(err, site_usecase.ErrProjectNotFound) {
		t.Fatalf("settings go with the page: %v", err)
	}
	if err := repo.DeleteProject(ctx, ws, in.Project.ID); !errors.Is(err, site_usecase.ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestSettingsSaveFromTheVersionRead(t *testing.T) {
	repo, _ := isolated(t, "site_settings")
	ctx := context.Background()
	ws := uuid.NewString()
	in := newRecord(ws, "home", time.Now().UTC())
	if _, err := repo.CreateProject(ctx, in); err != nil {
		t.Fatal(err)
	}
	s := &site_usecase.StoredSettings{ProjectID: in.Project.ID, WorkspaceID: ws, Settings: site.Settings{Consent: site.Consent{Banner: true}}, UpdatedAt: time.Now().UTC()}
	if err := repo.SaveSettings(ctx, s, 1); err != nil || s.Version != 2 {
		t.Fatalf("got %+v %v", s, err)
	}
	if err := repo.SaveSettings(ctx, s, 1); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("got %v", err)
	}
	got, _ := repo.GetSettings(ctx, ws, in.Project.ID)
	if !got.Settings.Consent.Banner || got.Version != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestDesignSystemsKeepOneDefaultAndUnlinkOnDelete(t *testing.T) {
	repo, _ := isolated(t, "site_design_systems")
	ctx := context.Background()
	ws := uuid.NewString()
	now := time.Now().UTC()
	first := &site_usecase.DesignSystemRecord{ID: uuid.NewString(), WorkspaceID: ws, System: site.DesignSystem{Name: "Um"}, IsDefault: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	second := &site_usecase.DesignSystemRecord{ID: uuid.NewString(), WorkspaceID: ws, System: site.DesignSystem{Name: "Dois", Tokens: map[string]string{"--color-brand": "#123456"},
		Fonts: []site.DesignFont{{Family: "Inter", Role: site.FontBody}}}, IsDefault: true, Version: 1, CreatedAt: now, UpdatedAt: now.Add(time.Second)}
	for _, d := range []*site_usecase.DesignSystemRecord{first, second} {
		if err := repo.CreateDesignSystem(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	list, err := repo.ListDesignSystems(ctx, ws)
	if err != nil || len(list) != 2 || !list[0].IsDefault || list[1].IsDefault || list[0].System.Tokens["--color-brand"] != "#123456" {
		t.Fatalf("got %+v %v", list, err)
	}
	first.IsDefault, first.System.Name = true, "Um novo"
	if err := repo.UpdateDesignSystem(ctx, first, 1); err != nil || first.Version != 2 {
		t.Fatalf("got %+v %v", first, err)
	}
	if err := repo.UpdateDesignSystem(ctx, first, 1); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("got %v", err)
	}
	got, _ := repo.GetDesignSystem(ctx, ws, second.ID)
	if got.IsDefault {
		t.Fatal("one default per workspace")
	}
	in := newRecord(ws, "home", now)
	in.Project.DesignSystemID = first.ID
	if _, err := repo.CreateProject(ctx, in); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteDesignSystem(ctx, ws, first.ID); err != nil {
		t.Fatal(err)
	}
	p, _ := repo.GetProject(ctx, ws, in.Project.ID)
	if p.DesignSystemID != "" {
		t.Fatalf("got %+v", p)
	}
	if _, err := repo.GetDesignSystem(ctx, ws, first.ID); !errors.Is(err, site_usecase.ErrDesignSystemNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestPublicationStateIsSetWithoutBumpingTheVersion(t *testing.T) {
	repo, _ := isolated(t, "site_publication")
	ctx := context.Background()
	ws := uuid.NewString()
	in := newRecord(ws, "home", time.Now().UTC())
	if _, err := repo.CreateProject(ctx, in); err != nil {
		t.Fatal(err)
	}
	dep := uuid.NewString()
	p, err := repo.SetPublication(ctx, ws, in.Project.ID, site_usecase.StatusPublished, dep)
	if err != nil || p.Status != site_usecase.StatusPublished || p.PublishedDeploymentID != dep || p.Version != 1 {
		t.Fatalf("got %+v %v", p, err)
	}
	p, err = repo.SetPublication(ctx, ws, in.Project.ID, site_usecase.StatusDraft, "")
	if err != nil || p.PublishedDeploymentID != "" {
		t.Fatalf("got %+v %v", p, err)
	}
	if _, err := repo.SetPublication(ctx, uuid.NewString(), in.Project.ID, site_usecase.StatusPublished, dep); !errors.Is(err, site_usecase.ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
	p.Status = site_usecase.StatusArchived
	if err := repo.UpdateProject(ctx, p, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetPublication(ctx, ws, in.Project.ID, site_usecase.StatusPublished, dep); !errors.Is(err, site_usecase.ErrProjectArchived) {
		t.Fatalf("got %v", err)
	}
}

func TestFileRevsKeepTheLatestPerPathAndStayInTheWorkspace(t *testing.T) {
	repo, db := isolated(t, "site_file_revs")
	ctx := context.Background()
	ws := uuid.NewString()
	at := time.Now().UTC().Truncate(time.Microsecond)
	in := newRecord(ws, "revs", at)
	in.FileRevs = []site_usecase.FileRev{{Path: "pages/index.html", Rev: 1, SHA256: "aa"}}
	if _, err := repo.CreateProject(ctx, in); err != nil {
		t.Fatal(err)
	}
	version := int64(1)
	for rev := uint64(2); rev <= FileRevRetention+2; rev++ {
		next := site_usecase.Revision{ID: uuid.NewString(), WorkspaceID: ws, ProjectID: in.Project.ID, Author: "person:u1", Kind: site_usecase.RevisionCheckpoint,
			Manifest: map[string]site_usecase.ManifestEntry{"pages/index.html": {SHA256: "s", Rev: rev}}, CreatedAt: at, UpdatedAt: at}
		p, err := repo.Commit(ctx, site_usecase.Commit{WorkspaceID: ws, ProjectID: in.Project.ID, ExpectedVersion: version, Revision: next,
			FileRevs: []site_usecase.FileRev{{Path: "pages/index.html", Rev: rev, SHA256: "s" + uuid.NewString()[:4]}}})
		if err != nil {
			t.Fatal(err)
		}
		version = p.Version
	}
	var kept int64
	db.Model(&schema.SiteFileRev{}).Where("project_id = ?", in.Project.ID).Count(&kept)
	if kept != FileRevRetention {
		t.Fatalf("kept %d", kept)
	}
	got, err := repo.FileRevs(ctx, ws, in.Project.ID, map[string][]uint64{"pages/index.html": {1, 2, 3, FileRevRetention + 2}})
	if err != nil || len(got["pages/index.html"]) != 2 || got["pages/index.html"][3] == "" || got["pages/index.html"][FileRevRetention+2] == "" {
		t.Fatalf("the oldest revs are pruned: %+v %v", got, err)
	}
	if other, err := repo.FileRevs(ctx, uuid.NewString(), in.Project.ID, map[string][]uint64{"pages/index.html": {3}}); err != nil || len(other) != 0 {
		t.Fatalf("another workspace read the history: %+v %v", other, err)
	}
	if err := repo.DeleteProject(ctx, ws, in.Project.ID); err != nil {
		t.Fatal(err)
	}
	db.Model(&schema.SiteFileRev{}).Where("project_id = ?", in.Project.ID).Count(&kept)
	if kept != 0 {
		t.Fatalf("a deleted page leaves no history: %d", kept)
	}
}
