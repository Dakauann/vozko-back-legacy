package conversation_usecase

import (
	"testing"

	"github.com/stretchr/testify/require"

	"vozko/domain/conversation"
)

func searchWithScope(t *testing.T, scope conversation.DepartmentAccessScope, selected string) (*inboxServiceTestHistoryProvider, error) {
	t.Helper()
	history := &inboxServiceTestHistoryProvider{}
	service := NewInboxService(history, nil, nil, &inboxServiceTestResolver{workspaceID: "ws-1"}, nil,
		&inboxServiceTestAuthorizer{canAccessCampaign: true, departmentScope: scope, departmentScopeOkay: true, ownerOrAdmin: !scope.Restrict}, nil, nil, inboxSources())
	_, _, err := service.SearchInbox("user-1", conversation.SearchInboxInput{WorkspaceID: "ws-1", SelectedDepartmentID: selected})
	return history, err
}

func TestAnOwnerViewingADepartmentSeesOnlyThatDepartment(t *testing.T) {
	history, err := searchWithScope(t, conversation.DepartmentAccessScope{WorkspaceHasDepartments: true}, "vendas")

	require.NoError(t, err)
	require.Equal(t, []string{"vendas"}, history.lastSearch.DepartmentIDs)
	require.True(t, history.lastSearch.RestrictDepartments)
	require.Empty(t, history.lastSearch.AssigneeOverrideUserID, "a picked view is exact, it does not add the owner's own conversations from other departments")
}

func TestARestrictedMemberViewingOwnDepartmentKeepsTheirAssignedConversations(t *testing.T) {
	history, err := searchWithScope(t, conversation.DepartmentAccessScope{DepartmentIDs: []string{"vendas", "suporte"}, Restrict: true, WorkspaceHasDepartments: true}, "vendas")

	require.NoError(t, err)
	require.Equal(t, []string{"vendas"}, history.lastSearch.DepartmentIDs)
	require.Equal(t, "user-1", history.lastSearch.AssigneeOverrideUserID)
}

func TestARestrictedMemberCannotViewAForeignDepartment(t *testing.T) {
	_, err := searchWithScope(t, conversation.DepartmentAccessScope{DepartmentIDs: []string{"suporte"}, Restrict: true, WorkspaceHasDepartments: true}, "vendas")

	require.ErrorIs(t, err, conversation.ErrUnauthorized)
}
