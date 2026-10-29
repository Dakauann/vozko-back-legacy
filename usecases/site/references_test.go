package site_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/emailinbox"
	"vozko/domain/customfield"
	"vozko/domain/pipeline"
	"vozko/domain/site"
	"vozko/domain/stage"
	uw "vozko/domain/unofficial_whatsapp"
	businessphone "vozko/domain/whatsapp/business_phone"
	"vozko/domain/workflow"
	"vozko/domain/workspace"
	wd "vozko/domain/workspace/workspace_department"
)

type refPipelines map[string]string

func (r refPipelines) GetByID(workspaceID, id string) (*pipeline.Pipeline, error) {
	if r[id] != workspaceID {
		return nil, errors.New("not found")
	}
	return &pipeline.Pipeline{ID: id, WorkspaceID: workspaceID}, nil
}

type refStages map[string]stage.Stage

func (r refStages) FindByID(id string) (*stage.Stage, error) {
	s, ok := r[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &s, nil
}

type refMembers map[string]string

func (r refMembers) GetMember(workspaceID, userID string) (*workspace.Member, error) {
	if r[userID] != workspaceID {
		return nil, errors.New("not a member")
	}
	return &workspace.Member{WorkspaceID: workspaceID, UserID: userID}, nil
}

type refDepartments map[string]string

func (r refDepartments) GetDepartmentByID(id string) (*wd.Department, error) {
	ws, ok := r[id]
	if !ok {
		return nil, nil
	}
	return &wd.Department{ID: id, WorkspaceID: ws}, nil
}

type refWorkflows map[string]string

func (r refWorkflows) FindByID(id string) (*workflow.Workflow, error) {
	ws, ok := r[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &workflow.Workflow{ID: id, WorkspaceID: ws}, nil
}

type refPhones map[string]string

func (r refPhones) FindByID(id string) (*businessphone.WhatsAppBusinessPhoneNumber, error) {
	ws, ok := r[id]
	if !ok {
		return nil, businessphone.ErrPhoneNumberNotFound
	}
	return &businessphone.WhatsAppBusinessPhoneNumber{ID: id, OwnerWorkspaceID: ws}, nil
}

type refInstances map[string]string

func (r refInstances) FindByID(_ context.Context, id string) (*uw.Instance, error) {
	ws, ok := r[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &uw.Instance{ID: id, WorkspaceID: ws}, nil
}

type refInboxes map[string]string

func (r refInboxes) FindByID(_ context.Context, workspaceID, id string) (*emailinbox.Inbox, error) {
	if r[id] != workspaceID {
		return nil, errors.New("not found")
	}
	return &emailinbox.Inbox{ID: id, WorkspaceID: workspaceID}, nil
}

func newReferences(t *testing.T) *WorkspaceReferences {
	t.Helper()
	refs, err := NewWorkspaceReferences(ReferenceDeps{
		Pipelines:   refPipelines{"f1": wsA, "f2": wsB},
		Stages:      refStages{"s1": {ID: "s1", WorkspaceID: wsA, PipelineID: "f1"}, "s2": {ID: "s2", WorkspaceID: wsA, PipelineID: "f9"}, "s3": {ID: "s3", WorkspaceID: wsB, PipelineID: "f2"}},
		Members:     refMembers{"u1": wsA, "u2": wsB},
		Departments: refDepartments{"d1": wsA, "d2": wsB},
		Workflows:   refWorkflows{"w1": wsA, "w2": wsB},
		Phones:      refPhones{"p1": wsA, "p2": wsB, "p3": ""},
		PhoneList:   refPhoneList{"p1": wsA, "p2": wsB},
		Instances:   refInstances{"i1": wsA, "i2": wsB},
		Inboxes:     refInboxes{"e1": wsA, "e2": wsB},
		Widgets:     refWidgets{"c1": wsA, "c2": wsB},
		Calendars:   refCalendars{wsA: true},
		LeadFields:  refLeadFields{wsA: {{Key: "cidade", Label: "Cidade", Type: customfield.TypeText, ObjectType: customfield.ObjectLead}, {Key: "cpf", Label: "CPF", Type: customfield.TypeText, ObjectType: customfield.ObjectLead, Sensitive: true}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return refs
}

type refLeadFields map[string][]*customfield.Definition

func (r refLeadFields) ListByObject(workspaceID string, objectType customfield.ObjectType) ([]*customfield.Definition, error) {
	if objectType != customfield.ObjectLead {
		return nil, errors.New("only lead fields")
	}
	return r[workspaceID], nil
}

func TestFormReferencesMustBelongToTheWorkspace(t *testing.T) {
	refs := newReferences(t)
	ok := site.FormSettings{
		FunnelID: "f1", StageID: "s1", Owner: &site.Owner{Kind: site.OwnerUser, ID: "u1"}, WorkflowID: "w1",
		FollowUp: &site.FollowUp{Kind: site.FollowUpWhatsApp, ID: "p1"},
	}
	if err := refs.Check(context.Background(), wsA, "contato", ok); err != nil {
		t.Fatal(err)
	}
	for _, accepted := range []site.FormSettings{
		{},
		{StageID: "s1"},
		{Owner: &site.Owner{Kind: site.OwnerUnassigned}},
		{Owner: &site.Owner{Kind: site.OwnerRoulette, ID: "d1"}},
		{WorkflowID: "w1", FollowUp: &site.FollowUp{Kind: site.FollowUpUnofficialWhatsApp, ID: "i1"}},
		{WorkflowID: "w1", FollowUp: &site.FollowUp{Kind: site.FollowUpEmail, ID: "e1"}},
	} {
		if err := refs.Check(context.Background(), wsA, "contato", accepted); err != nil {
			t.Fatalf("%+v: %v", accepted, err)
		}
	}
	cases := map[string]struct {
		form site.FormSettings
		want error
	}{
		"foreign funnel":     {site.FormSettings{FunnelID: "f2"}, ErrFormFunnelUnknown},
		"missing funnel":     {site.FormSettings{FunnelID: "f0"}, ErrFormFunnelUnknown},
		"stage of another":   {site.FormSettings{FunnelID: "f1", StageID: "s2"}, ErrFormStageUnknown},
		"foreign stage":      {site.FormSettings{StageID: "s3"}, ErrFormStageUnknown},
		"missing stage":      {site.FormSettings{StageID: "s0"}, ErrFormStageUnknown},
		"foreign user":       {site.FormSettings{Owner: &site.Owner{Kind: site.OwnerUser, ID: "u2"}}, ErrFormOwnerUnknown},
		"foreign roulette":   {site.FormSettings{Owner: &site.Owner{Kind: site.OwnerRoulette, ID: "d2"}}, ErrFormOwnerUnknown},
		"missing roulette":   {site.FormSettings{Owner: &site.Owner{Kind: site.OwnerRoulette, ID: "d0"}}, ErrFormOwnerUnknown},
		"unknown owner kind": {site.FormSettings{Owner: &site.Owner{Kind: "boss", ID: "u1"}}, ErrFormOwnerUnknown},
		"foreign workflow":   {site.FormSettings{WorkflowID: "w2", FollowUp: &site.FollowUp{Kind: site.FollowUpWhatsApp, ID: "p1"}}, ErrFormWorkflowUnknown},
		"workflow no follow": {site.FormSettings{WorkflowID: "w1"}, site.ErrFormChannelRequired},
		"foreign phone":      {site.FormSettings{FollowUp: &site.FollowUp{Kind: site.FollowUpWhatsApp, ID: "p2"}}, ErrFormChannelUnknown},
		"unowned phone":      {site.FormSettings{FollowUp: &site.FollowUp{Kind: site.FollowUpWhatsApp, ID: "p3"}}, ErrFormChannelUnknown},
		"foreign instance":   {site.FormSettings{FollowUp: &site.FollowUp{Kind: site.FollowUpUnofficialWhatsApp, ID: "i2"}}, ErrFormChannelUnknown},
		"foreign inbox":      {site.FormSettings{FollowUp: &site.FollowUp{Kind: site.FollowUpEmail, ID: "e2"}}, ErrFormChannelUnknown},
		"unknown channel":    {site.FormSettings{FollowUp: &site.FollowUp{Kind: "sms", ID: "x"}}, ErrFormChannelUnknown},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := refs.Check(context.Background(), wsA, "contato", c.form)
			var refErr *FormReferenceError
			if !errors.Is(err, c.want) || !errors.As(err, &refErr) || refErr.Form != "contato" {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestWorkspaceReferencesNeedEveryReader(t *testing.T) {
	if _, err := NewWorkspaceReferences(ReferenceDeps{}); !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("got %v", err)
	}
}
