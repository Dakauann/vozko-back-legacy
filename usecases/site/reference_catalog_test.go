package site_usecase

import (
	"context"
	"errors"
	"sort"
	"testing"

	"vozko/domain/emailinbox"
	"vozko/domain/pipeline"
	"vozko/domain/shared"
	"vozko/domain/stage"
	uw "vozko/domain/unofficial_whatsapp"
	businessphone "vozko/domain/whatsapp/business_phone"
	"vozko/domain/workflow"
	"vozko/domain/workspace"
	wd "vozko/domain/workspace/workspace_department"
)

func idsFor[T any](owners map[string]string, workspaceID string, build func(id string) T) []T {
	var ids []string
	for id, ws := range owners {
		if ws == workspaceID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	out := make([]T, 0, len(ids))
	for _, id := range ids {
		out = append(out, build(id))
	}
	return out
}

func (r refPipelines) ListByWorkspace(workspaceID, objectType string) ([]*pipeline.Pipeline, error) {
	return idsFor(r, workspaceID, func(id string) *pipeline.Pipeline {
		return &pipeline.Pipeline{ID: id, WorkspaceID: workspaceID, Name: "Funil " + id, ObjectType: pipeline.ObjectType(objectType)}
	}), nil
}

func (r refStages) ListByWorkspace(workspaceID string) ([]*stage.Stage, error) {
	var out []*stage.Stage
	for _, id := range []string{"s1", "s2", "s3"} {
		if s, ok := r[id]; ok && s.WorkspaceID == workspaceID {
			s.Name = "Etapa " + id
			out = append(out, &s)
		}
	}
	return out, nil
}

func (r refMembers) ListMembers(workspaceID string) ([]*workspace.Member, error) {
	return idsFor(r, workspaceID, func(id string) *workspace.Member {
		return &workspace.Member{WorkspaceID: workspaceID, UserID: id, Username: "Pessoa " + id}
	}), nil
}

func (r refDepartments) ListDepartments(workspaceID string) ([]wd.Department, error) {
	return idsFor(r, workspaceID, func(id string) wd.Department {
		return wd.Department{ID: id, WorkspaceID: workspaceID, Name: "Time " + id}
	}), nil
}

func (r refWorkflows) FindByWorkspaceID(workspaceID string) ([]*workflow.Workflow, error) {
	return idsFor(r, workspaceID, func(id string) *workflow.Workflow {
		return &workflow.Workflow{ID: id, WorkspaceID: workspaceID, Name: "Fluxo " + id}
	}), nil
}

type refPhoneList map[string]string

func (r refPhoneList) List(workspaceID string, _ businessphone.ListInput) (*shared.PaginatedResult[*businessphone.WhatsAppBusinessPhoneNumber], error) {
	return &shared.PaginatedResult[*businessphone.WhatsAppBusinessPhoneNumber]{Items: idsFor(r, workspaceID, func(id string) *businessphone.WhatsAppBusinessPhoneNumber {
		return &businessphone.WhatsAppBusinessPhoneNumber{ID: id, OwnerWorkspaceID: workspaceID, VerifiedName: "Clínica", DisplayPhoneNumber: "+55 11 90000-0000"}
	})}, nil
}

func (r refInstances) ListByWorkspace(_ context.Context, in uw.ListInstancesInput) (*shared.PaginatedResult[*uw.Instance], error) {
	return &shared.PaginatedResult[*uw.Instance]{Items: idsFor(r, in.WorkspaceID, func(id string) *uw.Instance {
		return &uw.Instance{ID: id, WorkspaceID: in.WorkspaceID, DisplayName: "Celular " + id, PhoneNumber: "5511988887777"}
	})}, nil
}

func (r refInboxes) ListByWorkspace(_ context.Context, workspaceID string) ([]*emailinbox.Inbox, error) {
	return idsFor(r, workspaceID, func(id string) *emailinbox.Inbox {
		return &emailinbox.Inbox{ID: id, WorkspaceID: workspaceID, Name: "Contato " + id}
	}), nil
}

func TestTheReferenceCatalogListsOnlyThisWorkspace(t *testing.T) {
	refs := newReferences(t)
	got, err := refs.Catalog(context.Background(), wsA)
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string][]ReferenceItem{
		"funnels": got.Funnels, "members": got.Members, "departments": got.Departments, "workflows": got.Workflows,
		"official": got.WhatsAppNumbers, "unofficial": got.UnofficialNumbers, "inboxes": got.EmailInboxes,
	}
	want := map[string]ReferenceItem{
		"funnels": {ID: "f1", Name: "Funil f1"}, "members": {ID: "u1", Name: "Pessoa u1"}, "departments": {ID: "d1", Name: "Time d1"},
		"workflows": {ID: "w1", Name: "Fluxo w1"}, "official": {ID: "p1", Name: "Clínica (+55 11 90000-0000)"},
		"unofficial": {ID: "i1", Name: "Celular i1 (5511988887777)"}, "inboxes": {ID: "e1", Name: "Contato e1"},
	}
	for name, items := range checks {
		if len(items) != 1 || items[0] != want[name] {
			t.Fatalf("%s = %+v", name, items)
		}
	}
	if len(got.Stages) != 2 || got.Stages[0] != (StageReference{ID: "s1", Name: "Etapa s1", FunnelID: "f1"}) {
		t.Fatalf("stages %+v", got.Stages)
	}
}

type failingInboxes struct{ refInboxes }

func (failingInboxes) ListByWorkspace(context.Context, string) ([]*emailinbox.Inbox, error) {
	return nil, errors.New("db down")
}

func TestTheReferenceCatalogRefusesWhenAListCannotBeRead(t *testing.T) {
	deps := newReferences(t).deps
	deps.Inboxes = failingInboxes{}
	refs, err := NewWorkspaceReferences(deps)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := refs.Catalog(context.Background(), wsA); err == nil {
		t.Fatal("a list that cannot be read refuses instead of looking empty")
	}
}
