package automationneeds

import (
	"context"
	"fmt"
	"strings"

	domain "vozko/domain/automationneeds"
	"vozko/domain/campaign"
	"vozko/domain/campaignaudience"
	"vozko/domain/conversation"
	"vozko/domain/conversationvars"
	"vozko/domain/customfield"
	"vozko/domain/shared"
)

type ConversationSnapshots interface {
	Snapshot(ctx context.Context, entryID string, entryType shared.EntryType) (conversationvars.Snapshot, error)
}

type LeadFieldDefinitions interface {
	ListByObject(workspaceID string, objectType customfield.ObjectType) ([]*customfield.Definition, error)
}

type LeadSubjects interface {
	Identified(ctx context.Context, workspaceID string, ids []string, bindings campaign.BindingPlan) (map[string]campaign.BindingSubject, error)
}

type ConversationRef struct {
	WorkspaceID string
	UserID      string
	IsAdmin     bool
	EntryID     string
	EntryType   shared.EntryType
}

type VariableRow struct {
	Need      domain.Need
	Current   string
	Suggested campaign.BindingSource
}

type FieldValue struct {
	Field campaignaudience.Field
	Value string
}

type VariablesForm struct {
	Automation *domain.Link
	Rows       []VariableRow
	Fields     []FieldValue
}

type SaveValues struct {
	ConversationRef
	Automation conversation.Automation
	Values     map[string]string
	Bindings   []conversationvars.Binding
}

func (q *Queries) VariablesForm(ctx context.Context, in ConversationRef, target *conversation.Automation) (VariablesForm, error) {
	if !q.allows(in) {
		return VariablesForm{}, ErrForbidden
	}
	automation, ok, err := q.targetOf(in, target)
	if err != nil || !ok {
		return VariablesForm{}, err
	}
	needs, err := q.d.Service.Needs(in.WorkspaceID, automation)
	if err != nil {
		return VariablesForm{}, err
	}
	link := rootOf(automation, needs)
	if link.Name == "" {
		link.Name = q.nameOf(in.WorkspaceID, automation)
	}
	form := VariablesForm{Automation: &link, Rows: []VariableRow{}}
	if len(needs) == 0 {
		return form, nil
	}
	snapshot, defs, err := q.conversation(ctx, in)
	if err != nil {
		return VariablesForm{}, err
	}
	fields := campaignaudience.FieldsOf(defs)
	plan, err := campaignaudience.FieldPlan(fields, defs)
	if err != nil {
		return VariablesForm{}, fmt.Errorf("%w: lead fields: %w", domain.ErrUnreadable, err)
	}
	subject, err := q.subjectOf(ctx, in.WorkspaceID, snapshot.LeadID, plan)
	if err != nil {
		return VariablesForm{}, err
	}
	for i, value := range plan.Values(subject) {
		form.Fields = append(form.Fields, FieldValue{Field: fields[i], Value: value})
	}
	for _, need := range needs {
		form.Rows = append(form.Rows, VariableRow{
			Need:      need,
			Current:   strings.TrimSpace(snapshot.Values[need.Name]),
			Suggested: campaign.SameKeyBinding(defs, need.Name),
		})
	}
	return form, nil
}

func (q *Queries) SaveConversationValues(ctx context.Context, in SaveValues) error {
	if !q.allows(in.ConversationRef) {
		return ErrForbidden
	}
	assignment, err := q.assign(in.WorkspaceID, append(conversationvars.LiteralBindings(in.Values), in.Bindings...))
	if err != nil {
		return err
	}
	needs, err := q.d.Service.Needs(in.WorkspaceID, in.Automation)
	if err != nil {
		return err
	}
	if unneeded := unneededNames(needs, assignment.Names()); len(unneeded) > 0 {
		return &conversationvars.VariablesError{Cause: conversationvars.ErrNotNeeded, Names: unneeded}
	}
	ref := conversationvars.Ref{WorkspaceID: in.WorkspaceID, EntryID: strings.TrimSpace(in.EntryID), EntryType: in.EntryType}
	resolved, err := q.resolve(ctx, ref, assignment)
	if err != nil || len(resolved) == 0 {
		return err
	}
	return q.d.Store.Save(ctx, ref, resolved, in.UserID)
}

