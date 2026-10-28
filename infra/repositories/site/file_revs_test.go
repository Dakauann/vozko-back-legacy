package site_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	site_usecase "vozko/usecases/site"
)

func TestACommitRecordsTheNewFileRevsAndPrunesOldOnes(t *testing.T) {
	repo, mock := newMock(t)
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE site_projects SET working_revision_id`).
		WillReturnRows(sqlmock.NewRows(projectColumns).AddRow(mockProject, mockWS, "Página", "pagina", "react", "draft", mockRev, 4, mockWS, now, now))
	mock.ExpectExec(`INSERT INTO "site_revisions"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "site_file_revs" \("workspace_id","project_id","path","rev","sha256","created_at"\) VALUES \(\$1,\$2,\$3,\$4,\$5,\$6\) ON CONFLICT \("project_id","path","rev"\) DO UPDATE SET "sha256"="excluded"."sha256","created_at"="excluded"."created_at"`).
		WithArgs(mockWS, mockProject, "pages/index.html", int64(5), "cc", now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM site_file_revs f USING \(SELECT path, rev, row_number\(\) OVER \(PARTITION BY path ORDER BY rev DESC\) AS n FROM site_file_revs WHERE project_id = \$1 AND path IN \(\$2\)\) ranked\s+WHERE f.project_id = \$3 AND f.path = ranked.path AND f.rev = ranked.rev AND ranked.n > \$4`).
		WithArgs(mockProject, "pages/index.html", mockProject, FileRevRetention).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	_, err := repo.Commit(context.Background(), site_usecase.Commit{
		WorkspaceID: mockWS, ProjectID: mockProject, ExpectedVersion: 3, FileRevs: []site_usecase.FileRev{{Path: "pages/index.html", Rev: 5, SHA256: "cc"}},
		Revision: site_usecase.Revision{ID: mockRev, WorkspaceID: mockWS, ProjectID: mockProject, Kind: site_usecase.RevisionAutosave, Author: "person:u1", CreatedAt: now, UpdatedAt: now},
	})
	if err != nil {
		t.Fatal(err)
	}
	met(t, mock)
}

func TestFileRevsAreReadInOneScopedQuery(t *testing.T) {
	repo, mock := newMock(t)
	mock.ExpectQuery(`SELECT "path","rev","sha256" FROM "site_file_revs" WHERE workspace_id = \$1 AND project_id = \$2 AND \(path, rev\) IN \(\(\$3,\$4\),\(\$5,\$6\)\)`).
		WithArgs(mockWS, mockProject, "pages/index.html", int64(2), "pages/index.html", int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"path", "rev", "sha256"}).AddRow("pages/index.html", 2, "aa"))
	got, err := repo.FileRevs(context.Background(), mockWS, mockProject, map[string][]uint64{"pages/index.html": {2, 3}})
	if err != nil || len(got) != 1 || got["pages/index.html"][2] != "aa" {
		t.Fatalf("got %+v %v", got, err)
	}
	if got, err := repo.FileRevs(context.Background(), mockWS, mockProject, nil); err != nil || len(got) != 0 {
		t.Fatalf("nothing asked reads nothing: %v %v", got, err)
	}
	if _, err := repo.FileRevs(context.Background(), "x", mockProject, map[string][]uint64{"a": {1}}); !errors.Is(err, site_usecase.ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
	met(t, mock)
}
