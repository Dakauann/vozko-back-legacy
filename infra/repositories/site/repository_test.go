package site_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"vozko/domain/shared"
	site_usecase "vozko/usecases/site"
)

const (
	mockWS      = "11111111-1111-4111-8111-111111111111"
	mockProject = "22222222-2222-4222-8222-222222222222"
	mockRev     = "33333333-3333-4333-8333-333333333333"
)

func newMock(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	return New(db), mock
}

func met(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

var projectColumns = []string{"id", "workspace_id", "name", "slug", "stack", "status", "working_revision_id", "version", "created_by", "created_at", "updated_at"}

func TestMalformedIDsNeverReachTheDatabase(t *testing.T) {
	repo, mock := newMock(t)
	ctx := context.Background()
	if _, err := repo.GetProject(ctx, mockWS, "1 OR 1=1"); !errors.Is(err, site_usecase.ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := repo.GetRevision(ctx, mockWS, mockProject, "x"); !errors.Is(err, site_usecase.ErrRevisionNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := repo.GetDesignSystem(ctx, "x", mockProject); !errors.Is(err, site_usecase.ErrDesignSystemNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, _, err := repo.ListProjects(ctx, site_usecase.ListQuery{WorkspaceID: ""}); !errors.Is(err, site_usecase.ErrWorkspaceRequired) {
		t.Fatalf("got %v", err)
	}
	if got, err := repo.Blobs(ctx, mockWS, nil); err != nil || len(got) != 0 {
		t.Fatalf("got %v %v", got, err)
	}
	met(t, mock)
}

func TestGetProjectReadsWithinTheWorkspace(t *testing.T) {
	repo, mock := newMock(t)
	now := time.Now()
	mock.ExpectQuery(`SELECT \* FROM "site_projects" WHERE workspace_id = \$1 AND id = \$2 LIMIT \$3`).
		WithArgs(mockWS, mockProject, 1).
		WillReturnRows(sqlmock.NewRows(projectColumns).AddRow(mockProject, mockWS, "Página", "pagina", "react", "draft", mockRev, 4, mockWS, now, now))
	p, err := repo.GetProject(context.Background(), mockWS, mockProject)
	if err != nil || p.Slug != "pagina" || p.Version != 4 || p.WorkingRevisionID != mockRev || p.DesignSystemID != "" {
		t.Fatalf("got %+v %v", p, err)
	}
	mock.ExpectQuery(`SELECT \* FROM "site_projects"`).WillReturnRows(sqlmock.NewRows(projectColumns))
	if _, err := repo.GetProject(context.Background(), mockWS, mockProject); !errors.Is(err, site_usecase.ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
	met(t, mock)
}

func TestAStaleCommitRollsBackAndReportsTheConflict(t *testing.T) {
	repo, mock := newMock(t)
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE site_projects SET working_revision_id = \$1, version = version \+ 1, updated_at = \$2\s+WHERE id = \$3 AND workspace_id = \$4 AND version = \$5 AND status <> \$6 RETURNING \*`).
		WithArgs(mockRev, now, mockProject, mockWS, int64(3), "archived").
		WillReturnRows(sqlmock.NewRows(projectColumns))
	mock.ExpectRollback()
	mock.ExpectQuery(`SELECT \* FROM "site_projects"`).
		WillReturnRows(sqlmock.NewRows(projectColumns).AddRow(mockProject, mockWS, "Página", "pagina", "react", "draft", mockRev, 5, mockWS, now, now))
	_, err := repo.Commit(context.Background(), site_usecase.Commit{
		WorkspaceID: mockWS, ProjectID: mockProject, ExpectedVersion: 3,
		Revision: site_usecase.Revision{ID: mockRev, WorkspaceID: mockWS, ProjectID: mockProject, Kind: site_usecase.RevisionAutosave, UpdatedAt: now},
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("got %v", err)
	}
	met(t, mock)
}

func TestACommitUpdatesThePageStoresNewBlobsAndInsertsTheVersion(t *testing.T) {
	repo, mock := newMock(t)
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE site_projects SET working_revision_id`).
		WillReturnRows(sqlmock.NewRows(projectColumns).AddRow(mockProject, mockWS, "Página", "pagina", "react", "draft", mockRev, 4, mockWS, now, now))
	mock.ExpectExec(`INSERT INTO "site_blobs" .* ON CONFLICT DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "site_revisions"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	p, err := repo.Commit(context.Background(), site_usecase.Commit{
		WorkspaceID: mockWS, ProjectID: mockProject, ExpectedVersion: 3, Blobs: []site_usecase.Blob{{SHA256: "ab", Bytes: []byte("x")}},
		Revision: site_usecase.Revision{ID: mockRev, WorkspaceID: mockWS, ProjectID: mockProject, Kind: site_usecase.RevisionAutosave, Author: "person:u1", CreatedAt: now, UpdatedAt: now},
	})
	if err != nil || p.Version != 4 {
		t.Fatalf("got %+v %v", p, err)
	}
	met(t, mock)
}

func TestATakenSlugIsReportedAsSuch(t *testing.T) {
	repo, mock := newMock(t)
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "site_projects"`).WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: slugConstraint})
	mock.ExpectRollback()
	_, err := repo.CreateProject(context.Background(), site_usecase.NewProject{
		Project:  site_usecase.Project{ID: mockProject, WorkspaceID: mockWS, Slug: "pagina", WorkingRevisionID: mockRev, CreatedAt: now, UpdatedAt: now},
		Revision: site_usecase.Revision{ID: mockRev, WorkspaceID: mockWS, ProjectID: mockProject},
	})
	if !errors.Is(err, site_usecase.ErrSlugTaken) {
		t.Fatalf("got %v", err)
	}
	met(t, mock)
}

func TestAStaleSettingsSaveIsAConflict(t *testing.T) {
	repo, mock := newMock(t)
	mock.ExpectQuery(`UPDATE site_settings SET settings = \$1, version = version \+ 1`).WillReturnRows(sqlmock.NewRows([]string{"project_id"}))
	mock.ExpectQuery(`SELECT \* FROM "site_settings" WHERE workspace_id = \$1 AND project_id = \$2`).
		WillReturnRows(sqlmock.NewRows([]string{"project_id", "workspace_id", "settings", "version"}).AddRow(mockProject, mockWS, []byte(`{}`), 5))
	err := repo.SaveSettings(context.Background(), &site_usecase.StoredSettings{ProjectID: mockProject, WorkspaceID: mockWS}, 4)
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("got %v", err)
	}
	met(t, mock)
}

func TestStoredSettingsAreDecodedStrictly(t *testing.T) {
	repo, mock := newMock(t)
	mock.ExpectQuery(`SELECT \* FROM "site_settings"`).
		WillReturnRows(sqlmock.NewRows([]string{"project_id", "workspace_id", "settings", "version"}).AddRow(mockProject, mockWS, []byte(`{"theme":"x"}`), 1))
	if _, err := repo.GetSettings(context.Background(), mockWS, mockProject); err == nil {
		t.Fatal("an unreadable settings row must fail closed")
	}
	met(t, mock)
}
