package workspace_department_repository

import (
	"errors"
	"fmt"

	"vozko/domain/cache"
	"vozko/domain/workspace"
	workspace_department "vozko/domain/workspace/workspace_department"
)

func bumpScope(versions cache.Versions, workspaceID string) error {
	if workspaceID == "" {
		return nil
	}
	if err := versions.Bump(workspaceID); err != nil {
		return fmt.Errorf("saved, but the cached department scope could not be refreshed: %w", err)
	}
	return nil
}

type scopeVersionedRepository struct {
	workspace_department.Repository
	versions cache.Versions
}

func NewScopeVersionedRepository(inner workspace_department.Repository, versions cache.Versions) workspace_department.Repository {
	return &scopeVersionedRepository{Repository: inner, versions: versions}
}

func (r *scopeVersionedRepository) CreateDepartment(dept *workspace_department.Department) error {
	if err := r.Repository.CreateDepartment(dept); err != nil {
		return err
	}
	return bumpScope(r.versions, dept.WorkspaceID)
}

func (r *scopeVersionedRepository) UpdateDepartment(dept *workspace_department.Department) error {
	return r.changingDepartment(dept.ID, func() error { return r.Repository.UpdateDepartment(dept) })
}

func (r *scopeVersionedRepository) DeleteDepartment(id string) error {
	return r.changingDepartment(id, func() error { return r.Repository.DeleteDepartment(id) })
}

func (r *scopeVersionedRepository) AddMember(dm *workspace_department.DepartmentMember) error {
	return r.changingDepartment(dm.DepartmentID, func() error { return r.Repository.AddMember(dm) })
}

func (r *scopeVersionedRepository) RemoveMember(departmentID, memberID string) error {
	return r.changingDepartment(departmentID, func() error { return r.Repository.RemoveMember(departmentID, memberID) })
}

func (r *scopeVersionedRepository) changingDepartment(departmentID string, write func() error) error {
	department, err := r.Repository.GetDepartmentByID(departmentID)
	if errors.Is(err, workspace_department.ErrDepartmentNotFound) {
		return write()
	}
	if err != nil {
		return err
	}
	if err := write(); err != nil {
		return err
	}
	return bumpScope(r.versions, department.WorkspaceID)
}

type scopeVersionedWorkspaceRepository struct {
	workspace.Repository
	versions cache.Versions
}

func NewScopeVersionedWorkspaceRepository(inner workspace.Repository, versions cache.Versions) workspace.Repository {
	return &scopeVersionedWorkspaceRepository{Repository: inner, versions: versions}
}

func (r *scopeVersionedWorkspaceRepository) AddMember(member *workspace.Member) error {
	if err := r.Repository.AddMember(member); err != nil {
		return err
	}
	return bumpScope(r.versions, member.WorkspaceID)
}

func (r *scopeVersionedWorkspaceRepository) AcceptInvite(inviteID string, member *workspace.Member, permissions []*workspace.Permission, departmentIDs []string) error {
	if err := r.Repository.AcceptInvite(inviteID, member, permissions, departmentIDs); err != nil {
		return err
	}
	return bumpScope(r.versions, member.WorkspaceID)
}

func (r *scopeVersionedWorkspaceRepository) RemoveMember(memberID string) error {
	return r.changingMember(memberID, func() error { return r.Repository.RemoveMember(memberID) })
}

func (r *scopeVersionedWorkspaceRepository) UpdateMemberRole(memberID string, role workspace.Role) error {
	return r.changingMember(memberID, func() error { return r.Repository.UpdateMemberRole(memberID, role) })
}

func (r *scopeVersionedWorkspaceRepository) UpdateMemberRoleID(memberID string, roleID string) error {
	return r.changingMember(memberID, func() error { return r.Repository.UpdateMemberRoleID(memberID, roleID) })
}

func (r *scopeVersionedWorkspaceRepository) AddPermission(perm *workspace.Permission) error {
	if perm == nil {
		return r.Repository.AddPermission(perm)
	}
	return r.changingMember(perm.MemberID, func() error { return r.Repository.AddPermission(perm) })
}

func (r *scopeVersionedWorkspaceRepository) RemovePermission(memberID string, resource workspace.Resource, action workspace.Action) error {
	return r.changingMember(memberID, func() error { return r.Repository.RemovePermission(memberID, resource, action) })
}

func (r *scopeVersionedWorkspaceRepository) SetPermissions(memberID string, permissions []*workspace.Permission) error {
	return r.changingMember(memberID, func() error { return r.Repository.SetPermissions(memberID, permissions) })
}

func (r *scopeVersionedWorkspaceRepository) changingMember(memberID string, write func() error) error {
	member, err := r.Repository.GetMemberByID(memberID)
	if errors.Is(err, workspace.ErrMemberNotFound) {
		return write()
	}
	if err != nil {
		return err
	}
	if err := write(); err != nil {
		return err
	}
	if member == nil {
		return nil
	}
	return bumpScope(r.versions, member.WorkspaceID)
}
