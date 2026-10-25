package middleware

import (
	"context"
	"net/http"
	"strings"

	"vozko/delivery/http/response"
	"vozko/domain/workspace"
)

func (m *WorkspaceMiddleware) CheckAnyAccess(grants []workspace.PermissionEntry, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r)
		if claims == nil {
			response.WriteError(w, http.StatusUnauthorized, "Unauthorized", nil)
			return
		}
		wsID := accessWorkspaceID(r)
		if claims.Role != "admin" {
			if wsID == "" {
				writeWorkspaceRequired(w)
				return
			}
			if !m.holdsAny(claims.UserID, wsID, grants) {
				required := grantKeys(grants)
				response.WriteError(w, http.StatusForbidden,
					"Insufficient permissions: requires one of "+required+" in this workspace",
					map[string]string{"required_permission": required})
				return
			}
		}
		if wsID != "" {
			r = r.WithContext(context.WithValue(r.Context(), WorkspaceIDContextKey, wsID))
		}
		handler(w, r)
	}
}

func (m *WorkspaceMiddleware) holdsAny(userID, workspaceID string, grants []workspace.PermissionEntry) bool {
	if m.accessChecker == nil {
		return false
	}
	for _, g := range grants {
		if m.accessChecker.Execute(userID, workspaceID, g.Resource, g.Action) == nil {
			return true
		}
	}
	return false
}

func grantKeys(grants []workspace.PermissionEntry) string {
	keys := make([]string, len(grants))
	for i, g := range grants {
		keys[i] = string(g.Resource) + ":" + string(g.Action)
	}
	return strings.Join(keys, " | ")
}
