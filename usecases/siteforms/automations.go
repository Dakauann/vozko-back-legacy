package siteforms

import (
	"context"

	"vozko/domain/workflow"
	whatsappform_usecase "vozko/usecases/whatsappform"
)

type AutomationCatalog = whatsappform_usecase.AutomationCatalog

type FormAutomation = whatsappform_usecase.FormAutomation

type FormAutomations struct {
	FormKey     string
	Automations []FormAutomation
}

func (s *Service) keyOf(ctx context.Context, workspaceID, projectID, formID string) (string, error) {
	owner, err := s.d.Projects.WorkspaceOf(ctx, projectID)
	if err != nil {
		return "", err
	}
	if owner == "" || owner != workspaceID {
		return "", ErrProjectNotFound
	}
	keys, err := s.KeysFor(ctx, workspaceID, projectID, []string{formID})
	if err != nil {
		return "", err
	}
	return keys[formID], nil
}

func (s *Service) Automations(ctx context.Context, workspaceID, projectID, formID string) (FormAutomations, error) {
	key, err := s.keyOf(ctx, workspaceID, projectID, formID)
	if err != nil {
		return FormAutomations{}, err
	}
	listed, err := s.d.Automations.List(ctx, workspaceID, key)
	if err != nil {
		return FormAutomations{}, err
	}
	out := FormAutomations{FormKey: key, Automations: []FormAutomation{}}
	for _, a := range listed {
		if a.Use == string(workflow.FormUseTrigger) {
			out.Automations = append(out.Automations, a)
		}
	}
	return out, nil
}

func (s *Service) ScaffoldAutomation(ctx context.Context, workspaceID, userID, projectID, formID string) (string, error) {
	key, err := s.keyOf(ctx, workspaceID, projectID, formID)
	if err != nil {
		return "", err
	}
	return s.d.Automations.Scaffold(ctx, workspaceID, userID, key, formID)
}

func (s *Service) WorkflowBound(ctx context.Context, workspaceID, projectID, formID, workflowID string) (bool, error) {
	view, err := s.Automations(ctx, workspaceID, projectID, formID)
	if err != nil {
		return false, err
	}
	for _, a := range view.Automations {
		if a.WorkflowID == workflowID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) PageForm(ctx context.Context, workspaceID, formKey string) (PageForm, error) {
	form, err := s.d.Forms.WorkspaceForm(ctx, workspaceID, formKey)
	if err != nil {
		return PageForm{}, err
	}
	if form == nil {
		return PageForm{}, ErrFormUnknownForWorkspace
	}
	return *form, nil
}
