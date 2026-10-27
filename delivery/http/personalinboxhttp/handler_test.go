package personalinboxhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	"vozko/domain/auth"
	"vozko/domain/conversation"
	"vozko/domain/personalinbox"
	"vozko/domain/shared"
	"vozko/infra/http/middleware"
)

type fakePreferences struct {
	stored    personalinbox.Preferences
	saveErr   error
	savedBy   shared.Person
	savedIn   string
	readIn    string
	readUser  string
	saveCalls int
}

func (f *fakePreferences) InboxPreferences(_ context.Context, workspaceID, userID string) personalinbox.Preferences {
	f.readIn, f.readUser = workspaceID, userID
	return f.stored
}

func (f *fakePreferences) SaveInboxPreferences(_ context.Context, by shared.Person, workspaceID string, prefs personalinbox.Preferences) (personalinbox.Preferences, error) {
	f.saveCalls++
	f.savedBy, f.savedIn = by, workspaceID
	if f.saveErr != nil {
		return personalinbox.Preferences{}, f.saveErr
	}
	f.stored = prefs
	return prefs, nil
}

func call(t *testing.T, h *Handler, method, body string, withClaims bool) *httptest.ResponseRecorder {
	t.Helper()
	router := mux.NewRouter()
	RegisterProtectedRoutes(router, h)
	req := httptest.NewRequest(method, "/inbox/preferences", strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, "ws-1")
	if withClaims {
		ctx = context.WithValue(ctx, middleware.ClaimsContextKey, &auth.Claims{UserID: "user-1", Role: "user"})
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

func newHandler(t *testing.T, prefs *fakePreferences) *Handler {
	t.Helper()
	h, err := NewHandler(prefs)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body response.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return body.Code
}

func TestTheHandlerRefusesToStartWithoutItsService(t *testing.T) {
	if _, err := NewHandler(nil); err == nil {
		t.Fatal("a handler without preferences must refuse to start")
	}
}

func TestReadingThePreferencesUsesTheSessionAndTheWorkspaceHeader(t *testing.T) {
	prefs := &fakePreferences{stored: personalinbox.Preferences{HideGroups: true}}
	rec := call(t, newHandler(t, prefs), http.MethodGet, "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body InboxPreferencesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !body.HideGroups {
		t.Fatalf("body = %s, %v", rec.Body.String(), err)
	}
	if prefs.readIn != "ws-1" || prefs.readUser != "user-1" {
		t.Fatalf("read %q/%q", prefs.readIn, prefs.readUser)
	}
}

func TestSavingThePreferencesStoresTheChoiceForTheCaller(t *testing.T) {
	prefs := &fakePreferences{}
	rec := call(t, newHandler(t, prefs), http.MethodPut, `{"hideGroups":true}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if !prefs.stored.HideGroups || prefs.savedBy.UserID != "user-1" || prefs.savedIn != "ws-1" {
		t.Fatalf("saved %+v by %+v in %q", prefs.stored, prefs.savedBy, prefs.savedIn)
	}
}

func TestEveryRefusalCarriesItsCode(t *testing.T) {
	cases := []struct {
		name   string
		method string
		body   string
		claims bool
		err    error
		status int
		code   string
	}{
		{"no session", http.MethodGet, "", false, nil, http.StatusUnauthorized, "unauthorized"},
		{"missing choice", http.MethodPut, `{}`, true, nil, http.StatusBadRequest, codeInvalid},
		{"no inbox access", http.MethodPut, `{"hideGroups":false}`, true, conversation.ErrUnauthorized, http.StatusForbidden, codeForbidden},
		{"no workspace", http.MethodPut, `{"hideGroups":false}`, true, personalinbox.ErrWorkspaceRequired, http.StatusForbidden, codeForbidden},
		{"storage down", http.MethodPut, `{"hideGroups":false}`, true, errors.New("db down"), http.StatusInternalServerError, codeFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prefs := &fakePreferences{saveErr: c.err}
			rec := call(t, newHandler(t, prefs), c.method, c.body, c.claims)
			if rec.Code != c.status || errorCode(t, rec) != c.code {
				t.Fatalf("got %d %s, want %d %s", rec.Code, rec.Body.String(), c.status, c.code)
			}
		})
	}
}

func TestTheRefusalCodesAreTheOnesTheFrontTranslates(t *testing.T) {
	want := []string{"unauthorized", "inbox_preferences_invalid", "inbox_preferences_forbidden", "inbox_preferences_failed"}
	got := RefusalCodes()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("refusal codes = %v, want %v", got, want)
	}
}
