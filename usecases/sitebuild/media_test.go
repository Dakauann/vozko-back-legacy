package sitebuild

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/media"
)

type mediaRows map[string]media.Media

func (m mediaRows) GetMediaForWorkspace(workspaceID, mediaID string) (*media.Media, error) {
	if mediaID == "broken" {
		return nil, errBoom
	}
	row, ok := m[mediaID]
	if !ok || row.WorkspaceID != workspaceID {
		return nil, media.ErrMediaNotFound
	}
	return &row, nil
}

func TestMediaFromRepositoryLocatesWorkspaceMediaOnly(t *testing.T) {
	rows := mediaRows{
		"a": {ID: "a", WorkspaceID: "ws1", URL: "https://cdn.vozko.test/a.webp", Type: media.MediaTypeProductImage},
		"b": {ID: "b", WorkspaceID: "ws2", URL: "https://cdn.vozko.test/b.webp", Type: media.MediaTypeProductImage},
		"c": {ID: "c", WorkspaceID: "ws1", URL: "https://cdn.vozko.test/c.csv", Type: media.MediaTypeLeadImport},
	}
	got, err := MediaFromRepository(rows).Locate(context.Background(), "ws1", []string{"a", "b", "c", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["a"] != "https://cdn.vozko.test/a.webp" {
		t.Fatalf("got %+v", got)
	}
	if _, err := MediaFromRepository(rows).Locate(context.Background(), "ws1", []string{"broken"}); !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
}
