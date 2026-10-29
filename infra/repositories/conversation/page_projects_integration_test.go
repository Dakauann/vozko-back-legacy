package conversation_repository

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func TestAPageProjectIsOnlyReadInsideItsWorkspace(t *testing.T) {
	db := repotest.IsolatedDB(t, "pageproject_test", &schema.SiteProject{})
	ctx := context.Background()
	workspace, live, draft := uuid.NewString(), uuid.NewString(), uuid.NewString()
	repotest.InsertRow(t, db, "site_projects", map[string]interface{}{"id": live, "workspace_id": workspace, "name": "Loja de outubro", "slug": "loja", "stack": "react", "status": "published"})
	repotest.InsertRow(t, db, "site_projects", map[string]interface{}{"id": draft, "workspace_id": workspace, "name": "Rascunho", "slug": "rascunho", "stack": "react", "status": "draft"})
	projects := NewPageProjects(db)

	got, err := projects.PageProject(ctx, workspace, live)
	if err != nil || got == nil || got.Name != "Loja de outubro" || !got.Live {
		t.Fatalf("live = %+v %v", got, err)
	}
	got, err = projects.PageProject(ctx, workspace, draft)
	if err != nil || got == nil || got.Name != "Rascunho" || got.Live {
		t.Fatalf("draft = %+v %v", got, err)
	}
	got, err = projects.PageProject(ctx, uuid.NewString(), live)
	if err != nil || got != nil {
		t.Fatalf("another workspace = %+v %v", got, err)
	}
}
