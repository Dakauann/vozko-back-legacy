package agentrunhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	"vozko/delivery/http/response/responsetest"
	"vozko/domain/agent"
	"vozko/domain/agentrun"
	aa "vozko/domain/ai_attendance"
	"vozko/domain/auth"
	"vozko/domain/shared"
	"vozko/infra/http/middleware"
	"vozko/pkg/codegolden"
)

type fakeRuns struct {
	filter agentrun.Filter
	person shared.Person
	page   shared.Pagination
	err    error
	detail *agentrun.Detail
}

func (f *fakeRuns) List(person shared.Person, _, _ string, filter agentrun.Filter, page shared.Pagination) (*shared.PaginatedResult[*agentrun.Run], error) {
	f.person, f.filter, f.page = person, filter, page
	if f.err != nil {
		return nil, f.err
	}
	return &shared.PaginatedResult[*agentrun.Run]{Items: []*agentrun.Run{{ID: "run-1", Status: agentrun.StatusOK}}, Page: 1, PageSize: 20, TotalItems: 1, TotalPages: 1}, nil
}

func (f *fakeRuns) Get(_ shared.Person, _, _, _ string) (*agentrun.Detail, error) {
	return f.detail, f.err
}

type owner struct{ ws string }

func (o owner) Execute(id string) (*agent.Agent, error) {
	return &agent.Agent{ID: id, WorkspaceID: o.ws}, nil
}

func request(path string, vars map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	ctx := context.WithValue(r.Context(), middleware.ClaimsContextKey, &auth.Claims{UserID: "u-1", Role: "user"})
	ctx = context.WithValue(ctx, middleware.WorkspaceIDContextKey, "ws-1")
	return mux.SetURLVars(r.WithContext(ctx), vars)
}

func TestListingRunsPassesTheFiltersAndTheCaller(t *testing.T) {
	runs := &fakeRuns{}
	rec := httptest.NewRecorder()
	NewHandler(runs, owner{ws: "ws-1"}).List(rec, request("/agents/ag-1/runs?source=conversation&status=tool_failed&toolId=t1&errorKind=timeout&handledFailure=true&from=2026-10-01T00:00:00Z&to=2026-10-02T00:00:00Z&page=2", map[string]string{"id": "ag-1"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	f := runs.filter
	if f.Source != agentrun.SourceConversation || f.Status != agentrun.StatusToolFailed || f.ToolInstanceID != "t1" || f.ErrorKind != "timeout" || !f.HandledFailure {
		t.Fatalf("filter %+v", f)
	}
	if !f.From.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) || runs.person.UserID != "u-1" || runs.page.Page != 2 {
		t.Fatalf("filter %+v person %+v page %+v", f, runs.person, runs.page)
	}
	var out struct {
		Data []agentrun.Run `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Data) != 1 || out.Data[0].ID != "run-1" {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestAnUnreadableDateIsRefusedWithItsCode(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(&fakeRuns{}, owner{ws: "ws-1"}).List(rec, request("/agents/ag-1/runs?from=ontem", map[string]string{"id": "ag-1"}))
	if rec.Code != http.StatusBadRequest || responsetest.CodeOf(t, rec) != "agent_run_filter_invalid" {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestAnInvalidFilterFromTheUseCaseKeepsItsCode(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(&fakeRuns{err: agentrun.ErrFilterInvalid}, owner{ws: "ws-1"}).List(rec, request("/agents/ag-1/runs?status=x", map[string]string{"id": "ag-1"}))
	if rec.Code != http.StatusBadRequest || responsetest.CodeOf(t, rec) != "agent_run_filter_invalid" {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestAnotherWorkspacesAgentRunsAreForbidden(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(&fakeRuns{}, owner{ws: "ws-2"}).List(rec, request("/agents/ag-1/runs", map[string]string{"id": "ag-1"}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestReadingARunReturnsItsSpans(t *testing.T) {
	runs := &fakeRuns{detail: &agentrun.Detail{Run: agentrun.Run{ID: "run-1"}, Spans: []aa.ToolCall{{ID: "s1", ToolName: "consultar_pedido"}}}}
	rec := httptest.NewRecorder()
	NewHandler(runs, owner{ws: "ws-1"}).Get(rec, request("/agents/ag-1/runs/run-1", map[string]string{"id": "ag-1", "runId": "run-1"}))
	var out agentrun.Detail
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil || len(out.Spans) != 1 || out.Spans[0].ToolName != "consultar_pedido" {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestAHiddenOrMissingRunAnswersNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(&fakeRuns{err: agentrun.ErrRunNotFound}, owner{ws: "ws-1"}).Get(rec, request("/agents/ag-1/runs/run-1", map[string]string{"id": "ag-1", "runId": "run-1"}))
	if rec.Code != http.StatusNotFound || responsetest.CodeOf(t, rec) != "agent_run_not_found" {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestAnUnexpectedRunErrorStillWritesACode(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(&fakeRuns{err: errors.New("db down")}, owner{ws: "ws-1"}).Get(rec, request("/agents/ag-1/runs/run-1", map[string]string{"id": "ag-1", "runId": "run-1"}))
	if rec.Code != http.StatusInternalServerError || responsetest.CodeOf(t, rec) != "agent_run_failed" {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestRunErrorCodesMatchTheGoldenSoTheFrontTranslatesThem(t *testing.T) {
	responsetest.RequireEveryErrorWritesItsCode(t, Refusals, writeError)
	codes := response.RefusalCodes(response.ErrorRefusalsOf(Refusals), []response.Refusal{refusalFailed, refusalFilterInvalid})
	codegolden.Compare(t, filepath.Join("testdata", "error_codes.golden"), codes, "update testdata/error_codes.golden, add the code to vozko-front src/lib/agents/error-codes.ts and translate agents.errors.<code> in the pt, en, es and de catalogs")
}
