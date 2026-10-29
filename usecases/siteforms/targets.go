package siteforms

import (
	"context"
	"fmt"

	"vozko/domain/customfield"
	"vozko/domain/site"
	"vozko/domain/siteform"
)

type LeadFieldDefinitions interface {
	ListByObject(workspaceID string, objectType customfield.ObjectType) ([]*customfield.Definition, error)
}

type FormTarget struct {
	Target string          `json:"target"`
	Label  string          `json:"label,omitempty"`
	Custom bool            `json:"custom"`
	Kinds  []siteform.Kind `json:"kinds"`
}

type FormTargets struct {
	Targets    []FormTarget    `json:"targets"`
	Settable   []siteform.Kind `json:"settable"`
	Insertable []siteform.Kind `json:"insertable"`
}

func (s *Service) FormTargets(_ context.Context, workspaceID string) (FormTargets, error) {
	defs, err := s.d.LeadFields.ListByObject(workspaceID, customfield.ObjectLead)
	if err != nil {
		return FormTargets{}, fmt.Errorf("lead fields of workspace %s: %w", workspaceID, err)
	}
	out := FormTargets{Settable: site.SettableFormInputs(), Insertable: site.InsertableFormInputs()}
	for _, target := range siteform.LeadTargets() {
		out.Targets = append(out.Targets, FormTarget{Target: target, Kinds: siteform.TargetKinds(target)})
	}
	for _, target := range siteform.CustomTargets(defs) {
		out.Targets = append(out.Targets, FormTarget{Target: target.Target, Label: target.Label, Custom: true, Kinds: target.Kinds})
	}
	return out, nil
}
