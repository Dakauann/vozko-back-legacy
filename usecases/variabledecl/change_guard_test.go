package variabledecl

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"vozko/domain/agent"
	"vozko/domain/automationneeds"
	"vozko/domain/conversation"
	domain "vozko/domain/variabledecl"
	"vozko/domain/workflow"
	"vozko/domain/workspace"
	workspace_department "vozko/domain/workspace/workspace_department"
)

type fakeAutomations struct {
	agents    []*agent.Agent
	workflows []*workflow.Workflow
	err       error
}

func (f *fakeAutomations) Automations(context.Context, string) ([]*agent.Agent, []*workflow.Workflow, error) {
	return f.agents, f.workflows, f.err
}

type grants map[string]bool

func (g grants) HasWorkspacePermission(userID, _, resource, action string, isSystemAdmin bool) bool {
	return isSystemAdmin || g[userID+":"+resource+":"+action]
}

func editor(userID string) Editor {
	return Editor{Requester: workspace.Requester{UserID: userID}}
}

func usedDeclaration() domain.Declaration {
	return domain.Declaration{Name: "turno", Label: "Turno", Type: domain.Type{Kind: domain.KindText}}
}

func guarded(t *testing.T, permissions grants) (*Service, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	repo.rows["ws"] = map[string]domain.Declaration{"turno": usedDeclaration(), "livre": {Name: "livre", Label: "Livre", Type: domain.Type{Kind: domain.KindText}}}
	s := newService(t, repo)
	s.automations = &fakeAutomations{
		agents:    []*agent.Agent{{ID: "a1", Name: "Cobrador", DepartmentID: "vendas", MessagingPrompt: "Turno {{turno}}"}},
		workflows: []*workflow.Workflow{{ID: "w1", Name: "Matrículas", DepartmentID: "suporte", Graph: workflow.Graph{Variables: []workflow.VariableUse{{Name: "turno"}}}}},
	}
	s.permissions = permissions
	return s, repo
}

var (
	agentUser = automationneeds.DeclarationUser{Link: automationneeds.Link{Automation: conversation.Automation{Kind: conversation.AutomationAgent, ID: "a1"}, Name: "Cobrador"}, DepartmentID: "vendas"}
	flowUser  = automationneeds.DeclarationUser{Link: automationneeds.Link{Automation: conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "w1"}, Name: "Matrículas"}, DepartmentID: "suporte"}
)

func TestChangingADeclarationNeedsUpdateRightsOnEverythingThatUsesIt(t *testing.T) {
	both := grants{"u:agents:update": true, "u:workflows:update": true}
	s, repo := guarded(t, grants{"u:workflows:update": true})
	_, err := s.Save(context.Background(), "ws", editor("u"), domain.Declaration{Name: "turno", Label: "Período", Type: domain.Type{Kind: domain.KindText}})
	var refused *ChangeForbidden
	if !errors.As(err, &refused) || !errors.Is(err, domain.ErrChangeForbidden) || refused.Name != "turno" || !reflect.DeepEqual(refused.Users, []automationneeds.DeclarationUser{agentUser}) {
		t.Fatalf("err = %v", err)
	}
	if repo.rows["ws"]["turno"].Label != "Turno" {
		t.Fatal("a refused change was stored")
	}
	if err := s.Delete(context.Background(), "ws", editor("u"), "turno"); !errors.As(err, &refused) || !reflect.DeepEqual(refused.Users, []automationneeds.DeclarationUser{agentUser}) {
		t.Fatalf("delete = %v", err)
	}
	s.permissions = both
	if _, err := s.Save(context.Background(), "ws", editor("u"), domain.Declaration{Name: "turno", Label: "Período", Type: domain.Type{Kind: domain.KindText}}); err != nil {
		t.Fatalf("with both grants = %v", err)
	}
	scoped := Editor{Requester: workspace.Requester{UserID: "u"}, Departments: &workspace_department.DepartmentFilter{DepartmentIDs: []string{"suporte"}}}
	if _, err := s.Save(context.Background(), "ws", scoped, usedDeclaration()); !errors.As(err, &refused) || !reflect.DeepEqual(refused.Users, []automationneeds.DeclarationUser{agentUser}) {
		t.Fatalf("an agent outside the person's departments = %v", err)
	}
}

func TestCreatingOrChangingAnUnusedDeclarationNeedsNoOtherGrant(t *testing.T) {
	s, _ := guarded(t, grants{})
	if _, err := s.Save(context.Background(), "ws", editor("u"), domain.Declaration{Name: "nova", Label: "Nova", Type: domain.Type{Kind: domain.KindText}}); err != nil {
		t.Fatalf("create = %v", err)
	}
	if _, err := s.Save(context.Background(), "ws", editor("u"), domain.Declaration{Name: "livre", Label: "Livre 2", Type: domain.Type{Kind: domain.KindText}}); err != nil {
		t.Fatalf("unused = %v", err)
	}
	if err := s.Delete(context.Background(), "ws", editor("u"), "livre"); err != nil {
		t.Fatalf("unused delete = %v", err)
	}
}

func TestEditableNamesWhatLocksEachDeclarationForThisPerson(t *testing.T) {
	s, _ := guarded(t, grants{"u:agents:update": true})
	rows, err := s.Editable(context.Background(), "ws", editor("u"))
	if err != nil {
		t.Fatal(err)
	}
	locks := map[string][]automationneeds.DeclarationUser{}
	for _, row := range rows {
		locks[row.Name] = row.LockedBy
	}
	if !reflect.DeepEqual(locks, map[string][]automationneeds.DeclarationUser{"livre": nil, "turno": {flowUser}}) {
		t.Fatalf("locks = %+v", locks)
	}
	admin := Editor{Requester: workspace.Requester{UserID: "x", SystemAdmin: true}}
	if rows, _ := s.Editable(context.Background(), "ws", admin); rows[1].LockedBy != nil {
		t.Fatalf("a system admin = %+v", rows)
	}
}

func TestTheChangeGuardFailsClosedWhenUsesCannotBeRead(t *testing.T) {
	s, _ := guarded(t, grants{"u:agents:update": true, "u:workflows:update": true})
	s.automations = &fakeAutomations{err: errors.New("db down")}
	if _, err := s.Save(context.Background(), "ws", editor("u"), usedDeclaration()); !errors.Is(err, domain.ErrUnreadable) {
		t.Fatalf("save = %v", err)
	}
	if err := s.Delete(context.Background(), "ws", editor("u"), "turno"); !errors.Is(err, domain.ErrUnreadable) {
		t.Fatalf("delete = %v", err)
	}
	if _, err := s.Editable(context.Background(), "ws", editor("u")); !errors.Is(err, domain.ErrUnreadable) {
		t.Fatalf("editable = %v", err)
	}
}

func TestNewServiceRefusesMissingDependencies(t *testing.T) {
	registry, _ := domain.NewRegistry(domain.Builtin()...)
	for name, deps := range map[string]ServiceDeps{
		"repo":        {Registry: registry, Automations: &fakeAutomations{}, Permissions: grants{}},
		"automations": {Repo: newFakeRepo(), Registry: registry, Permissions: grants{}},
		"permissions": {Repo: newFakeRepo(), Registry: registry, Automations: &fakeAutomations{}},
	} {
		if _, err := NewService(deps); err == nil {
			t.Fatalf("%s missing was accepted", name)
		}
	}
}
