package automationneedshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"vozko/domain/agent"
	domain "vozko/domain/automationneeds"
	workspace_domain "vozko/domain/workspace"
	"vozko/infra/http/middleware"
	automationneeds_usecase "vozko/usecases/automationneeds"
)

type fakeAgentRows struct {
	candidate *agent.Agent
	rows      []automationneeds_usecase.AgentVariable
	err       error
}

func (f *fakeAgentRows) AgentVariables(_ context.Context, _ string, candidate *agent.Agent) ([]automationneeds_usecase.AgentVariable, error) {
	f.candidate = candidate
	return f.rows, f.err
}

type fakeAgentDrafts map[string]*agent.Agent

func (f fakeAgentDrafts) Execute(id string) (*agent.Agent, error) {
	if found, ok := f[id]; ok {
		return found, nil
	}
	return nil, agent.ErrAgentNotFound
}

func serveAgentRows(t *testing.T, rows *fakeAgentRows, target, body string) (*httptest.ResponseRecorder, []string) {
	t.Helper()
	var asked []string
	gate := func(r workspace_domain.Resource, a workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		asked = append(asked, string(r)+":"+string(a))
		return next
	}
	router := mux.NewRouter()
	drafts := fakeAgentDrafts{
		"a1":    {ID: "a1", WorkspaceID: "ws-1", Name: "Matrículas", MessagingPrompt: "salvo", InternalTools: []agent.ToolBinding{{Name: "run_workflow"}}},
		"other": {ID: "other", WorkspaceID: "ws-2"},
	}
	RegisterAgentVariableRoutes(router, NewAgentVariablesHandler(drafts, rows), gate)
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, "ws-1")))
	return rec, asked
}

func TestTheAgentPanelReadsTheTypedCandidateOverTheSavedDraft(t *testing.T) {
	rows := &fakeAgentRows{rows: []automationneeds_usecase.AgentVariable{
		{Need: domain.Need{Name: "turno", Label: "Turno", Default: "Manhã", Declared: true}, InAgent: true, InPrompt: true, Used: true, FilledBy: []automationneeds_usecase.Filler{{Kind: automationneeds_usecase.FilledByDefault, Value: "Manhã"}}},
		{Need: domain.Need{Name: "lead_name", Label: "Nome do contato"}, Required: true, InAgent: true, Problem: agent.ErrAgentVariableNameReserved},
	}}
	rec, asked := serveAgentRows(t, rows, "/agents/a1/conversation-variables", `{"messagingPrompt":" Oi {{turno}} ","variables":[{"name":" turno ","defaultValue":"Manhã"},{"name":"lead_name"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if strings.Join(asked, ",") != "agents:read,agents:read" {
		t.Fatalf("asked = %v", asked)
	}
	c := rows.candidate
	if c.ID != "a1" || c.Name != "Matrículas" || c.MessagingPrompt != "Oi {{turno}}" || len(c.InternalTools) != 1 || c.Variables[0].Name != "turno" || len(c.Variables) != 2 {
		t.Fatalf("candidate = %+v", c)
	}
	var got AgentVariablesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	turno, platform := got.Variables[0], got.Variables[1]
	if turno.Label != "Turno" || turno.Required || !turno.InAgent || !turno.InPrompt || turno.Default != "Manhã" || turno.FilledBy[0].Kind != "default" || turno.Problem != "" {
		t.Fatalf("turno = %+v", turno)
	}
	if platform.Problem != "agent_variable_name_reserved" || platform.Used {
		t.Fatalf("lead_name = %+v", platform)
	}
}

func TestTheAgentPanelReadsANewAgentFromTheBodyAlone(t *testing.T) {
	rows := &fakeAgentRows{}
	rec, _ := serveAgentRows(t, rows, "/agents/conversation-variables", `{"messagingPrompt":"Oi {{apelido}}"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"variables":[]`) {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if rows.candidate.ID != "" || rows.candidate.WorkspaceID != "ws-1" || rows.candidate.MessagingPrompt != "Oi {{apelido}}" {
		t.Fatalf("candidate = %+v", rows.candidate)
	}
}

func TestTheAgentPanelRefusesOtherWorkspacesUnknownFieldsAndUnreadableNeeds(t *testing.T) {
	cases := []struct {
		target string
		body   string
		err    error
		status int
		code   string
	}{
		{"/agents/other/conversation-variables", `{}`, nil, http.StatusForbidden, "agent_forbidden"},
		{"/agents/gone/conversation-variables", `{}`, nil, http.StatusNotFound, "agent_not_found"},
		{"/agents/conversation-variables", `{"tools":[]}`, nil, http.StatusBadRequest, "invalid_body"},
		{"/agents/conversation-variables", `{}`, domain.ErrUnreadable, http.StatusServiceUnavailable, "automation_needs_unreadable"},
		{"/agents/conversation-variables", `{}`, errors.New("boom"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tc := range cases {
		rec, _ := serveAgentRows(t, &fakeAgentRows{err: tc.err}, tc.target, tc.body)
		var body struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != tc.status || body.Code != tc.code {
			t.Errorf("%s %s: status = %d body = %s", tc.target, tc.body, rec.Code, rec.Body.String())
		}
	}
}