func (q *Queries) ResolveValues(ctx context.Context, ref conversationvars.Ref, bindings []conversationvars.Binding) (map[string]string, error) {
	assignment, err := q.assign(ref.WorkspaceID, bindings)
	if err != nil {
		return nil, err
	}
	return q.resolve(ctx, ref, assignment)
}

func (q *Queries) assign(workspaceID string, bindings []conversationvars.Binding) (conversationvars.Assignment, error) {
	defs, err := q.definitions(workspaceID)
	if err != nil {
		return conversationvars.Assignment{}, err
	}
	return conversationvars.Assign(bindings, defs)
}

func (q *Queries) resolve(ctx context.Context, ref conversationvars.Ref, assignment conversationvars.Assignment) (map[string]string, error) {
	snapshot, err := q.d.Conversations.Snapshot(ctx, strings.TrimSpace(ref.EntryID), ref.EntryType)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	subject, err := q.subjectOf(ctx, ref.WorkspaceID, snapshot.LeadID, assignment.Plan())
	if err != nil {
		return nil, err
	}
	return assignment.Resolve(subject, snapshot.Values)
}

func (q *Queries) LeadFields(workspaceID string) ([]*customfield.Definition, error) {
	return q.definitions(workspaceID)
}

func (q *Queries) allows(in ConversationRef) bool {
	return q.d.Access.CanAccessEntry(in.UserID, in.WorkspaceID, in.EntryID, string(in.EntryType), in.IsAdmin)
}

func (q *Queries) targetOf(in ConversationRef, target *conversation.Automation) (conversation.Automation, bool, error) {
	if target != nil {
		return *target, true, nil
	}
	profile, err := q.d.Profiles.EntryAutomation(in.EntryID, string(in.EntryType))
	if err != nil {
		return conversation.Automation{}, false, fmt.Errorf("%w: automation of %s %s: %w", domain.ErrUnreadable, in.EntryType, in.EntryID, err)
	}
	automation, ok := profile.Configured()
	return automation, ok, nil
}

func (q *Queries) conversation(ctx context.Context, in ConversationRef) (conversationvars.Snapshot, []*customfield.Definition, error) {
	snapshot, err := q.d.Conversations.Snapshot(ctx, in.EntryID, in.EntryType)
	if err != nil {
		return conversationvars.Snapshot{}, nil, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	defs, err := q.definitions(in.WorkspaceID)
	if err != nil {
		return conversationvars.Snapshot{}, nil, err
	}
	return snapshot, defs, nil
}

func (q *Queries) definitions(workspaceID string) ([]*customfield.Definition, error) {
	defs, err := q.d.Definitions.ListByObject(workspaceID, customfield.ObjectLead)
	if err != nil {
		return nil, fmt.Errorf("%w: lead fields of %s: %w", domain.ErrUnreadable, workspaceID, err)
	}
	return defs, nil
}

func (q *Queries) subjectOf(ctx context.Context, workspaceID, leadID string, plan campaign.BindingPlan) (campaign.BindingSubject, error) {
	if leadID == "" || !plan.NeedsLead() {
		return campaign.BindingSubject{}, nil
	}
	subjects, err := q.d.Subjects.Identified(ctx, workspaceID, []string{leadID}, plan)
	if err != nil {
		return campaign.BindingSubject{}, fmt.Errorf("%w: lead %s: %w", domain.ErrUnreadable, leadID, err)
	}
	return subjects[leadID], nil
}

func unneededNames(needs domain.Needs, names []string) []string {
	var out []string
	for _, name := range names {
		if needs.Find(name) == nil {
			out = append(out, name)
		}
	}
	return out
}
