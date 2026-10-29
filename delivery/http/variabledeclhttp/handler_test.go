package variabledeclhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"vozko/domain/auth"
	"vozko/domain/automationneeds"
	"vozko/domain/conversation"
	domain "vozko/domain/variabledecl"
	workspace_domain "vozko/domain/workspace"
	"vozko/infra/http/middleware"
	variabledecl_usecase "vozko/usecases/variabledecl"
)

type fakeDeclarations struct {
	rows     []domain.Declaration
	locks    map[string][]automationneeds.DeclarationUser
	saved    []domain.Declaration
	savedBy  string
	editor   variabledecl_usecase.Editor
	err      error
	registry *domain.Registry
}

func (f *fakeDeclarations) Editable(_ context.Context, _ string, editor variabledecl_usecase.Editor) ([]variabledecl_usecase.EditableDeclaration, error) {
	f.editor = editor
	out := make([]variabledecl_usecase.EditableDeclaration, 0, len(f.rows))
	for _, d := range f.rows {
		out = append(out, variabledecl_usecase.EditableDeclaration{Declaration: d, LockedBy: f.locks[d.Name]})
	}
	return out, f.err
}

func (f *fakeDeclarations) Save(_ context.Context, _ string, editor variabledecl_usecase.Editor, d domain.Declaration) (domain.Declaration, error) {
	if f.err != nil {
		return domain.Declaration{}, f.err
	}
	checked, err := f.registry.CheckDeclaration(d)
	if err != nil {
		return domain.Declaration{}, err
	}
	f.saved = append(f.saved, checked)
	f.savedBy = editor.Requester.UserID
	return checked, nil
}

func (f *fakeDeclarations) Delete(_ context.Context, _ string, editor variabledecl_usecase.Editor, _ string) error {
	f.editor = editor
	return f.err
}

func (f *fakeDeclarations) Registry() *domain.Registry { return f.registry }

func newFake(t *testing.T) *fakeDeclarations {
	t.Helper()
	registry, err := domain.NewRegistry(domain.Builtin()...)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeDeclarations{registry: registry}
}

type gate struct{ asked []string }

func (g *gate) anyOf(grants []workspace_domain.PermissionEntry, next http.HandlerFunc) http.HandlerFunc {
	names := make([]string, len(grants))
	for i, grant := range grants {
		names[i] = string(grant.Resource) + ":" + string(grant.Action)
	}
	g.asked = append(g.asked, strings.Join(names, "|"))
	return next
}

func serve(t *testing.T, f *fakeDeclarations, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := mux.NewRouter()
	g := &gate{}
	RegisterRoutes(router, NewHandler(f), g.anyOf)
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, "ws-1")
	ctx = context.WithValue(ctx, middleware.ClaimsContextKey, &auth.Claims{UserID: "u-1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, into any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		t.Fatalf("body %s: %v", rec.Body.String(), err)
	}
}

func codeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Code
}

func TestRoutesLetWorkflowAndAgentEditorsShareTheCatalog(t *testing.T) {
	g := &gate{}
	RegisterRoutes(mux.NewRouter(), NewHandler(newFake(t)), g.anyOf)
	want := []string{"workflows:read|agents:read", "workflows:update|agents:update", "workflows:update|agents:update"}
	if strings.Join(g.asked, ",") != strings.Join(want, ",") {
		t.Fatalf("asked = %v, want %v", g.asked, want)
	}
}

func TestListAnswersDeclarationsSystemKeysAndTypes(t *testing.T) {
	f := newFake(t)
	f.rows = []domain.Declaration{{Name: "turno", Label: "Turno", Type: domain.Type{Kind: domain.KindChoice, Options: []string{"Manhã"}}, UpdatedAt: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)}}
	rec := serve(t, f, http.MethodGet, "/conversation-variables", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var got ListResponse
	decode(t, rec, &got)
	if len(got.Declarations) != 1 || got.Declarations[0].Ref != "campvars.turno" || got.Declarations[0].Type.Shape != "choice" || got.Declarations[0].Type.Options[0] != "Manhã" {
		t.Fatalf("declarations = %+v", got.Declarations)
	}
	if len(got.System) != 3 || got.System[0].Name != "lead_name" || got.System[0].Label != "Nome do contato" || got.System[0].LabelKey != "conversation_lead_name" {
		t.Fatalf("system = %+v", got.System)
	}
	if len(got.Types) != 4 || got.Types[0].Kind != "text" || got.Types[0].Shape != "text" {
		t.Fatalf("types = %+v", got.Types)
	}
}

