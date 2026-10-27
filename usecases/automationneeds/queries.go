package automationneeds

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"vozko/domain/agent"
	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/conversationvars"
	"vozko/domain/shared"
	"vozko/domain/workflow"
)

var (
	ErrQueriesUnwired = errors.New("automation needs: the queries need the service, the publish guard, the drafts, the records, the values and the access check")
	ErrForbidden      = errors.New("automation needs: no access to this conversation")
	ErrTooMany        = errors.New("automation needs: ask for at most 100 automations at once")
)

const maxAsked = 100

type DraftAgents interface {
	FindByID(agentID string) (*agent.Agent, error)
}

type DraftWorkflows interface {
	FindByID(workflowID string) (*workflow.Workflow, error)
}

type EntryAccess interface {
	CanAccessEntry(userID, workspaceID, entryID, entryType string, isAdmin bool) bool
}

type QueriesDeps struct {
	Service   *Service
	Guard     *PublishGuard
	Agents    DraftAgents
	Workflows DraftWorkflows
	Records   domain.SwitchOffRecords
	Store     conversationvars.Store
	Access    EntryAccess
}

type Queries struct {
	d QueriesDeps
}

func NewQueries(d QueriesDeps) (*Queries, error) {
	if d.Service == nil || d.Guard == nil || d.Agents == nil || d.Workflows == nil || d.Records == nil || d.Store == nil || d.Access == nil {
		return nil, ErrQueriesUnwired
	}
	return &Queries{d: d}, nil
}

type Eligibility struct {
	Automation domain.Link
	Needs      domain.Needs
	Readable   bool
}

func (e Eligibility) Receptive() bool {
	return e.Readable && len(e.Needs) == 0
}

func (q *Queries) Eligibility(workspaceID string, asked []conversation.Automation) ([]Eligibility, error) {
	if len(asked) > maxAsked {
		return nil, ErrTooMany
	}
	out := make([]Eligibility, 0, len(asked))
	for _, a := range asked {
		needs, err := q.d.Service.Needs(workspaceID, a)
		item := Eligibility{Automation: rootOf(a, needs), Needs: needs, Readable: err == nil}
		if err == nil && item.Automation.Name == "" {
			item.Automation.Name = q.nameOf(workspaceID, a)
		}
		out = append(out, item)
	}
	return out, nil
}

func (q *Queries) nameOf(workspaceID string, a conversation.Automation) string {
	src := q.d.Service.Source()
	switch a.Kind {
	case conversation.AutomationAgent:
		if found, err := src.Agent(a.ID); err == nil && found != nil && found.WorkspaceID == workspaceID {
			return found.Name
		}
	case conversation.AutomationWorkflow:
		if found, err := src.Workflow(a.ID); err == nil && found != nil && found.WorkspaceID == workspaceID {
			return found.Name
		}
	}
	return ""
}

func (q *Queries) PublishImpact(ctx context.Context, workspaceID string, a conversation.Automation) ([]domain.SwitchOff, error) {
	switch a.Kind {
	case conversation.AutomationAgent:
		draft, err := q.d.Agents.FindByID(a.ID)
		if err != nil || draft == nil || draft.WorkspaceID != workspaceID {
			return nil, fmt.Errorf("%w: agent %s", domain.ErrNotFound, a.ID)
		}
		return q.d.Guard.Impact(ctx, workspaceID, a, domain.OverlayAgent(q.d.Service.Source(), draft))
	case conversation.AutomationWorkflow:
		draft, err := q.d.Workflows.FindByID(a.ID)
		if err != nil || draft == nil || draft.WorkspaceID != workspaceID {
			return nil, fmt.Errorf("%w: workflow %s", domain.ErrNotFound, a.ID)
		}
		return q.d.Guard.Impact(ctx, workspaceID, a, domain.OverlayWorkflow(q.d.Service.Source(), draft))
	}
	return nil, fmt.Errorf("%w: unknown kind %q", domain.ErrNotFound, a.Kind)
}

type OpenSwitchOff struct {
	Record      domain.SwitchOffRecord
	Needs       domain.Needs
	EligibleNow bool
}

func (q *Queries) OpenSwitchOff(ctx context.Context, workspaceID string, kind domain.SlotKind, slotID string) (*OpenSwitchOff, error) {
	if !kind.Valid() {
		return nil, fmt.Errorf("%w: channel %q", domain.ErrNotFound, kind)
	}
	record, err := q.d.Records.Open(ctx, workspaceID, kind, slotID)
	if err != nil || record == nil {
		return nil, err
	}
	needs, err := q.d.Service.Needs(workspaceID, record.Automation.Automation)
	return &OpenSwitchOff{Record: *record, Needs: needs, EligibleNow: err == nil && len(needs) == 0}, nil
}

type SaveValues struct {
	WorkspaceID string
	UserID      string
	IsAdmin     bool
	EntryID     string
	EntryType   shared.EntryType
	Automation  conversation.Automation
	Values      map[string]string
}

func (q *Queries) SaveConversationValues(ctx context.Context, in SaveValues) error {
	if !q.d.Access.CanAccessEntry(in.UserID, in.WorkspaceID, in.EntryID, string(in.EntryType), in.IsAdmin) {
		return ErrForbidden
	}
	values, err := conversationvars.Normalize(in.Values)
	if err != nil {
		return err
	}
	needs, err := q.d.Service.Needs(in.WorkspaceID, in.Automation)
	if err != nil {
		return err
	}
	for name := range values {
		if needs.Find(name) == nil {
			return fmt.Errorf("%w: %q", conversationvars.ErrNotNeeded, name)
		}
	}
	ref := conversationvars.Ref{WorkspaceID: in.WorkspaceID, EntryID: strings.TrimSpace(in.EntryID), EntryType: in.EntryType}
	return q.d.Store.Save(ctx, ref, values, in.UserID)
}
