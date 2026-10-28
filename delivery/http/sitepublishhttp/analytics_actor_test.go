package sitepublishhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/domain/auth"
	"vozko/domain/sitepublish"
	"vozko/domain/user"
	"vozko/infra/http/middleware"
	siteanalytics_usecase "vozko/usecases/siteanalytics"
)

type recordingAnalytics struct {
	fakeAnalytics
	actor siteanalytics_usecase.Actor
}

func (f *recordingAnalytics) Read(ctx context.Context, a siteanalytics_usecase.Actor, projectID, from, to string) (siteanalytics_usecase.Report, error) {
	f.actor = a
	return f.fakeAnalytics.Read(ctx, a, projectID, from, to)
}

func TestAnalyticsTellsWhetherTheViewerIsAPlatformAdminForTheDealScope(t *testing.T) {
	analytics := &recordingAnalytics{}
	r, _ := router(NewHandler(&fakePublisher{}, analytics, sitepublish.Addresses{Domain: "paginas.test"}))
	req := httptest.NewRequest(http.MethodGet, "/sites/projects/p1/analytics", nil)
	ctx := context.WithValue(req.Context(), middleware.ClaimsContextKey, &auth.Claims{UserID: "u-1", Role: string(user.RoleAdmin)})
	ctx = context.WithValue(ctx, middleware.WorkspaceIDContextKey, "ws-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req.WithContext(ctx))
	if w.Code != http.StatusOK || analytics.actor != (siteanalytics_usecase.Actor{UserID: "u-1", WorkspaceID: "ws-1", IsAdmin: true}) {
		t.Fatalf("got %d %+v", w.Code, analytics.actor)
	}
}
