package variabledecl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"vozko/domain/agent"
	"vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/shared/deps"
	domain "vozko/domain/variabledecl"
	"vozko/domain/workflow"
	"vozko/domain/workspace"
	workspace_department "vozko/domain/workspace/workspace_department"
)

type Automations interface {
	Automations(ctx context.Context, workspaceID string) ([]*agent.Agent, []*workflow.Workflow, error)
}

type ServiceDeps struct {
	Repo        domain.Repository
	Registry    *domain.Registry
	Automations Automations
	Permissions workspace.PermissionChecker
}

type Service struct {
	repo        domain.Repository
	registry    *domain.Registry
	automations Automations
	permissions workspace.PermissionChecker
	now         func() time.Time
}

type Editor struct {
	Requester   workspace.Requester
	Departments *workspace_department.DepartmentFilter
}

type EditableDeclaration struct {
	domain.Declaration
	LockedBy []automationneeds.DeclarationUser
}

type ChangeForbidden struct {
	Name  string
	Users []automationneeds.DeclarationUser
}

func (e *ChangeForbidden) Error() string {
	names := make([]string, 0, len(e.Users))
	for _, user := range e.Users {
		names = append(names, user.Name)
	}
	return fmt.Sprintf("%v: %s is used by %s", domain.ErrChangeForbidden, e.Name, strings.Join(names, ", "))
}

func (e *ChangeForbidden) Is(target error) bool { return target == domain.ErrChangeForbidden }

func NewService(d ServiceDeps) (*Service, error) {
	if err := deps.Require("variable declarations", map[string]any{
		"repository": d.Repo, "type registry": d.Registry, "automations": d.Automations, "permissions": d.Permissions,
	}); err != nil {
		return nil, err
	}
	return &Service{repo: d.Repo, registry: d.Registry, automations: d.Automations, permissions: d.Permissions, now: time.Now}, nil
}

func (s *Service) Registry() *domain.Registry { return s.registry }

func (s *Service) List(ctx context.Context, workspaceID string) ([]domain.Declaration, error) {
	rows, err := s.repo.List(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	return domain.NewCatalog(rows).All(), nil
}

func (s *Service) Described(ctx context.Context, workspaceID string) ([]domain.Declaration, error) {
	rows, err := s.List(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.registry.Described(rows), nil
}

func (s *Service) Catalog(ctx context.Context, workspaceID string) (domain.Catalog, error) {
	rows, err := s.List(ctx, workspaceID)
	if err != nil {
		return domain.Catalog{}, err
	}
	return domain.NewCatalog(rows), nil
}

func (s *Service) Editable(ctx context.Context, workspaceID string, editor Editor) ([]EditableDeclaration, error) {
	rows, err := s.List(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	users, err := s.users(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]EditableDeclaration, 0, len(rows))
	for _, d := range rows {
		out = append(out, EditableDeclaration{Declaration: d, LockedBy: s.locking(workspaceID, editor, users[d.Name])})
	}
	return out, nil
}

func (s *Service) Save(ctx context.Context, workspaceID string, editor Editor, d domain.Declaration) (domain.Declaration, error) {
	if err := s.guardChange(ctx, workspaceID, editor, strings.TrimSpace(d.Name), false); err != nil {
		return domain.Declaration{}, err
	}
	prepared, err := s.registry.Prepare(ctx, workspaceID, d)
	if err != nil {
		return domain.Declaration{}, err
	}
	checked, err := s.registry.CheckDeclaration(prepared)
	if err != nil {
		return domain.Declaration{}, err
	}
	checked.UpdatedBy = editor.Requester.UserID
	checked.UpdatedAt = s.now().UTC()
	if err := s.repo.Save(ctx, workspaceID, checked); err != nil {
		return domain.Declaration{}, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	return checked, nil
}

func (s *Service) Delete(ctx context.Context, workspaceID string, editor Editor, name string) error {
	name = strings.TrimSpace(name)
	if err := s.guardChange(ctx, workspaceID, editor, name, true); err != nil {
		return err
	}
	removed, err := s.repo.Delete(ctx, workspaceID, name)
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	if !removed {
		return fmt.Errorf("%w: %s", domain.ErrNotFound, name)
	}
	return nil
}

func (s *Service) Rejected(ctx context.Context, workspaceID string, values map[string]string) ([]string, error) {
	catalog, err := s.Catalog(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.registry.Rejected(ctx, workspaceID, catalog, values)
}

func (s *Service) guardChange(ctx context.Context, workspaceID string, editor Editor, name string, mustExist bool) error {
	catalog, err := s.Catalog(ctx, workspaceID)
	if err != nil {
		return err
	}
	if _, exists := catalog.Find(name); !exists {
		if mustExist {
			return fmt.Errorf("%w: %s", domain.ErrNotFound, name)
		}
		return nil
	}
	users, err := s.users(ctx, workspaceID)
	if err != nil {
		return err
	}
	if locked := s.locking(workspaceID, editor, users[name]); len(locked) > 0 {
		return &ChangeForbidden{Name: name, Users: locked}
	}
	return nil
}

func (s *Service) users(ctx context.Context, workspaceID string) (map[string][]automationneeds.DeclarationUser, error) {
	agents, workflows, err := s.automations.Automations(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	return automationneeds.DeclarationUsers(agents, workflows), nil
}

func (s *Service) locking(workspaceID string, editor Editor, users []automationneeds.DeclarationUser) []automationneeds.DeclarationUser {
	var locked []automationneeds.DeclarationUser
	for _, user := range users {
		if !s.mayChange(workspaceID, editor, user) {
			locked = append(locked, user)
		}
	}
	return locked
}

func (s *Service) mayChange(workspaceID string, editor Editor, user automationneeds.DeclarationUser) bool {
	resource := workspace.ResourceWorkflows
	if user.Kind == conversation.AutomationAgent {
		resource = workspace.ResourceAgents
	}
	if !s.permissions.HasWorkspacePermission(editor.Requester.UserID, workspaceID, string(resource), string(workspace.ActionUpdate), editor.Requester.SystemAdmin) {
		return false
	}
	return editor.Departments == nil || editor.Departments.CanAccess(user.DepartmentID)
}
