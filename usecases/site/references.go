package site_usecase

import (
	"context"
	"errors"

	"vozko/domain/customfield"
	"vozko/domain/emailinbox"
	"vozko/domain/pipeline"
	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/domain/stage"
	uw "vozko/domain/unofficial_whatsapp"
	"vozko/domain/webchat"
	businessphone "vozko/domain/whatsapp/business_phone"
	"vozko/domain/workflow"
	"vozko/domain/workspace"
	wd "vozko/domain/workspace/workspace_department"
)

type ReferenceDeps struct {
	Pipelines interface {
		GetByID(workspaceID, id string) (*pipeline.Pipeline, error)
		ListByWorkspace(workspaceID, objectType string) ([]*pipeline.Pipeline, error)
	}
	Stages interface {
		FindByID(id string) (*stage.Stage, error)
		ListByWorkspace(workspaceID string) ([]*stage.Stage, error)
	}
	Members interface {
		GetMember(workspaceID, userID string) (*workspace.Member, error)
		ListMembers(workspaceID string) ([]*workspace.Member, error)
	}
	Departments interface {
		GetDepartmentByID(id string) (*wd.Department, error)
		ListDepartments(workspaceID string) ([]wd.Department, error)
	}
	Workflows interface {
		FindByID(workflowID string) (*workflow.Workflow, error)
		FindByWorkspaceID(workspaceID string) ([]*workflow.Workflow, error)
	}
	Phones interface {
		FindByID(id string) (*businessphone.WhatsAppBusinessPhoneNumber, error)
	}
	PhoneList interface {
		List(workspaceID string, input businessphone.ListInput) (*shared.PaginatedResult[*businessphone.WhatsAppBusinessPhoneNumber], error)
	}
	Instances interface {
		FindByID(ctx context.Context, id string) (*uw.Instance, error)
		ListByWorkspace(ctx context.Context, in uw.ListInstancesInput) (*shared.PaginatedResult[*uw.Instance], error)
	}
	Inboxes interface {
		FindByID(ctx context.Context, workspaceID, id string) (*emailinbox.Inbox, error)
		ListByWorkspace(ctx context.Context, workspaceID string) ([]*emailinbox.Inbox, error)
	}
	Widgets interface {
		webchat.SiteWidgetFinder
		ListByWorkspace(ctx context.Context, in webchat.ListWidgetsInput) (*shared.PaginatedResult[*webchat.Widget], error)
	}
	Calendars interface {
		Connected(ctx context.Context, workspaceID string) (bool, error)
	}
	LeadFields interface {
		ListByObject(workspaceID string, objectType customfield.ObjectType) ([]*customfield.Definition, error)
	}
}

type WorkspaceReferences struct {
	deps ReferenceDeps
}

var _ FormReferences = (*WorkspaceReferences)(nil)

func NewWorkspaceReferences(deps ReferenceDeps) (*WorkspaceReferences, error) {
	if deps.Pipelines == nil || deps.Stages == nil || deps.Members == nil || deps.Departments == nil ||
		deps.Workflows == nil || deps.Phones == nil || deps.PhoneList == nil || deps.Instances == nil || deps.Inboxes == nil || deps.Widgets == nil || deps.Calendars == nil || deps.LeadFields == nil {
		return nil, ErrMissingDependency
	}
	return &WorkspaceReferences{deps: deps}, nil
}

func (r *WorkspaceReferences) Check(ctx context.Context, workspaceID, formID string, form site.FormSettings) error {
	if err := r.check(ctx, workspaceID, form); err != nil {
		return &FormReferenceError{Form: formID, Err: err}
	}
	return nil
}

func (r *WorkspaceReferences) check(ctx context.Context, workspaceID string, form site.FormSettings) error {
	if form.FunnelID != "" && !r.funnel(workspaceID, form.FunnelID) {
		return ErrFormFunnelUnknown
	}
	if form.StageID != "" && !r.stage(workspaceID, form.FunnelID, form.StageID) {
		return ErrFormStageUnknown
	}
	if form.Owner != nil && !r.owner(workspaceID, *form.Owner) {
		return ErrFormOwnerUnknown
	}
	if form.WorkflowID != "" {
		if form.FollowUp == nil {
			return site.ErrFormChannelRequired
		}
		if !r.workflow(workspaceID, form.WorkflowID) {
			return ErrFormWorkflowUnknown
		}
	}
	if form.FollowUp != nil && !r.channel(ctx, workspaceID, *form.FollowUp) {
		return ErrFormChannelUnknown
	}
	return nil
}

func (r *WorkspaceReferences) CheckWebchat(ctx context.Context, workspaceID, ref string) error {
	_, err := webchat.FindSiteWidget(ctx, r.deps.Widgets, workspaceID, ref)
	if errors.Is(err, webchat.ErrWidgetNotFound) {
		return ErrWebchatWidgetUnknown
	}
	return err
}

func (r *WorkspaceReferences) funnel(workspaceID, id string) bool {
	p, err := r.deps.Pipelines.GetByID(workspaceID, id)
	return err == nil && p != nil && p.WorkspaceID == workspaceID
}

func (r *WorkspaceReferences) stage(workspaceID, funnelID, id string) bool {
	s, err := r.deps.Stages.FindByID(id)
	if err != nil || s == nil || s.WorkspaceID != workspaceID {
		return false
	}
	return funnelID == "" || s.PipelineID == funnelID
}

func (r *WorkspaceReferences) owner(workspaceID string, owner site.Owner) bool {
	switch owner.Kind {
	case site.OwnerUnassigned:
		return owner.ID == ""
	case site.OwnerUser:
		m, err := r.deps.Members.GetMember(workspaceID, owner.ID)
		return err == nil && m != nil && m.WorkspaceID == workspaceID
	case site.OwnerRoulette:
		d, err := r.deps.Departments.GetDepartmentByID(owner.ID)
		return err == nil && d != nil && d.WorkspaceID == workspaceID
	}
	return false
}

func (r *WorkspaceReferences) workflow(workspaceID, id string) bool {
	w, err := r.deps.Workflows.FindByID(id)
	return err == nil && w != nil && w.WorkspaceID == workspaceID
}

func (r *WorkspaceReferences) channel(ctx context.Context, workspaceID string, follow site.FollowUp) bool {
	switch follow.Kind {
	case site.FollowUpWhatsApp:
		p, err := r.deps.Phones.FindByID(follow.ID)
		return err == nil && p != nil && p.BelongsToWorkspace(workspaceID)
	case site.FollowUpUnofficialWhatsApp:
		i, err := r.deps.Instances.FindByID(ctx, follow.ID)
		return err == nil && i != nil && i.WorkspaceID == workspaceID
	case site.FollowUpEmail:
		inbox, err := r.deps.Inboxes.FindByID(ctx, workspaceID, follow.ID)
		return err == nil && inbox != nil && inbox.WorkspaceID == workspaceID
	}
	return false
}
