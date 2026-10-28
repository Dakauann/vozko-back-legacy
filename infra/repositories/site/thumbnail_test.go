package site_repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	site_usecase "vozko/usecases/site"
)

const mockMedia = "44444444-4444-4444-8444-444444444444"

func TestSetThumbnailSwapsTheMediaAndAnswersThePreviousOne(t *testing.T) {
	repo, mock := newMock(t)
	mock.ExpectQuery(`UPDATE site_projects p SET thumbnail_media_id = \$1 FROM \(SELECT id, thumbnail_media_id FROM site_projects WHERE id = \$2 AND workspace_id = \$3 FOR UPDATE\) previous WHERE p.id = previous.id RETURNING previous.thumbnail_media_id`).
		WithArgs(mockMedia, mockProject, mockWS).
		WillReturnRows(sqlmock.NewRows([]string{"thumbnail_media_id"}).AddRow("old-media"))
	previous, err := repo.SetThumbnail(context.Background(), mockWS, mockProject, mockMedia)
	if err != nil || previous != "old-media" {
		t.Fatalf("got %q %v", previous, err)
	}
	met(t, mock)
}

func TestSetThumbnailOfAMissingPageRefuses(t *testing.T) {
	repo, mock := newMock(t)
	mock.ExpectQuery(`UPDATE site_projects p SET thumbnail_media_id`).WillReturnRows(sqlmock.NewRows([]string{"thumbnail_media_id"}))
	if _, err := repo.SetThumbnail(context.Background(), mockWS, mockProject, mockMedia); !errors.Is(err, site_usecase.ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := repo.SetThumbnail(context.Background(), "x", mockProject, mockMedia); !errors.Is(err, site_usecase.ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
	met(t, mock)
}
