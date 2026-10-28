package site_usecase

import (
	"context"
	"strings"

	"vozko/domain/pipeline"
	"vozko/domain/shared"
	uw "vozko/domain/unofficial_whatsapp"
	businessphone "vozko/domain/whatsapp/business_phone"
)

const maxCatalogItems = 50

type ReferenceItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type StageReference struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FunnelID string `json:"funnel_id"`
}

type ReferenceCatalog struct {
	WhatsAppNumbers   []ReferenceItem  `json:"whatsapp_numbers"`
	UnofficialNumbers []ReferenceItem  `json:"unofficial_numbers"`
	Funnels           []ReferenceItem  `json:"funnels"`
	Stages            []StageReference `json:"stages"`
	Members           []ReferenceItem  `json:"members"`
	Departments       []ReferenceItem  `json:"departments"`
	Workflows         []ReferenceItem  `json:"workflows"`
	EmailInboxes      []ReferenceItem  `json:"email_inboxes"`
}

func capped[T any](items []T) []T {
	if len(items) > maxCatalogItems {
		return items[:maxCatalogItems]
	}
	return items
}

func labelled(name, detail string) string {
	name, detail = strings.TrimSpace(name), strings.TrimSpace(detail)
	switch {
	case name == "":
		return detail
	case detail == "" || detail == name:
		return name
	}
	return name + " (" + detail + ")"
}

func (r *WorkspaceReferences) Catalog(ctx context.Context, workspaceID string) (ReferenceCatalog, error) {
	if workspaceID == "" {
		return ReferenceCatalog{}, ErrWorkspaceRequired
	}
	out := ReferenceCatalog{
		WhatsAppNumbers: []ReferenceItem{}, UnofficialNumbers: []ReferenceItem{}, Funnels: []ReferenceItem{}, Stages: []StageReference{},
		Members: []ReferenceItem{}, Departments: []ReferenceItem{}, Workflows: []ReferenceItem{}, EmailInboxes: []ReferenceItem{},
	}
	page := shared.QueryOptions{Pagination: shared.Pagination{Page: 1, PageSize: maxCatalogItems}}
	phones, err := r.deps.PhoneList.List(workspaceID, businessphone.ListInput{Options: page})
	if err != nil {
		return ReferenceCatalog{}, err
	}
	for _, p := range phones.Items {
		if p != nil && p.BelongsToWorkspace(workspaceID) {
			out.WhatsAppNumbers = append(out.WhatsAppNumbers, ReferenceItem{ID: p.ID, Name: labelled(p.VerifiedName, p.DisplayPhoneNumber)})
		}
	}
	instances, err := r.deps.Instances.ListByWorkspace(ctx, uw.ListInstancesInput{WorkspaceID: workspaceID, Options: page})
	if err != nil {
		return ReferenceCatalog{}, err
	}
	for _, i := range instances.Items {
		if i != nil && i.WorkspaceID == workspaceID {
			out.UnofficialNumbers = append(out.UnofficialNumbers, ReferenceItem{ID: i.ID, Name: labelled(i.Label(), i.PhoneNumber)})
		}
	}
	funnels, err := r.deps.Pipelines.ListByWorkspace(workspaceID, string(pipeline.ObjectOpportunity))
	if err != nil {
		return ReferenceCatalog{}, err
	}
	for _, f := range funnels {
		if f != nil && f.WorkspaceID == workspaceID {
			out.Funnels = append(out.Funnels, ReferenceItem{ID: f.ID, Name: f.Name})
		}
	}
	stages, err := r.deps.Stages.ListByWorkspace(workspaceID)
	if err != nil {
		return ReferenceCatalog{}, err
	}
	for _, s := range stages {
		if s != nil && s.WorkspaceID == workspaceID {
			out.Stages = append(out.Stages, StageReference{ID: s.ID, Name: s.Name, FunnelID: s.PipelineID})
		}
	}
	members, err := r.deps.Members.ListMembers(workspaceID)
	if err != nil {
		return ReferenceCatalog{}, err
	}
	for _, m := range members {
		if m != nil && m.WorkspaceID == workspaceID {
			out.Members = append(out.Members, ReferenceItem{ID: m.UserID, Name: labelled(m.Username, m.Email)})
		}
	}
	departments, err := r.deps.Departments.ListDepartments(workspaceID)
	if err != nil {
		return ReferenceCatalog{}, err
	}
	for _, d := range departments {
		if d.WorkspaceID == workspaceID {
			out.Departments = append(out.Departments, ReferenceItem{ID: d.ID, Name: d.Name})
		}
	}
	workflows, err := r.deps.Workflows.FindByWorkspaceID(workspaceID)
	if err != nil {
		return ReferenceCatalog{}, err
	}
	for _, w := range workflows {
		if w != nil && w.WorkspaceID == workspaceID {
			out.Workflows = append(out.Workflows, ReferenceItem{ID: w.ID, Name: w.Name})
		}
	}
	inboxes, err := r.deps.Inboxes.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return ReferenceCatalog{}, err
	}
	for _, i := range inboxes {
		if i != nil && i.WorkspaceID == workspaceID {
			out.EmailInboxes = append(out.EmailInboxes, ReferenceItem{ID: i.ID, Name: labelled(i.Name, i.Address.String())})
		}
	}
	out.WhatsAppNumbers, out.UnofficialNumbers, out.Funnels = capped(out.WhatsAppNumbers), capped(out.UnofficialNumbers), capped(out.Funnels)
	out.Stages, out.Members, out.Departments = capped(out.Stages), capped(out.Members), capped(out.Departments)
	out.Workflows, out.EmailInboxes = capped(out.Workflows), capped(out.EmailInboxes)
	return out, nil
}
