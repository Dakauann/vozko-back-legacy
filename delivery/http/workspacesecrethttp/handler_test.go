package workspacesecrethttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"vozko/domain/auth"
	"vozko/domain/workspacesecret"
	"vozko/domain/workspace"
	"vozko/infra/http/middleware"
)

type fakeSecrets struct {
	put       []string
	deleted   []string
	summaries []workspacesecret.Summary
	err       error
}

func (f *fakeSecrets) Put(_ context.Context, workspaceID, userID, name, value string) (workspacesecret.Summary, error) {
	f.put = append(f.put, workspaceID, userID, name, value)
	return workspacesecret.Summary{Name: name, UpdatedBy: userID, UpdatedAt: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)}, f.err
}

func (f *fakeSecrets) Delete(_ context.Context, workspaceID, name string) error {
	f.deleted = append(f.deleted, workspaceID, name)
	return f.err
}

func (f *fakeSecrets) List(context.Context, string) ([]workspacesecret.Summary, error) {
	return f.summaries, f.err
}

func (f *fakeSecrets) Resolve(context.Context, string, []string) (map[string]string, error) {
	return map[string]string{"never": "returned"}, nil
}

func authed(req *http.Request, vars map[string]string) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.ClaimsContextKey, &auth.Claims{UserID: "u-1", Role: "member"})
	ctx = context.WithValue(ctx, middleware.WorkspaceIDContextKey, "ws-1")
	return mux.SetURLVars(req.WithContext(ctx), vars)
}

func TestPutStoresTheValueAndAnswersWithoutIt(t *testing.T) {
	fake := &fakeSecrets{}
	rec := httptest.NewRecorder()
	req := authed(httptest.NewRequest(http.MethodPut, "/workflow-secrets/senha", strings.NewReader(`{"value":"p4ss"}`)), map[string]string{"name": "senha"})
	NewHandler(fake).Put(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Join(fake.put, "|") != "ws-1|u-1|senha|p4ss" {
		t.Fatalf("put %v", fake.put)
	}
	if strings.Contains(rec.Body.String(), "p4ss") {
		t.Fatalf("a value must never come back: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"senha"`) {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestPutRejectsAMalformedBody(t *testing.T) {
	fake := &fakeSecrets{}
	rec := httptest.NewRecorder()
	NewHandler(fake).Put(rec, authed(httptest.NewRequest(http.MethodPut, "/workflow-secrets/senha", strings.NewReader(`nope`)), map[string]string{"name": "senha"}))
	if rec.Code != http.StatusBadRequest || len(fake.put) != 0 {
		t.Fatalf("status %d put %v", rec.Code, fake.put)
	}
}

func TestListReturnsNamesOnly(t *testing.T) {
	fake := &fakeSecrets{summaries: []workspacesecret.Summary{{Name: "api_key", UpdatedBy: "u-1", UpdatedAt: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)}}}
	rec := httptest.NewRecorder()
	NewHandler(fake).List(rec, authed(httptest.NewRequest(http.MethodGet, "/workflow-secrets", nil), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body SecretListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Secrets) != 1 || body.Secrets[0].Name != "api_key" || body.Secrets[0].UpdatedBy != "u-1" {
		t.Fatalf("body %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "value") {
		t.Fatalf("listing must not carry values: %s", rec.Body.String())
	}
}

func TestDeleteAnswersNoContent(t *testing.T) {
	fake := &fakeSecrets{}
	rec := httptest.NewRecorder()
	NewHandler(fake).Delete(rec, authed(httptest.NewRequest(http.MethodDelete, "/workflow-secrets/senha", nil), map[string]string{"name": "senha"}))
	if rec.Code != http.StatusNoContent || strings.Join(fake.deleted, "|") != "ws-1|senha" {
		t.Fatalf("status %d deleted %v", rec.Code, fake.deleted)
	}
}

func TestDomainErrorsMapToStatusCodes(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{workspacesecret.ErrInvalidName, http.StatusBadRequest},
		{workspacesecret.ErrValueRequired, http.StatusBadRequest},
		{workspacesecret.ErrValueTooLong, http.StatusBadRequest},
		{workspacesecret.ErrWorkspaceRequired, http.StatusBadRequest},
		{workspacesecret.ErrNotFound, http.StatusNotFound},
		{errors.New("db down"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.err.Error(), func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewHandler(&fakeSecrets{err: c.err}).Delete(rec, authed(httptest.NewRequest(http.MethodDelete, "/workflow-secrets/x", nil), map[string]string{"name": "x"}))
			if rec.Code != c.want {
				t.Fatalf("got %d want %d", rec.Code, c.want)
			}
		})
	}
}

func TestEveryRouteOnBothPathsNeedsAgentOrWorkflowEditing(t *testing.T) {
	want := []workspace.PermissionEntry{
		{Resource: workspace.ResourceAgents, Action: workspace.ActionUpdate},
		{Resource: workspace.ResourceWorkflows, Action: workspace.ActionUpdate},
	}
	reached := map[string]bool{}
	anyOf := func(grants []workspace.PermissionEntry, h http.HandlerFunc) http.HandlerFunc {
		if !reflect.DeepEqual(grants, want) {
			t.Fatalf("grants %v", grants)
		}
		return func(w http.ResponseWriter, r *http.Request) {
			reached[r.Method+" "+r.URL.Path] = true
		}
	}
	router := mux.NewRouter()
	RegisterRoutes(router, NewHandler(&fakeSecrets{}), anyOf)
	for _, base := range []string{"/workspace-secrets", "/workflow-secrets"} {
		for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			path := base + "/x"
			if m == http.MethodGet {
				path = base
			}
			router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(m, path, nil))
			if !reached[m+" "+path] {
				t.Errorf("%s %s is not routed through the grant check", m, path)
			}
		}
	}
}
