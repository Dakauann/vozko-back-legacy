package sitehttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/gorilla/mux"

	"vozko/domain/sitenext"
	workspace_domain "vozko/domain/workspace"
	site_usecase "vozko/usecases/site"
	sitenext_usecase "vozko/usecases/sitenext"
)

type fakeNextSteps struct {
	result    sitenext_usecase.Result
	err       error
	workspace string
	project   string
}

func (f *fakeNextSteps) Next(_ context.Context, workspaceID, projectID string) (sitenext_usecase.Result, error) {
	f.workspace, f.project = workspaceID, projectID
	return f.result, f.err
}

func nextStepsRouter(steps NextSteps) (*mux.Router, map[string]grant) {
	r := mux.NewRouter()
	grants := map[string]grant{}
	ac := func(res workspace_domain.Resource, action workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			tmpl, _ := mux.CurrentRoute(req).GetPathTemplate()
			grants[req.Method+" "+tmpl] = grant{res, action}
			next(w, req)
		}
	}
	RegisterNextStepsRoutes(r, NewNextStepsHandler(steps), ac)
	return r, grants
}

func TestNextStepsAnswersTheStepsOfThePageForAnEditor(t *testing.T) {
	steps := &fakeNextSteps{result: sitenext_usecase.Result{Checked: true, Steps: []sitenext.Step{
		{Code: sitenext.CodeAddBooking, Label: "Adicionar um bloco de agendamento", Action: sitenext.Action{Kind: sitenext.ActionPrompt, Prompt: "Adicione um bloco"}},
		{Code: sitenext.CodeSiteChat, Label: "Adicionar o chat do site", Action: sitenext.Action{Kind: sitenext.ActionSettings, Tab: sitenext.TabIntegrations}},
		{Code: sitenext.CodePublish, Label: "Publicar o site", Action: sitenext.Action{Kind: sitenext.ActionPublish}},
	}}}
	r, grants := nextStepsRouter(steps)
	rec := call(r, http.MethodGet, "/sites/projects/p-1/next-steps", "", nil)
	if rec.Code != http.StatusOK || grants["GET /sites/projects/{id}/next-steps"] != (grant{workspace_domain.ResourceSites, workspace_domain.ActionUpdate}) {
		t.Fatalf("got %d %s %v", rec.Code, rec.Body.String(), grants)
	}
	if steps.workspace != testWS || steps.project != "p-1" {
		t.Fatalf("scope %s %s", steps.workspace, steps.project)
	}
	var body NextStepsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Checked || len(body.Steps) != 3 {
		t.Fatalf("body %+v", body)
	}
	if got := body.Steps[0]; got.Code != "add_booking" || got.Action.Kind != "prompt" || got.Action.Prompt != "Adicione um bloco" || got.Action.Tab != "" {
		t.Fatalf("prompt step %+v", got)
	}
	if got := body.Steps[1]; got.Action.Kind != "settings" || got.Action.Tab != "integrations" {
		t.Fatalf("settings step %+v", got)
	}
	if got := body.Steps[2]; got.Action.Kind != "publish" {
		t.Fatalf("publish step %+v", got)
	}
}

func TestNextStepsAlwaysSendsAList(t *testing.T) {
	r, _ := nextStepsRouter(&fakeNextSteps{result: sitenext_usecase.Result{}})
	rec := call(r, http.MethodGet, "/sites/projects/p-1/next-steps", "", nil)
	if rec.Code != http.StatusOK || rec.Body.String() == "" {
		t.Fatalf("got %d", rec.Code)
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	if string(raw["steps"]) != "[]" || string(raw["checked"]) != "false" {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestNextStepsErrorsCarryCodes(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{site_usecase.ErrProjectNotFound, http.StatusNotFound, "project_not_found"},
		{errors.Join(sitenext_usecase.ErrUnavailable, errors.New("db")), http.StatusServiceUnavailable, CodeNextStepsUnavailable},
		{errors.New("boom"), http.StatusInternalServerError, CodeInternal},
	}
	for _, tc := range cases {
		r, _ := nextStepsRouter(&fakeNextSteps{err: tc.err})
		rec := call(r, http.MethodGet, "/sites/projects/p-1/next-steps", "", nil)
		if rec.Code != tc.status || decodeError(t, rec).Code != tc.code {
			t.Fatalf("%v got %d %s", tc.err, rec.Code, rec.Body.String())
		}
	}
}
