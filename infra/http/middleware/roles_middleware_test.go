package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/domain/auth"
	"vozko/domain/user"
)

func roleRefusalOf(t *testing.T, guard func(http.Handler) http.Handler, role string) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	reached := false
	handler := guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	req := httptest.NewRequest(http.MethodPost, "/admin/plans/p1/duplicate", nil)
	req = req.WithContext(context.WithValue(req.Context(), ClaimsContextKey, &auth.Claims{UserID: "u1", Role: role}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec, reached
}

func TestARoleRefusalCarriesTheStableForbiddenCode(t *testing.T) {
	roles := NewRolesMiddleware()
	guards := map[string]func(http.Handler) http.Handler{
		"RequireRole":    roles.RequireRole(user.RoleAdmin),
		"RequireAnyRole": roles.RequireAnyRole(user.RoleAdmin),
	}
	for name, guard := range guards {
		rec, reached := roleRefusalOf(t, guard, "user")
		var body struct {
			Code     string            `json:"code"`
			Expected map[string]string `json:"expected"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: decode %q: %v", name, rec.Body.String(), err)
		}
		if reached || rec.Code != http.StatusForbidden || body.Code != RefusalRoleRequired.Code || body.Expected["role"] == "" {
			t.Fatalf("%s: reached %v, status %d, body %s", name, reached, rec.Code, rec.Body.String())
		}
	}
}

func TestAnAdminPassesTheRoleGuard(t *testing.T) {
	if rec, reached := roleRefusalOf(t, NewRolesMiddleware().RequireRole(user.RoleAdmin), string(user.RoleAdmin)); !reached || rec.Code != http.StatusOK {
		t.Fatalf("admin was refused: %d %s", rec.Code, rec.Body.String())
	}
}