func TestSaveTakesTheNameFromThePathAndStampsTheUser(t *testing.T) {
	f := newFake(t)
	rec := serve(t, f, http.MethodPut, "/conversation-variables/valor", `{"label":"Valor","description":"Mensalidade","type":{"kind":"number"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if len(f.saved) != 1 || f.saved[0].Name != "valor" || f.savedBy != "u-1" {
		t.Fatalf("saved = %+v by %q", f.saved, f.savedBy)
	}
}

func TestSaveRefusalsAreCoded(t *testing.T) {
	cases := []struct {
		target, body, code string
		status             int
	}{
		{"/conversation-variables/minha%20var", `{"label":"X","type":{"kind":"text"}}`, "variable_name_invalid", http.StatusBadRequest},
		{"/conversation-variables/lead_name", `{"label":"X","type":{"kind":"text"}}`, "variable_name_reserved", http.StatusBadRequest},
		{"/conversation-variables/a", `{"label":"","type":{"kind":"text"}}`, "variable_label_invalid", http.StatusBadRequest},
		{"/conversation-variables/a", `{"label":"A","description":"` + strings.Repeat("d", 501) + `","type":{"kind":"text"}}`, "variable_description_too_long", http.StatusBadRequest},
		{"/conversation-variables/a", `{"label":"A","type":{"kind":"planet"}}`, "variable_type_unknown", http.StatusBadRequest},
		{"/conversation-variables/a", `{"label":"A","type":{"kind":"choice"}}`, "variable_type_invalid", http.StatusBadRequest},
	}
	for _, tc := range cases {
		rec := serve(t, newFake(t), http.MethodPut, tc.target, tc.body)
		if rec.Code != tc.status || codeOf(t, rec) != tc.code {
			t.Errorf("%s: status %d code %q, want %d %q", tc.body, rec.Code, codeOf(t, rec), tc.status, tc.code)
		}
	}
}

func TestUnreadableAndMissingAreCoded(t *testing.T) {
	f := newFake(t)
	f.err = domain.ErrUnreadable
	if rec := serve(t, f, http.MethodGet, "/conversation-variables", ""); rec.Code != http.StatusServiceUnavailable || codeOf(t, rec) != "variable_declarations_unreadable" {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	f.err = errors.Join(domain.ErrNotFound)
	if rec := serve(t, f, http.MethodDelete, "/conversation-variables/x", ""); rec.Code != http.StatusNotFound || codeOf(t, rec) != "variable_not_found" {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	f.err = nil
	if rec := serve(t, f, http.MethodDelete, "/conversation-variables/x", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestListSaysWhichDeclarationsThisPersonCanOnlyRead(t *testing.T) {
	f := newFake(t)
	f.rows = []domain.Declaration{{Name: "livre", Label: "Livre", Type: domain.Type{Kind: domain.KindText}}, {Name: "turno", Label: "Turno", Type: domain.Type{Kind: domain.KindText}}}
	f.locks = map[string][]automationneeds.DeclarationUser{"turno": {{Link: automationneeds.Link{Automation: conversation.Automation{Kind: conversation.AutomationAgent, ID: "a1"}, Name: "Cobrador"}}}}
	rec := serve(t, f, http.MethodGet, "/conversation-variables", "")
	var got ListResponse
	decode(t, rec, &got)
	livre, turno := got.Declarations[0], got.Declarations[1]
	if !livre.Editable || len(livre.LockedBy) != 0 || livre.LockedBy == nil {
		t.Fatalf("livre = %+v", livre)
	}
	if turno.Editable || len(turno.LockedBy) != 1 || turno.LockedBy[0].Kind != "agent" || turno.LockedBy[0].Name != "Cobrador" {
		t.Fatalf("turno = %+v", turno)
	}
	if f.editor.Requester.UserID != "u-1" {
		t.Fatalf("editor = %+v", f.editor)
	}
}

func TestAChangeTheGrantsDoNotCoverNamesWhatUsesTheDeclaration(t *testing.T) {
	f := newFake(t)
	f.err = &variabledecl_usecase.ChangeForbidden{Name: "turno", Users: []automationneeds.DeclarationUser{{Link: automationneeds.Link{Automation: conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "w1"}, Name: "Matrículas"}}}}
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		rec := serve(t, f, method, "/conversation-variables/turno", `{"label":"Turno","type":{"kind":"text"}}`)
		var body struct {
			Code    string `json:"code"`
			Details struct {
				Name  string `json:"name"`
				Users []struct {
					Kind string `json:"kind"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"users"`
			} `json:"details"`
		}
		decode(t, rec, &body)
		if rec.Code != http.StatusForbidden || body.Code != "variable_change_forbidden" || body.Details.Name != "turno" || len(body.Details.Users) != 1 || body.Details.Users[0].Name != "Matrículas" || body.Details.Users[0].Kind != "workflow" {
			t.Fatalf("%s: status %d body %s", method, rec.Code, rec.Body.String())
		}
	}
}
