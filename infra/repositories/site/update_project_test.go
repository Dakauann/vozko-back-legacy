package site_repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	site_usecase "vozko/usecases/site"
)

func TestAProjectEditNeverOverwritesAPublicationStatus(t *testing.T) {
	repo, mock := newMock(t)
	now := time.Now()
	mock.ExpectQuery(`UPDATE site_projects SET name = \$1, slug = \$2, status = CASE WHEN status IN \('published','suspended'\) THEN status ELSE \$3 END, design_system_id = \$4, version = version \+ 1, updated_at = \$5\s+WHERE id = \$6 AND workspace_id = \$7 AND version = \$8 AND NOT \(status IN \('published','suspended'\) AND \(\$9 = 'archived' OR slug <> \$10\)\) RETURNING \*`).
		WithArgs("Nova", "nova", "draft", nil, sqlmock.AnyArg(), mockProject, mockWS, int64(3), "draft", "nova").
		WillReturnRows(sqlmock.NewRows(projectColumns).AddRow(mockProject, mockWS, "Nova", "nova", "html", "published", mockRev, 4, mockWS, now, now))
	p := &site_usecase.Project{ID: mockProject, WorkspaceID: mockWS, Name: "Nova", Slug: "nova", Status: site_usecase.StatusDraft}
	if err := repo.UpdateProject(context.Background(), p, 3); err != nil {
		t.Fatal(err)
	}
	if p.Status != site_usecase.StatusPublished {
		t.Fatalf("a publish that landed meanwhile keeps its status: %s", p.Status)
	}
	met(t, mock)
}
