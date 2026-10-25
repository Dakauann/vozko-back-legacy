package agentscenariohttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	"vozko/delivery/http/response/responsetest"
	"vozko/domain/agent"
	"vozko/domain/agentscenario"
	"vozko/domain/auth"
	"vozko/domain/shared"
	"vozko/infra/http/middleware"
	"vozko/pkg/codegolden"
	agentscenario_usecase "vozko/usecases/agentscenario"
)

type fakeScenarios struct {
	Scenarios
	draft  agentscenario_usecase.Draft
	by     string
	person shared.Person
	err    error
}

func (f *fakeScenarios) Create(_, _ string, d agentscenario_usecase.Draft, by string) (*agentscenario.Scenario, error) {
	f.draft, f.by = d, by
	return &agentscenario.Scenario{ID: "s-1", Name: d.Name}, f.err
}

func (f *fakeScenarios) Run(context.Context, string, string, string) (*agentscenario.Run, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &agentscenario.Run{ID: "r-1", Passed: true}, nil
}

func (f *fakeScenarios) FromRun(person shared.Person, _, _, runID string) (*agentscenario.Scenario, error) {
	f.person = person
	return &agentscenario.Scenario{SourceRunID: runID}, f.err
}

type owner struct{ ws string }

func (o owner) Execute(id string) (*agent.Agent, error) {
	return &agent.Agent{ID: id, WorkspaceID: o.ws}, nil
}

func request(method, path string, body interface{}, vars map[string]string) *http.Request {
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	ctx := context.WithValue(r.Context(), middleware.ClaimsContextKey, &auth.Claims{UserID: "u-1", Role: "user"})
	ctx = context.WithValue(ctx, middleware.WorkspaceIDContextKey, "ws-1")
	return mux.SetURLVars(r.WithContext(ctx), vars)
}

func TestCreatingAScenarioPassesTheDraftAndTheAuthor(t *testing.T) {
	f := &fakeScenarios{}
	rec := httptest.NewRecorder()
	NewHandler(f, owner{ws: "ws-1"}).Create(rec, request(http.MethodPost, "/agents/ag-1/scenarios", map[string]interface{}{
		"name": "Pedido", "messages": []map[string]string{{"content": "oi"}},
		"mocks":      map[string]interface{}{"t1": map[string]interface{}{"result": "ok"}},
		"assertions": []map[string]string{{"kind": "tool_called", "toolId": "t1"}},
	}, map[string]string{"id": "ag-1"}))
	if rec.Code != http.StatusCreated || f.draft.Name != "Pedido" || f.by != "u-1" || f.draft.Mocks["t1"].Result != "ok" || f.draft.Assertions[0].ToolID != "t1" {
		t.Fatalf("status %d draft %+v by %q", rec.Code, f.draft, f.by)
	}
}

func TestScenarioLimitsAnswerWithTheirCodes(t *testing.T) {
	for err, code := range map[error]string{
		agentscenario.ErrTooLong:      "agent_scenario_too_long",
		agentscenario.ErrMockTooLarge: "agent_scenario_mock_too_large",
		agentscenario.ErrInvalid:      "agent_scenario_invalid",
		agentscenario.ErrNotFound:     "agent_scenario_not_found",
		errors.New("db down"):         "agent_scenario_failed",
	} {
		rec := httptest.NewRecorder()
		NewHandler(&fakeScenarios{err: err}, owner{ws: "ws-1"}).Run(rec, request(http.MethodPost, "/agents/ag-1/scenarios/s-1/run", nil, map[string]string{"id": "ag-1", "scenarioId": "s-1"}))
		if responsetest.CodeOf(t, rec) != code {
			t.Errorf("%v: status %d body %s", err, rec.Code, rec.Body.String())
		}
	}
}

func TestReplayingARunUsesTheCallerForVisibility(t *testing.T) {
	f := &fakeScenarios{}
	rec := httptest.NewRecorder()
	NewHandler(f, owner{ws: "ws-1"}).FromRun(rec, request(http.MethodPost, "/agents/ag-1/runs/run-1/scenario", nil, map[string]string{"id": "ag-1", "runId": "run-1"}))
	if rec.Code != http.StatusOK || f.person.UserID != "u-1" {
		t.Fatalf("status %d person %+v", rec.Code, f.person)
	}
}

func TestAnotherWorkspacesAgentScenariosAreForbidden(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(&fakeScenarios{}, owner{ws: "ws-2"}).Run(rec, request(http.MethodPost, "/agents/ag-1/scenarios/s-1/run", nil, map[string]string{"id": "ag-1", "scenarioId": "s-1"}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestScenarioErrorCodesMatchTheGoldenSoTheFrontTranslatesThem(t *testing.T) {
	responsetest.RequireEveryErrorWritesItsCode(t, Refusals, writeError)
	codes := response.RefusalCodes(response.ErrorRefusalsOf(Refusals), []response.Refusal{refusalFailed, refusalInvalidBody})
	codegolden.Compare(t, filepath.Join("testdata", "error_codes.golden"), codes, "update testdata/error_codes.golden, add the code to vozko-front src/lib/agents/error-codes.ts and translate agents.errors.<code> in the pt, en, es and de catalogs")
}
