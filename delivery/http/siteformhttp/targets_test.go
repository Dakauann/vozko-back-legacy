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
	"vozko/domain/siteform"
	workspace_domain "vozko/domain/workspace"
	"vozko/infra/http/middleware"
	"vozko/usecases/siteforms"
)

func (f *fakeForms) FormTargets(_ context.Context, workspaceID string) (siteforms.FormTargets, error) {
	f.workspace = workspaceID
	return f.targets, f.err
}

func targetsRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/sites/forms/targets", nil)
	return req.WithContext(context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, "w1"))
}

func TestFormTargetsTellTheEditorWhichLeadFieldsAFieldCanFill(t *testing.T) {
	forms := &fakeForms{targets: siteforms.FormTargets{
		Targets:    []siteforms.FormTarget{{Target: "lead.phone", Kinds: []siteform.Kind{siteform.KindTel, siteform.KindText}}, {Target: "field.cidade", Label: "Cidade", Custom: true, Kinds: []siteform.Kind{siteform.KindText}}},
		Settable:   []siteform.Kind{siteform.KindText},
		Insertable: []siteform.Kind{siteform.KindText, siteform.KindTextarea},
	}}
	var guarded []string
	r := mux.NewRouter()
	RegisterProtectedRoutes(r, NewHandler(forms), func(res workspace_domain.Resource, act workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		guarded = append(guarded, string(res)+":"+string(act))
		return next
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, targetsRequest())
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `{"target":"lead.phone","custom":false,"kinds":["tel","text"]}`) || !strings.Contains(body, `"label":"Cidade"`) || !strings.Contains(body, `"insertable":["text","textarea"]`) {
		t.Fatalf("targets %d %s", rec.Code, body)
	}
	if forms.workspace != "w1" || !strings.Contains(strings.Join(guarded, ","), "sites:read") {
		t.Fatalf("workspace %q guards %v", forms.workspace, guarded)
	}
}

func TestFormTargetsRefuseWhenTheLeadFieldsCannotBeRead(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(&fakeForms{err: errors.New("db down")}).FormTargets(rec, targetsRequest())
	if rec.Code != http.StatusInternalServerError || responsetest.CodeOf(t, rec) != CodeInternal {
		t.Fatalf("failure %d %s", rec.Code, rec.Body.String())
	}
}
