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
	domain "vozko/domain/variabledecl"
	workspace_domain "vozko/domain/workspace"
	"vozko/infra/http/middleware"
)

type fakeDeclarations struct {
	rows     []domain.Declaration
	saved    []domain.Declaration
	savedBy  string
	err      error
	registry *domain.Registry
}

func (f *fakeDeclarations) List(context.Context, string) ([]domain.Declaration, error) {
	return f.rows, f.err
}

func (f *fakeDeclarations) Save(_ context.Context, _ string, by string, d domain.Declaration) (domain.Declaration, error) {
	if f.err != nil {
		return domain.Declaration{}, f.err
	}
	checked, err := f.registry.CheckDeclaration(d)
	if err != nil {
		return domain.Declaration{}, err
	}
	f.saved = append(f.saved, checked)
	f.savedBy = by
	return checked, nil
}

func (f *fakeDeclarations) Delete(context.Context, string, string) error { return f.err }

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

func (g *gate) ac(r workspace_domain.Resource, a workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
	g.asked = append(g.asked, string(r)+":"+string(a))
	return next
}

func serve(t *testing.T, f *fakeDeclarations, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := mux.NewRouter()
	g := &gate{}
	RegisterRoutes(router, NewHandler(f), g.ac)
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

func TestRoutesAskForTheWorkflowPermissions(t *testing.T) {
	g := &gate{}
	RegisterRoutes(mux.NewRouter(), NewHandler(newFake(t)), g.ac)
	want := []string{"workflows:read", "workflows:update", "workflows:update"}
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
