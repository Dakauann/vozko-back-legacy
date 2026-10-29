package webchat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"vozko/domain/sitepublish"
	wcdomain "vozko/domain/webchat"
	wcuc "vozko/usecases/webchat"
)

const previewOrigin = "https://preview--abc.vozko.site"

type previewGrants map[string]sitepublish.PreviewGrant

func (p previewGrants) PreviewOrigin(origin string) bool {
	_, ok := sitepublish.Addresses{Domain: "vozko.site"}.PreviewOriginHost(origin)
	return ok
}

func (p previewGrants) PreviewGrant(_ context.Context, origin string) (sitepublish.PreviewGrant, bool, error) {
	g, ok := p[origin]
	return g, ok, nil
}

func demoRouter(t *testing.T, w *wcdomain.Widget) (*mux.Router, wcdomain.Keys) {
	t.Helper()
	keys, err := wcdomain.DeriveKeys("test-root")
	if err != nil {
		t.Fatal(err)
	}
	svc := wcuc.NewVisitorService(wcuc.VisitorDeps{
		Widgets:        widgetStore{w: w},
		Keys:           keys,
		Previews:       previewGrants{previewOrigin: {WorkspaceID: w.WorkspaceID, ProjectID: "p1", Origin: previewOrigin, Expires: time.Now().Add(time.Hour)}},
		PreviewEditors: []string{"https://app.vozko.com"},
	})
	h, err := NewPublicHandler(svc, nil, apiOrigin, testSolver)
	if err != nil {
		t.Fatal(err)
	}
	r := mux.NewRouter()
	RegisterPublicRoutes(r, h)
	return r, keys
}

func TestAPreviewFrameIsADemoEmbeddableOnlyByThePreviewAndTheEditor(t *testing.T) {
	r, _ := demoRouter(t, activeWidget())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, PublicPrefix+"/pk1/frame?origin="+url.QueryEscape(previewOrigin), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors "+previewOrigin+" https://app.vozko.com") || strings.Contains(csp, "loja.example.com") {
		t.Fatalf("csp = %q", csp)
	}
	if !strings.Contains(rec.Body.String(), `data-demo="true"`) {
		t.Fatal("the preview frame does not know it is a demo")
	}
}

func TestAPreviewFrameWithoutAValidGrantIsNotServed(t *testing.T) {
	r, _ := demoRouter(t, activeWidget())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, PublicPrefix+"/pk1/frame?origin="+url.QueryEscape("https://preview--forged.vozko.site"), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAPublishedFrameIsNoDemo(t *testing.T) {
	r, _ := demoRouter(t, activeWidget())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, PublicPrefix+"/pk1/frame?origin="+url.QueryEscape("https://loja.example.com"), nil))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `data-demo="true"`) {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors https://loja.example.com https://*.example.org") {
		t.Fatalf("csp = %q", rec.Header().Get("Content-Security-Policy"))
	}
}

func TestADemoStreamOnlyPingsAndNeverSubscribes(t *testing.T) {
	w := activeWidget()
	r, keys := demoRouter(t, w)
	token, err := wcdomain.NewDemoToken(keys, wcdomain.VisitorGrant{WidgetID: w.ID, VisitorID: "demo-1", ExpiresAt: time.Now().Add(time.Hour)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, PublicPrefix+"/session/stream", nil).WithContext(ctx)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "event: ping") {
		t.Fatalf("stream = %d %q", rec.Code, rec.Body.String())
	}
}
