package siteformhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"vozko/domain/auth"
	workspace_domain "vozko/domain/workspace"
	"vozko/infra/http/middleware"
	"vozko/usecases/siteforms"
)

func (f *fakeForms) Automations(_ context.Context, workspaceID, projectID, formID string) (siteforms.FormAutomations, error) {
	f.workspace = workspaceID + "|" + projectID + "|" + formID
	return f.automations, f.err
}

func (f *fakeForms) ScaffoldAutomation(_ context.Context, workspaceID, userID, projectID, formID string) (string, error) {
	f.workspace = workspaceID + "|" + userID + "|" + projectID + "|" + formID
	return "wf-new", f.err
}

func automationsRequest(method string, claims *auth.Claims) *http.Request {
	req := httptest.NewRequest(method, "/sites/projects/p1/forms/contato/automations", nil)
	ctx := context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, "w1")
	if claims != nil {
		ctx = context.WithValue(ctx, middleware.ClaimsContextKey, claims)
	}
	return req.WithContext(ctx)
}

func automationsRouter(forms *fakeForms, guarded *[]string) *mux.Router {
	r := mux.NewRouter()
	RegisterProtectedRoutes(r, NewHandler(forms), func(res workspace_domain.Resource, act workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		*guarded = append(*guarded, string(res)+":"+string(act))
		return next
	})
	return r
}

func TestPageFormAutomationsListWhatRunsOnSubmitAndScaffoldNeedsWorkflowCreation(t *testing.T) {
	forms := &fakeForms{automations: siteforms.FormAutomations{FormKey: "key-1", Automations: []siteforms.FormAutomation{{WorkflowID: "wf-1", Name: "Boas vindas", Active: true, Use: "trigger"}}}}
	var guarded []string
	r := automationsRouter(forms, &guarded)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, automationsRequest(http.MethodGet, nil))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"formKey":"key-1"`) || !strings.Contains(body, `"workflowId":"wf-1"`) || !strings.Contains(body, `"active":true`) || forms.workspace != "w1|p1|contato" {
		t.Fatalf("list %d %s %s", rec.Code, body, forms.workspace)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, automationsRequest(http.MethodPost, &auth.Claims{UserID: "u1", Role: "user"}))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"workflowId":"wf-new"`) || forms.workspace != "w1|u1|p1|contato" {
		t.Fatalf("scaffold %d %s %s", rec.Code, rec.Body.String(), forms.workspace)
	}
	joined := strings.Join(guarded, ",")
	if !strings.Contains(joined, "sites:read") || !strings.Contains(joined, "workflows:create") {
		t.Fatalf("guards %v", guarded)
	}
}

func TestPageFormAutomationsRefuseWithCodes(t *testing.T) {
	var guarded []string
	r := automationsRouter(&fakeForms{}, &guarded)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, automationsRequest(http.MethodPost, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous scaffold %d", rec.Code)
	}
	for err, want := range map[error]string{
		siteforms.ErrProjectNotFound: "project_not_found",
		siteforms.ErrFormIDInvalid:   "form_id_invalid",
		errors.New("boom"):           "internal_error",
	} {
		rec := httptest.NewRecorder()
		automationsRouter(&fakeForms{err: err}, &guarded).ServeHTTP(rec, automationsRequest(http.MethodGet, nil))
		if !strings.Contains(rec.Body.String(), `"`+want+`"`) {
			t.Fatalf("%v: %d %s", err, rec.Code, rec.Body.String())
		}
	}
}
