package ws

import "vozko/domain/conversation"

type departmentScopeKey struct {
	userID      string
	workspaceID string
	isAdmin     bool
}

type memoizedDepartmentScope struct {
	scope   conversation.DepartmentAccessScope
	allowed bool
}

type departmentScopeMemo struct {
	authorizer conversation.ConversationAuthorizer
	known      map[departmentScopeKey]memoizedDepartmentScope
}

func (h *ConversationHub) newDepartmentScopeMemo() *departmentScopeMemo {
	return &departmentScopeMemo{authorizer: h.authorizer, known: make(map[departmentScopeKey]memoizedDepartmentScope)}
}

func (m *departmentScopeMemo) scope(userID, workspaceID string, isAdmin bool) (conversation.DepartmentAccessScope, bool) {
	key := departmentScopeKey{userID: userID, workspaceID: workspaceID, isAdmin: isAdmin}
	if known, ok := m.known[key]; ok {
		return known.scope, known.allowed
	}
	scope, allowed := m.authorizer.GetDepartmentScope(userID, workspaceID, isAdmin)
	m.known[key] = memoizedDepartmentScope{scope: scope, allowed: allowed}
	return scope, allowed
}
