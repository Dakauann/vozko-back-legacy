package conversation_repository

import (
	"context"
	"errors"
	"testing"
)

func TestAnUnreadablePageProjectIsAnError(t *testing.T) {
	db, mock := pageOriginMock(t)
	mock.ExpectQuery(`FROM site_projects WHERE id = .* AND workspace_id = `).WillReturnError(errors.New("down"))
	if got, err := NewPageProjects(db).PageProject(context.Background(), "w", "p"); err == nil || got != nil {
		t.Fatalf("got %+v %v", got, err)
	}
}
