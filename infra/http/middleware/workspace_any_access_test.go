package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/domain/auth"
	"vozko/domain/workspace"
)

type grantedAccess map[workspace.Resource]workspace.Action

func (g grantedAccess) Execute(_, _ string, resource workspace.Resource, action workspace.Action) error {
	if g[resource] == action {
		return nil
	}
	return errors.New("denied")
}

func anyAccessRequest(role string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/workspace-secrets", nil)
	req.Header.Set("X-Workspace-ID", "ws-1")
	return req.WithContext(context.WithValue(req.Context(), ClaimsContextKey, &auth.Claims{UserID: "u-1", Role: role}))
}

var secretGrants = []workspace.PermissionEntry{
	{Resource: workspace.ResourceAgents, Action: workspace.ActionUpdate},
	{Resource: workspace.ResourceWorkflows, Action: workspace.ActionUpdate},
}

func serveAnyAccess(t *testing.T, checker WorkspaceAccessChecker, role string) (int, string) {
	t.Helper()
	mw := NewWorkspaceMiddleware(checker, nil, nil)
	rec := httptest.NewRecorder()
	var seen string
	mw.CheckAnyAccess(secretGrants, func(w http.ResponseWriter, r *http.Request) {
		seen = GetWorkspaceID(r)
		w.WriteHeader(http.StatusNoContent)
	})(rec, anyAccessRequest(role))
	return rec.Code, seen
}

func TestAnyAccessLetsInWhoHoldsOnlyTheSecondGrant(t *testing.T) {
	code, ws := serveAnyAccess(t, grantedAccess{workspace.ResourceWorkflows: workspace.ActionUpdate}, "member")
	if code != http.StatusNoContent || ws != "ws-1" {
		t.Fatalf("code %d workspace %q", code, ws)
	}
}

func TestAnyAccessLetsInWhoHoldsOnlyTheFirstGrant(t *testing.T) {
	code, _ := serveAnyAccess(t, grantedAccess{workspace.ResourceAgents: workspace.ActionUpdate}, "member")
	if code != http.StatusNoContent {
		t.Fatalf("code %d", code)
	}
}

func TestAnyAccessRefusesWhoHoldsNoneOfTheGrants(t *testing.T) {
	code, _ := serveAnyAccess(t, grantedAccess{workspace.ResourceWorkflows: workspace.ActionRead}, "member")
	if code != http.StatusForbidden {
		t.Fatalf("code %d", code)
	}
}

func TestAnyAccessRefusesWhenTheCheckerIsMissing(t *testing.T) {
	code, _ := serveAnyAccess(t, nil, "member")
	if code != http.StatusForbidden {
		t.Fatalf("code %d", code)
	}
}
