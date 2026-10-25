package workspace_department_repository

import (
	"errors"
	"strconv"
	"sync"
	"time"

	"vozko/domain/cache"
	"vozko/domain/workspace"
	workspace_department "vozko/domain/workspace/workspace_department"
)

type memoryState struct {
	cache.SharedState
	mu     sync.Mutex
	values map[string]string
	ttls   map[string]time.Duration
	incrs  int
	incErr error
}

func newMemoryState() *memoryState {
	return &memoryState{values: map[string]string{}, ttls: map[string]time.Duration{}}
}

func (s *memoryState) GetString(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key], nil
}

func (s *memoryState) SetString(key, value string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	s.ttls[key] = ttl
	return nil
}

func (s *memoryState) Incr(key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.incErr != nil {
		return 0, s.incErr
	}
	n, _ := strconv.ParseInt(s.values[key], 10, 64)
	n++
	s.values[key] = strconv.FormatInt(n, 10)
	s.incrs++
	return n, nil
}

func (s *memoryState) ttlsSet() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]time.Duration, 0, len(s.ttls))
	for _, ttl := range s.ttls {
		out = append(out, ttl)
	}
	return out
}

type unreadableVersions struct{}

func (unreadableVersions) Version(string) (string, error) { return "", errors.New("redis down") }
func (unreadableVersions) Bump(string) error              { return errors.New("redis down") }

type fakeDepartments struct {
	workspace_department.Repository
	mu          sync.Mutex
	has         map[string]bool
	hasErr      error
	memberIDs   map[string][]string
	memberErr   error
	departments map[string]*workspace_department.Department
	lookupErr   error
	writeErr    error
	hasCalls    int
	memberCalls int
	writes      []string
}

func newFakeDepartments() *fakeDepartments {
	return &fakeDepartments{
		has:         map[string]bool{},
		memberIDs:   map[string][]string{},
		departments: map[string]*workspace_department.Department{},
	}
}

func (f *fakeDepartments) HasDepartments(workspaceID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hasCalls++
	if f.hasErr != nil {
		return false, f.hasErr
	}
	return f.has[workspaceID], nil
}

func (f *fakeDepartments) GetMemberDepartmentIDs(workspaceID, userID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.memberCalls++
	if f.memberErr != nil {
		return nil, f.memberErr
	}
	return f.memberIDs[workspaceID+"|"+userID], nil
}

func (f *fakeDepartments) GetDepartmentByID(id string) (*workspace_department.Department, error) {
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	department, ok := f.departments[id]
	if !ok {
		return nil, workspace_department.ErrDepartmentNotFound
	}
	copied := *department
	return &copied, nil
}

func (f *fakeDepartments) wrote(name string) error {
	f.writes = append(f.writes, name)
	return f.writeErr
}

func (f *fakeDepartments) CreateDepartment(*workspace_department.Department) error {
	return f.wrote("create")
}

func (f *fakeDepartments) UpdateDepartment(*workspace_department.Department) error {
	return f.wrote("update")
}

func (f *fakeDepartments) DeleteDepartment(string) error {
	return f.wrote("delete")
}

func (f *fakeDepartments) AddMember(*workspace_department.DepartmentMember) error {
	return f.wrote("add member")
}

func (f *fakeDepartments) RemoveMember(string, string) error {
	return f.wrote("remove member")
}

type fakeWorkspaces struct {
	workspace.Repository
	members   map[string]*workspace.Member
	lookupErr error
	writeErr  error
	writes    []string
}

func (f *fakeWorkspaces) GetMemberByID(memberID string) (*workspace.Member, error) {
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	member, ok := f.members[memberID]
	if !ok {
		return nil, workspace.ErrMemberNotFound
	}
	return member, nil
}

func (f *fakeWorkspaces) wrote(name string) error {
	f.writes = append(f.writes, name)
	return f.writeErr
}

func (f *fakeWorkspaces) AddMember(*workspace.Member) error { return f.wrote("add member") }
func (f *fakeWorkspaces) RemoveMember(string) error         { return f.wrote("remove member") }
func (f *fakeWorkspaces) UpdateMemberRole(string, workspace.Role) error {
	return f.wrote("role")
}
func (f *fakeWorkspaces) UpdateMemberRoleID(string, string) error { return f.wrote("role id") }
func (f *fakeWorkspaces) AddPermission(*workspace.Permission) error {
	return f.wrote("add permission")
}
func (f *fakeWorkspaces) RemovePermission(string, workspace.Resource, workspace.Action) error {
	return f.wrote("remove permission")
}
func (f *fakeWorkspaces) SetPermissions(string, []*workspace.Permission) error {
	return f.wrote("set permissions")
}
func (f *fakeWorkspaces) AcceptInvite(string, *workspace.Member, []*workspace.Permission, []string) error {
	return f.wrote("accept invite")
}
