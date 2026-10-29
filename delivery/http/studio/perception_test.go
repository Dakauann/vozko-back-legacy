package studiohttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"vozko/domain/auth"
	workspace_domain "vozko/domain/workspace"
	"vozko/infra/http/middleware"
	studio_usecase "vozko/usecases/studio"
)

const shapesSurface = `{"canvas":{"width":1080,"height":1080,"background":"#ffffff"},"layers":[{"id":"a","type":"shape","shape":"rect","fill":"#111111","transform":{"x":0.5,"y":0.5,"w":0.3,"h":0.3,"rotation":0,"opacity":1},"effects":[{"id":"d","type":"drop-shadow","color":"#000000","opacity":1,"x":0,"y":30,"blur":0,"spread":0}]}],"texts":{}}`

func servePerception(t *testing.T, analyzer PerceptionAnalyzer, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := mux.NewRouter()
	granted := workspace_domain.Action("")
	allow := func(resource workspace_domain.Resource, action workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if resource == workspace_domain.ResourceMedia && strings.Contains(r.URL.Path, "/perception/") {
				granted = action
			}
			next(w, r)
		}
	}
	handler := NewHandler(&fakeService{}, &fakeCapabilities{}, &fakeExports{}, &fakePresets{})
	if analyzer != nil {
		handler = handler.WithPerception(analyzer)
	}
	RegisterProtectedRoutes(router, handler, allow)
	req := httptest.NewRequest(http.MethodPost, "/studio/perception/analyze", strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.ClaimsContextKey, &auth.Claims{UserID: "u-1"})
	ctx = context.WithValue(ctx, middleware.WorkspaceIDContextKey, "ws-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(ctx))
	if granted != workspace_domain.ActionRead {
		t.Fatalf("the analysis asked for %q", granted)
	}
	return rec
}

func TestPerceptionAnswersTheShapesOfEveryScene(t *testing.T) {
	rec := servePerception(t, studio_usecase.NewPerceptionService(), `{"approximate":false,"layers":{"depth":true,"layout":true},"surfaces":[`+shapesSurface+`]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d %s", rec.Code, rec.Body.String())
	}
	var body PerceptionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Shapes) != 1 || len(body.Shapes[0]) == 0 {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPerceptionRefusesFactsItCannotReadWithACode(t *testing.T) {
	for _, body := range []string{`{"layers":{"depth":true}}`, `not json`, `{"approximate":false,"layers":{"depth":false,"layout":true},"surfaces":[{"canvas":{"width":100,"height":100,"background":"#fff"},"layers":[{"id":"t","type":"text","text":"Oi","transform":{"x":0.5,"y":0.5,"w":0.5,"h":0.2,"rotation":0,"opacity":1}}],"texts":{}}]}`} {
		rec := servePerception(t, studio_usecase.NewPerceptionService(), body)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"invalid_facts"`) {
			t.Fatalf("body %s answered %d %s", body, rec.Code, rec.Body.String())
		}
	}
}

func TestPerceptionTooLargeAnswersItsOwnCode(t *testing.T) {
	huge := `{"approximate":false,"layers":{"depth":true,"layout":false},"surfaces":[{"canvas":{"width":100,"height":100,"background":"` + strings.Repeat("a", maxPerceptionBytes) + `"},"layers":[],"texts":{}}]}`
	rec := servePerception(t, studio_usecase.NewPerceptionService(), huge)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), `"facts_too_large"`) {
		t.Fatalf("code = %d %s", rec.Code, rec.Body.String())
	}
}

func TestPerceptionWithoutTheAnalyzerIsUnavailable(t *testing.T) {
	rec := servePerception(t, nil, `{}`)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"unavailable"`) {
		t.Fatalf("code = %d %s", rec.Code, rec.Body.String())
	}
}

var _ PerceptionAnalyzer = (*studio_usecase.PerceptionService)(nil)