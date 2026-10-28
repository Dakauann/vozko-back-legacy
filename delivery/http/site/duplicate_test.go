package sitehttp

import (
	"net/http"
	"testing"

	workspace_domain "vozko/domain/workspace"
)

func TestDuplicateCopiesThePageForThePerson(t *testing.T) {
	svc := newFake()
	r, grants := router(svc)
	rec := call(r, http.MethodPost, "/sites/projects/p-1/duplicate", `{"name":"Promo"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if svc.duplicateIn.ProjectID != "p-1" || svc.duplicateIn.Name != "Promo" || svc.duplicateIn.UserID != testUser || svc.duplicateIn.WorkspaceID != testWS {
		t.Fatalf("input %+v", svc.duplicateIn)
	}
	if got := grants["POST /sites/projects/{id}/duplicate"]; got != (grant{workspace_domain.ResourceSites, workspace_domain.ActionCreate}) {
		t.Fatalf("guarded by %+v", got)
	}
	if rec := call(r, http.MethodPost, "/sites/projects/p-1/duplicate", "", nil); rec.Code != http.StatusCreated {
		t.Fatalf("an empty body keeps the default name: %d %s", rec.Code, rec.Body.String())
	}
}
