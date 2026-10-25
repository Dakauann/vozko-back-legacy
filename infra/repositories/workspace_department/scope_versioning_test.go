package workspace_department_repository

import (
	"errors"
	"testing"

	"vozko/domain/workspace"
	workspace_department "vozko/domain/workspace/workspace_department"
)

func versionOf(t *testing.T, state *memoryState, workspaceID string) string {
	t.Helper()
	version, err := NewScopeVersions(state).Version(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	return version
}

func departmentWrites() map[string]func(workspace_department.Repository) error {
	return map[string]func(workspace_department.Repository) error{
		"create": func(r workspace_department.Repository) error {
			return r.CreateDepartment(&workspace_department.Department{ID: "dept-new", WorkspaceID: "ws-1"})
		},
		"update": func(r workspace_department.Repository) error {
			return r.UpdateDepartment(&workspace_department.Department{ID: "dept-a", Name: "Suporte"})
		},
		"delete": func(r workspace_department.Repository) error {
			return r.DeleteDepartment("dept-a")
		},
		"add member": func(r workspace_department.Repository) error {
			return r.AddMember(&workspace_department.DepartmentMember{DepartmentID: "dept-a", MemberID: "member-ana"})
		},
		"remove member": func(r workspace_department.Repository) error {
			return r.RemoveMember("dept-a", "member-ana")
		},
	}
}

func versionedDepartments() (*memoryState, *fakeDepartments, workspace_department.Repository) {
	state := newMemoryState()
	inner := newFakeDepartments()
	inner.departments["dept-a"] = &workspace_department.Department{ID: "dept-a", WorkspaceID: "ws-1"}
	return state, inner, NewScopeVersionedRepository(inner, NewScopeVersions(state))
}

func TestEveryDepartmentWriteBumpsOnlyItsWorkspace(t *testing.T) {
	for name, write := range departmentWrites() {
		t.Run(name, func(t *testing.T) {
			state, inner, repo := versionedDepartments()

			if err := write(repo); err != nil {
				t.Fatal(err)
			}

			if len(inner.writes) != 1 || inner.writes[0] != name {
				t.Fatalf("writes = %v, want the one %s", inner.writes, name)
			}
			if got := versionOf(t, state, "ws-1"); got != "1" {
				t.Fatalf("ws-1 version = %s, want 1", got)
			}
			if got := versionOf(t, state, "ws-2"); got != "0" {
				t.Fatalf("ws-2 version = %s, want untouched 0", got)
			}
		})
	}
}

func TestAFailedDepartmentWriteBumpsNothing(t *testing.T) {
	for name, write := range departmentWrites() {
		t.Run(name, func(t *testing.T) {
			state, inner, repo := versionedDepartments()
			boom := errors.New("constraint")
			inner.writeErr = boom

			if err := write(repo); !errors.Is(err, boom) {
				t.Fatalf("error = %v, want the write error", err)
			}
			if state.incrs != 0 {
				t.Fatalf("a failed write bumped %d times", state.incrs)
			}
		})
	}
}

func TestAWriteToAMissingDepartmentPassesThroughWithoutABump(t *testing.T) {
	state, inner, repo := versionedDepartments()
	delete(inner.departments, "dept-a")
	inner.writeErr = workspace_department.ErrDepartmentNotFound

	if err := repo.DeleteDepartment("dept-a"); !errors.Is(err, workspace_department.ErrDepartmentNotFound) {
		t.Fatalf("error = %v, want the repository's own not found", err)
	}
	if len(inner.writes) != 1 {
		t.Fatalf("the write must still run exactly as before, got %v", inner.writes)
	}
	if state.incrs != 0 {
		t.Fatal("a department that does not exist changes no scope")
	}
}

func TestAnUnreadableDepartmentStopsTheWrite(t *testing.T) {
	_, inner, repo := versionedDepartments()
	boom := errors.New("connection reset")
	inner.lookupErr = boom

	if err := repo.RemoveMember("dept-a", "member-ana"); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the lookup error", err)
	}
	if len(inner.writes) != 0 {
		t.Fatalf("nothing may be written when the workspace to refresh is unknown, got %v", inner.writes)
	}
}

func TestADepartmentWriteReportsAFailedBump(t *testing.T) {
	state, inner, repo := versionedDepartments()
	boom := errors.New("redis down")
	state.incErr = boom

	err := repo.AddMember(&workspace_department.DepartmentMember{DepartmentID: "dept-a", MemberID: "member-ana"})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the bump error surfaced", err)
	}
	if len(inner.writes) != 1 {
		t.Fatalf("the write itself happened, got %v", inner.writes)
	}
}

