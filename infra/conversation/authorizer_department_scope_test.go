package conversation

import (
	"errors"
	"reflect"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/workspace"
)

type scopedRoleMembership struct {
	role     workspace.Role
	readable bool
}

func (m scopedRoleMembership) GetMember(workspaceID, userID string) (*workspace.Member, error) {
	return &workspace.Member{ID: "member-" + userID, WorkspaceID: workspaceID, UserID: userID, Role: m.role}, nil
}

func (m scopedRoleMembership) HasPermission(_ string, resource workspace.Resource, action workspace.Action) (bool, error) {
	return m.readable && resource == workspace.ResourceConversations && action == workspace.ActionRead, nil
}

type scriptedDepartments struct {
	has       bool
	hasErr    error
	ids       []string
	idsErr    error
	hasChecks int
	idReads   int
}

func (d *scriptedDepartments) HasDepartments(string) (bool, error) {
	d.hasChecks++
	return d.has, d.hasErr
}

func (d *scriptedDepartments) GetMemberDepartmentIDs(string, string) ([]string, error) {
	d.idReads++
	return d.ids, d.idsErr
}

func scopeAuthorizer(members workspaceMembershipRepository, departments departmentMembershipRepository) *Authorizer {
	return NewAuthorizer(stubEntryAccess{}, &stubPlacements{}, members, departments,
		&stubAssignments{}, stubResolver{}, &stubShared{data: map[string]string{}}).(*Authorizer)
}

func TestGetDepartmentScopeKeepsEveryBranchAndErrorAnswer(t *testing.T) {
	boom := errors.New("connection reset")
	cases := []struct {
		name        string
		role        workspace.Role
		readable    bool
		departments *scriptedDepartments
		want        conversation.DepartmentAccessScope
		allowed     bool
	}{
		{"owner in a workspace with departments", workspace.RoleOwner, false, &scriptedDepartments{has: true},
			conversation.DepartmentAccessScope{WorkspaceHasDepartments: true}, true},
		{"admin in a workspace without departments", workspace.RoleAdmin, false, &scriptedDepartments{},
			conversation.DepartmentAccessScope{}, true},
		{"owner when the department check fails", workspace.RoleOwner, false, &scriptedDepartments{has: true, hasErr: boom},
			conversation.DepartmentAccessScope{}, true},
		{"employee without conversations read", workspace.RoleMember, false, &scriptedDepartments{has: true},
			conversation.DepartmentAccessScope{}, false},
		{"employee in a workspace without departments", workspace.RoleMember, true, &scriptedDepartments{ids: []string{"dept-a"}},
			conversation.DepartmentAccessScope{}, true},
		{"employee when the department check fails", workspace.RoleMember, true, &scriptedDepartments{hasErr: boom},
			conversation.DepartmentAccessScope{Restrict: true}, true},
		{"employee in no department of a departmented workspace", workspace.RoleMember, true, &scriptedDepartments{has: true},
			conversation.DepartmentAccessScope{Restrict: true, WorkspaceHasDepartments: true}, true},
		{"employee when their departments cannot be read", workspace.RoleMember, true, &scriptedDepartments{has: true, idsErr: boom},
			conversation.DepartmentAccessScope{Restrict: true, WorkspaceHasDepartments: true}, true},
		{"employee in departments", workspace.RoleMember, true, &scriptedDepartments{has: true, ids: []string{"dept-b", "", "dept-a", "dept-b"}},
			conversation.DepartmentAccessScope{DepartmentIDs: []string{"dept-b", "dept-a"}, Restrict: true, WorkspaceHasDepartments: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := scopeAuthorizer(scopedRoleMembership{role: tc.role, readable: tc.readable}, tc.departments)

			got, allowed := a.GetDepartmentScope(viewer, testWorkspace, false)

			if allowed != tc.allowed || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("scope = %+v allowed %v, want %+v allowed %v", got, allowed, tc.want, tc.allowed)
			}
		})
	}
}

func TestGetDepartmentScopeWithoutADepartmentRepository(t *testing.T) {
	owner, ownerAllowed := scopeAuthorizer(scopedRoleMembership{role: workspace.RoleOwner}, nil).GetDepartmentScope(viewer, testWorkspace, false)
	if !ownerAllowed || !reflect.DeepEqual(owner, conversation.DepartmentAccessScope{}) {
		t.Fatalf("owner = %+v %v", owner, ownerAllowed)
	}
	employee, employeeAllowed := scopeAuthorizer(scopedRoleMembership{role: workspace.RoleMember, readable: true}, nil).GetDepartmentScope(viewer, testWorkspace, false)
	if !employeeAllowed || !reflect.DeepEqual(employee, conversation.DepartmentAccessScope{Restrict: true}) {
		t.Fatalf("employee = %+v %v", employee, employeeAllowed)
	}
}

func TestGetDepartmentScopeOnlyAsksWhetherDepartmentsExist(t *testing.T) {
	departments := &scriptedDepartments{}
	a := scopeAuthorizer(scopedRoleMembership{role: workspace.RoleMember, readable: true}, departments)

	a.GetDepartmentScope(viewer, testWorkspace, false)

	if departments.hasChecks != 1 || departments.idReads != 0 {
		t.Fatalf("checks = %d id reads = %d, want one existence check and no member read when the workspace has none", departments.hasChecks, departments.idReads)
	}
}

func TestResolvedEntryAccessReadsEachUsersScopeOncePerBroadcast(t *testing.T) {
	departments := &scriptedDepartments{has: true, ids: []string{"dept-2"}}
	members := &countingMembership{stubMembership: stubMembership{permissions: map[workspace.Action]bool{workspace.ActionViewOthers: true}}}
	a := NewAuthorizer(stubEntryAccess{}, &stubPlacements{}, members, departments,
		&stubAssignments{assignment: assignedTo(owner)}, stubResolver{}, &stubShared{data: map[string]string{}}).(*Authorizer)

	allow := a.ResolveEntryAccess(testWorkspace, testEntry, testEntryType)
	for _, user := range []string{viewer, viewer, viewer, "user-other", "user-other"} {
		want := a.CanAccessEntry(user, testWorkspace, testEntry, testEntryType, false)
		got := allow(user, false)
		if got != want {
			t.Fatalf("%s: resolved = %v, CanAccessEntry = %v", user, got, want)
		}
	}

	departments.hasChecks = 0
	fresh := a.ResolveEntryAccess(testWorkspace, testEntry, testEntryType)
	for _, user := range []string{viewer, viewer, viewer, "user-other", "user-other"} {
		fresh(user, false)
	}
	if departments.hasChecks != 2 {
		t.Fatalf("department scope read %d times for two users across five checks, want 2", departments.hasChecks)
	}
}
