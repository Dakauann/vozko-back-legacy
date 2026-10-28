package sitepublishhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/usecases/sitepublish"
)

func (f *fakePublisher) SettingsChanges(_ context.Context, a sitepublish_usecase.Actor, projectID string) (sitepublish.SettingsChanges, error) {
	f.actor, f.project = a, projectID
	if f.err != nil {
		return sitepublish.SettingsChanges{}, f.err
	}
	return sitepublish.DiffSettings(site.Settings{}, true, site.Settings{Redirects: []site.Redirect{{From: "/promo", To: "https://loja.example.com/", Status: 302}}}), nil
}

func TestSettingsChangesAreReadWithSitesRead(t *testing.T) {
	pub := &fakePublisher{}
	r, seen := router(NewHandler(pub, fakeAnalytics{}, sitepublish.Addresses{Domain: "paginas.test"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request(http.MethodGet, "/sites/projects/p1/settings-changes", ""))
	if w.Code != http.StatusOK || len(*seen) != 1 || (*seen)[0] != (grant{"sites", "read"}) || pub.project != "p1" || pub.actor.WorkspaceID != "ws-1" {
		t.Fatalf("got %d %v %s", w.Code, *seen, w.Body.String())
	}
	var got sitepublish.SettingsChanges
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v %s", err, w.Body.String())
	}
	if len(got.Redirects) != 1 || got.Redirects[0].Action != sitepublish.ChangeAdded || got.Redirects[0].To != "https://loja.example.com/" || got.Integrations == nil {
		t.Fatalf("got %+v from %s", got, w.Body.String())
	}
}

func TestSettingsChangesRefuseLikeTheOtherPublishReads(t *testing.T) {
	r, _ := router(NewHandler(&fakePublisher{err: sitepublish_usecase.ErrProjectNotFound}, fakeAnalytics{}, sitepublish.Addresses{Domain: "paginas.test"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request(http.MethodGet, "/sites/projects/p1/settings-changes", ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}
