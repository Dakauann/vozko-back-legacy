package site_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	site_usecase "vozko/usecases/site"
)

const mockMark = "55555555-5555-4555-8555-555555555555"

func publishMark(now time.Time) site_usecase.PublishMark {
	return site_usecase.PublishMark{
		WorkspaceID: mockWS, ProjectID: mockProject, SourceRevisionID: mockRev, SourceVersion: 4,
		Revision: site_usecase.Revision{ID: mockMark, WorkspaceID: mockWS, ProjectID: mockProject, ParentID: mockRev, Kind: site_usecase.RevisionPublish, Author: "person:u1", CreatedAt: now, UpdatedAt: now},
	}
}

func TestThePublishMarkMovesTheWorkingRevisionWithoutANewVersion(t *testing.T) {
	repo, mock := newMock(t)
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "site_revisions"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`UPDATE site_projects SET working_revision_id = \$1 WHERE id = \$2 AND workspace_id = \$3 AND version = \$4 AND working_revision_id = \$5 RETURNING \*`).
		WithArgs(mockMark, mockProject, mockWS, int64(4), mockRev).
		WillReturnRows(sqlmock.NewRows(projectColumns).AddRow(mockProject, mockWS, "Página", "pagina", "html", "published", mockMark, 4, mockWS, now, now))
	mock.ExpectCommit()
	marked, err := repo.MarkPublished(context.Background(), publishMark(now))
	if err != nil {
		t.Fatal(err)
	}
	if !marked.Current || marked.Project.Version != 4 || marked.Project.WorkingRevisionID != mockMark {
		t.Fatalf("got %+v", marked.Project)
	}
	met(t, mock)
}

func TestThePublishMarkOfAnEditedPageLeavesTheEditInPlace(t *testing.T) {
	repo, mock := newMock(t)
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "site_revisions"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`UPDATE site_projects SET working_revision_id`).WillReturnRows(sqlmock.NewRows(projectColumns))
	mock.ExpectQuery(`SELECT \* FROM "site_projects" WHERE id = \$1 AND workspace_id = \$2`).
		WillReturnRows(sqlmock.NewRows(projectColumns).AddRow(mockProject, mockWS, "Página", "pagina", "html", "published", "66666666-6666-4666-8666-666666666666", 5, mockWS, now, now))
	mock.ExpectCommit()
	marked, err := repo.MarkPublished(context.Background(), publishMark(now))
	if err != nil {
		t.Fatal(err)
	}
	if marked.Current || marked.Project.Version != 5 {
		t.Fatalf("got %+v", marked)
	}
	met(t, mock)
}

func TestThePublishMarkOfAMissingPageRefuses(t *testing.T) {
	repo, mock := newMock(t)
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "site_revisions"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`UPDATE site_projects SET working_revision_id`).WillReturnRows(sqlmock.NewRows(projectColumns))
	mock.ExpectQuery(`SELECT \* FROM "site_projects"`).WillReturnRows(sqlmock.NewRows(projectColumns))
	mock.ExpectRollback()
	if _, err := repo.MarkPublished(context.Background(), publishMark(now)); !errors.Is(err, site_usecase.ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
	bad := publishMark(now)
	bad.ProjectID = "x"
	if _, err := repo.MarkPublished(context.Background(), bad); !errors.Is(err, site_usecase.ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
	met(t, mock)
}
