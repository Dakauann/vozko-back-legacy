package agentpublishhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response/responsetest"
	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/auth"
	"vozko/domain/lint"
	"vozko/domain/workspace"
	"vozko/infra/http/middleware"
	agent_usecase "vozko/usecases/agent"
)

type fakePublishing struct {
	agent_usecase.AgentPublishing
	err error
	by  workspace.Requester
}

func (f *fakePublishing) Publish(_ context.Context, _ string, by workspace.Requester) (agent.Version, error) {
	f.by = by
	return agent.Version{Version: 4, Snapshot: agent.Snapshot{Tools: []agent.ToolPin{{ID: "t1", Revision: 2}}}}, f.err
}

func (f *fakePublishing) Publication(context.Context, string) (agent.Publication, error) {
	v := 4
	return agent.Publication{PublishedVersion: &v, HasDraftChanges: true}, f.err
}

type owner struct{ ws string }

func (o owner) Execute(id string) (*agent.Agent, error) { return &agent.Agent{ID: id, WorkspaceID: o.ws}, nil }

func call(h http.HandlerFunc, method string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/agents/ag-1/publish", nil)
	ctx := context.WithValue(r.Context(), middleware.ClaimsContextKey, &auth.Claims{UserID: "u-1"})
	ctx = context.WithValue(ctx, middleware.WorkspaceIDContextKey, "ws-1")
	rec := httptest.NewRecorder()
	h(rec, mux.SetURLVars(r.WithContext(ctx), map[string]string{"id": "ag-1"}))
	return rec
}

func TestPublishingAnswersTheNewVersion(t *testing.T) {
	f := &fakePublishing{}
	rec := call(NewHandler(f, owner{ws: "ws-1"}, nil).Publish, http.MethodPost)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"version":4`) || f.by.UserID != "u-1" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestABlockedDraftAnswers422WithTheIssues(t *testing.T) {
	report := agenttool.Report{Issues: []agenttool.Issue{{Code: "http_url_not_https", Severity: lint.SeverityBlocking, ToolID: "t1", Message: "x"}}}
	rec := call(NewHandler(&fakePublishing{err: agenttool.BlockedBy(agenttool.ErrNotPublishable, report)}, owner{ws: "ws-1"}, nil).Publish, http.MethodPost)
	if rec.Code != http.StatusUnprocessableEntity || responsetest.CodeOf(t, rec) != "agent_not_publishable" || !strings.Contains(rec.Body.String(), "http_url_not_https") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestPublicationOfAnotherWorkspaceIsForbidden(t *testing.T) {
	rec := call(NewHandler(&fakePublishing{}, owner{ws: "ws-2"}, nil).Publication, http.MethodGet)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}
	rec = call(NewHandler(&fakePublishing{}, owner{ws: "ws-1"}, nil).Publication, http.MethodGet)
	if !strings.Contains(rec.Body.String(), `"hasUnpublishedChanges":true`) {
		t.Fatalf("body %s", rec.Body.String())
	}
}