func memberWrites() map[string]func(workspace.Repository) error {
	return map[string]func(workspace.Repository) error{
		"add member": func(r workspace.Repository) error {
			return r.AddMember(&workspace.Member{ID: "member-new", WorkspaceID: "ws-1", UserID: "new"})
		},
		"accept invite": func(r workspace.Repository) error {
			return r.AcceptInvite("invite-1", &workspace.Member{ID: "member-new", WorkspaceID: "ws-1", UserID: "new"}, nil, []string{"dept-a"})
		},
		"remove member": func(r workspace.Repository) error {
			return r.RemoveMember("member-ana")
		},
		"role": func(r workspace.Repository) error {
			return r.UpdateMemberRole("member-ana", workspace.RoleAdmin)
		},
		"role id": func(r workspace.Repository) error {
			return r.UpdateMemberRoleID("member-ana", "role-1")
		},
		"set permissions": func(r workspace.Repository) error {
			return r.SetPermissions("member-ana", nil)
		},
		"add permission": func(r workspace.Repository) error {
			return r.AddPermission(&workspace.Permission{MemberID: "member-ana"})
		},
		"remove permission": func(r workspace.Repository) error {
			return r.RemovePermission("member-ana", workspace.ResourceConversations, workspace.ActionRead)
		},
	}
}

func versionedWorkspaces() (*memoryState, *fakeWorkspaces, workspace.Repository) {
	state := newMemoryState()
	inner := &fakeWorkspaces{members: map[string]*workspace.Member{
		"member-ana": {ID: "member-ana", WorkspaceID: "ws-1", UserID: "ana"},
	}}
	return state, inner, NewScopeVersionedWorkspaceRepository(inner, NewScopeVersions(state))
}

func TestEveryMemberEventBumpsTheMembersWorkspace(t *testing.T) {
	for name, write := range memberWrites() {
		t.Run(name, func(t *testing.T) {
			state, inner, repo := versionedWorkspaces()

			if err := write(repo); err != nil {
				t.Fatal(err)
			}

			if len(inner.writes) != 1 || inner.writes[0] != name {
				t.Fatalf("writes = %v, want the one %s", inner.writes, name)
			}
			if got := versionOf(t, state, "ws-1"); got != "1" {
				t.Fatalf("ws-1 version = %s, want 1", got)
			}
		})
	}
}

func TestAFailedMemberEventBumpsNothing(t *testing.T) {
	for name, write := range memberWrites() {
		t.Run(name, func(t *testing.T) {
			state, inner, repo := versionedWorkspaces()
			boom := errors.New("constraint")
			inner.writeErr = boom

			if err := write(repo); !errors.Is(err, boom) {
				t.Fatalf("error = %v, want the write error", err)
			}
			if state.incrs != 0 {
				t.Fatalf("a failed write bumped %d times", state.incrs)
			}
		})
	}
}

func TestAMemberEventForAnUnknownMemberPassesThrough(t *testing.T) {
	state, inner, repo := versionedWorkspaces()
	inner.writeErr = workspace.ErrMemberNotFound

	if err := repo.RemoveMember("member-ghost"); !errors.Is(err, workspace.ErrMemberNotFound) {
		t.Fatalf("error = %v, want the repository's own answer", err)
	}
	if len(inner.writes) != 1 || state.incrs != 0 {
		t.Fatalf("writes = %v bumps = %d, want the write and no bump", inner.writes, state.incrs)
	}
}

func TestAnUnreadableMemberStopsTheMemberEvent(t *testing.T) {
	_, inner, repo := versionedWorkspaces()
	boom := errors.New("connection reset")
	inner.lookupErr = boom

	if err := repo.UpdateMemberRole("member-ana", workspace.RoleAdmin); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the lookup error", err)
	}
	if len(inner.writes) != 0 {
		t.Fatalf("nothing may be written when the workspace to refresh is unknown, got %v", inner.writes)
	}
}

func TestAMemberEventReportsAFailedBump(t *testing.T) {
	state, inner, repo := versionedWorkspaces()
	boom := errors.New("redis down")
	state.incErr = boom

	if err := repo.RemoveMember("member-ana"); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the bump error surfaced", err)
	}
	if len(inner.writes) != 1 {
		t.Fatalf("the write itself happened, got %v", inner.writes)
	}
}
