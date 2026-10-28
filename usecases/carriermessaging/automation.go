package carriermessaging_usecase

import (
	"context"
	"errors"
	"vozko/domain/automationneeds"

	"vozko/domain/conversation"
	"vozko/domain/rcs"
	"vozko/domain/sms"
	"vozko/usecases/shared/automationrefs"
)

var ErrAutomationIncomplete = errors.New("carrier messaging: an enabled automation needs its agent or workflow")

type AutomationRefs interface {
	OwnedBy(ctx context.Context, workspaceID string, refs automationrefs.Refs) error
}

type InboxSettings struct {
	DepartmentID *string
	PipelineID   *string
	Automation   conversation.ChannelAutomation
}

func (s *Service) checkInbox(ctx context.Context, workspaceID string, kind automationneeds.SlotKind, in InboxSettings) error {
	a := in.Automation
	if (a.EnableAgentResponses && a.AgentID == nil) || (a.EnableWorkflow && a.WorkflowID == nil) {
		return ErrAutomationIncomplete
	}
	if err := s.AutomationRefs.OwnedBy(ctx, workspaceID, automationrefs.Refs{
		AgentID: a.AgentID, WorkflowID: a.WorkflowID, PipelineID: in.PipelineID, DepartmentID: in.DepartmentID,
	}); err != nil {
		return err
	}
	return s.Receptive.Admit(workspaceID, kind, a)
}

func (s *Service) SetSenderInbox(ctx context.Context, workspaceID, id string, in InboxSettings) (*rcs.Sender, error) {
	if err := s.checkInbox(ctx, workspaceID, automationneeds.SlotRCS, in); err != nil {
		return nil, err
	}
	sender, err := s.Repos.Senders.FindByID(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	sender.DepartmentID, sender.PipelineID, sender.Automation = in.DepartmentID, in.PipelineID, in.Automation
	sender.UpdatedAt = s.Now()
	if err := s.Repos.Senders.Update(ctx, sender); err != nil {
		return nil, err
	}
	s.Receptive.Saved(ctx, workspaceID, automationneeds.SlotRCS, sender.ID, sender.Automation)
	return sender, nil
}

func (s *Service) SetRouteInbox(ctx context.Context, workspaceID, id string, in InboxSettings) (*sms.Route, error) {
	if err := s.checkInbox(ctx, workspaceID, automationneeds.SlotSMS, in); err != nil {
		return nil, err
	}
	route, err := s.Repos.Routes.FindByID(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	route.DepartmentID, route.PipelineID, route.Automation = in.DepartmentID, in.PipelineID, in.Automation
	saved, err := s.saveRoute(ctx, *route)
	if err != nil {
		return nil, err
	}
	s.Receptive.Saved(ctx, workspaceID, automationneeds.SlotSMS, saved.ID, saved.Automation)
	return saved, nil
}
