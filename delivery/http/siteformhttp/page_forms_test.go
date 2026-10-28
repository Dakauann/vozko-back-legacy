package siteformhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response/responsetest"
	workspace_domain "vozko/domain/workspace"
	"vozko/infra/http/middleware"
	"vozko/usecases/siteforms"
)

func (f *fakeForms) PageForms(_ context.Context, workspaceID string) ([]siteforms.PageForm, error) {
	f.workspace = workspaceID
	return f.pageForms, f.err
}

func pageFormsRequest(workspaceID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/sites/forms", nil)
	if workspaceID != "" {
		req = req.WithContext(context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, workspaceID))
	}
	return req
}

func TestPageFormsAreListedByKeyForTheWorkflowPicker(t *testing.T) {
	forms := &fakeForms{pageForms: []siteforms.PageForm{{ProjectID: "p1", ProjectName: "Clínica Sorriso", FormID: "contato", FormKey: "key-1", Live: true}}}
	var guarded []string
	r := mux.NewRouter()
	RegisterProtectedRoutes(r, NewHandler(forms), func(res workspace_domain.Resource, act workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		guarded = append(guarded, string(res)+":"+string(act))
		return next
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, pageFormsRequest("w1"))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"formKey":"key-1"`) || !strings.Contains(body, `"projectName":"Clínica Sorriso"`) || !strings.Contains(body, `"live":true`) {
		t.Fatalf("list %d %s", rec.Code, body)
	}
	if forms.workspace != "w1" {
		t.Fatalf("workspace %q", forms.workspace)
	}
	if !strings.Contains(strings.Join(guarded, ","), "sites:read") {
		t.Fatalf("guards %v", guarded)
	}
}

func TestPageFormsAnswerAnEmptyListAndRefuseOnStoreFailure(t *testing.T) {
	h := NewHandler(&fakeForms{})
	rec := httptest.NewRecorder()
	h.PageForms(rec, pageFormsRequest("w1"))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("empty %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	NewHandler(&fakeForms{err: errors.New("db down")}).PageForms(rec, pageFormsRequest("w1"))
	if rec.Code != http.StatusInternalServerError || responsetest.CodeOf(t, rec) != CodeInternal {
		t.Fatalf("failure %d %s", rec.Code, rec.Body.String())
	}
}
