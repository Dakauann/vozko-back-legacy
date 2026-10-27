package campaignaudience_usecase

import (
	"context"
	"fmt"
	"strings"

	"vozko/domain/campaignaudience"
	"vozko/domain/customfield"
	"vozko/domain/whatsapp/template"
)

func (s *Service) definitions(workspaceID string) ([]*customfield.Definition, error) {
	defs, err := s.deps.Definitions.ListByObject(workspaceID, customfield.ObjectLead)
	if err != nil {
		return nil, fmt.Errorf("lead fields of workspace %s: %w", workspaceID, err)
	}
	return defs, nil
}

func (s *Service) Fields(_ context.Context, a Actor) ([]campaignaudience.Field, error) {
	defs, err := s.definitions(a.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return campaignaudience.FieldsOf(defs), nil
}

type TemplateReader interface {
	Get(workspaceID, templateID string) (*template.Template, error)
}

type TemplateSuggestions struct {
	TemplateID  string                        `json:"templateId"`
	Suggestions []campaignaudience.Suggestion `json:"suggestions"`
}

func (s *Service) suggestionTemplate(a Actor, templateID string) (*template.Template, error) {
	if s.deps.Templates == nil || s.deps.Suggestions == nil {
		return nil, campaignaudience.ErrUnavailable
	}
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil, campaignaudience.ErrTemplateRequired
	}
	tmpl, err := s.deps.Templates.Get(a.WorkspaceID, templateID)
	if err != nil {
		return nil, err
	}
	if tmpl == nil {
		return nil, template.ErrTemplateNotFound
	}
	return tmpl, nil
}

func (s *Service) TemplateSuggestions(ctx context.Context, a Actor, templateID string) (TemplateSuggestions, error) {
	tmpl, err := s.suggestionTemplate(a, templateID)
	if err != nil {
		return TemplateSuggestions{}, err
	}
	stored, err := s.deps.Suggestions.Get(ctx, a.WorkspaceID, tmpl.ID)
	if err != nil {
		return TemplateSuggestions{}, err
	}
	if stored == nil {
		stored = []campaignaudience.Suggestion{}
	}
	return TemplateSuggestions{TemplateID: tmpl.ID, Suggestions: stored}, nil
}

func (s *Service) SaveTemplateSuggestions(ctx context.Context, a Actor, templateID string, suggestions []campaignaudience.Suggestion) (TemplateSuggestions, error) {
	tmpl, err := s.suggestionTemplate(a, templateID)
	if err != nil {
		return TemplateSuggestions{}, err
	}
	defs, err := s.definitions(a.WorkspaceID)
	if err != nil {
		return TemplateSuggestions{}, err
	}
	valid, err := campaignaudience.ValidateSuggestions(suggestions, tmpl.ParameterCount(), defs)
	if err != nil {
		return TemplateSuggestions{}, err
	}
	if err := s.deps.Suggestions.Replace(ctx, a.WorkspaceID, tmpl.ID, a.UserID, valid); err != nil {
		return TemplateSuggestions{}, err
	}
	return TemplateSuggestions{TemplateID: tmpl.ID, Suggestions: valid}, nil
}
