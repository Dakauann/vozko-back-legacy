package googlesheet

import (
	"context"
	"fmt"
	"strings"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
)

type OriginNames struct {
	Agents    NameLookup
	Workflows NameLookup
	People    NameLookup
}

func (n OriginNames) complete() bool {
	return n.Agents != nil && n.Workflows != nil && n.People != nil
}

type NamedRef struct {
	ID   string
	Name string
}

type OriginView struct {
	Kind         googlesheet.OriginKind
	Conversation *shared.EntryRef
	Agent        *NamedRef
	Workflow     *NamedRef
	RunID        string
	Person       *NamedRef
}

type RowView struct {
	Row    *googlesheet.Row
	Origin OriginView
}

type namedLookup struct {
	label  string
	lookup NameLookup
	seen   map[string]string
}

func newNamedLookup(label string, lookup NameLookup) *namedLookup {
	return &namedLookup{label: label, lookup: lookup, seen: map[string]string{}}
}

func (l *namedLookup) ref(ctx context.Context, workspaceID, id string) (*NamedRef, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	name, seen := l.seen[id]
	if !seen {
		found, err := l.lookup(ctx, workspaceID, id)
		if err != nil {
			return nil, fmt.Errorf("googlesheet: %s name of %s: %w", l.label, id, err)
		}
		name = strings.TrimSpace(found)
		l.seen[id] = name
	}
	if name == "" {
		return nil, nil
	}
	return &NamedRef{ID: id, Name: name}, nil
}

type originDescriber struct {
	workspaceID string
	agents      *namedLookup
	workflows   *namedLookup
	people      *namedLookup
}

func (n OriginNames) describer(workspaceID string) *originDescriber {
	return &originDescriber{
		workspaceID: workspaceID,
		agents:      newNamedLookup("agent", n.Agents),
		workflows:   newNamedLookup("workflow", n.Workflows),
		people:      newNamedLookup("person", n.People),
	}
}

func (d *originDescriber) describe(ctx context.Context, origin googlesheet.Origin) (OriginView, error) {
	view := OriginView{Kind: origin.Kind}
	if entry, ok := origin.Conversation(); ok {
		view.Conversation = &entry
	}
	var err error
	if view.Agent, err = d.agents.ref(ctx, d.workspaceID, origin.AgentID); err != nil {
		return OriginView{}, err
	}
	if view.Workflow, err = d.workflows.ref(ctx, d.workspaceID, origin.WorkflowID); err != nil {
		return OriginView{}, err
	}
	if view.Workflow != nil {
		view.RunID = strings.TrimSpace(origin.RunID)
	}
	if view.Person, err = d.people.ref(ctx, d.workspaceID, origin.ActorID); err != nil {
		return OriginView{}, err
	}
	return view, nil
}

func (n OriginNames) views(ctx context.Context, workspaceID string, rows []*googlesheet.Row) ([]RowView, error) {
	describer := n.describer(workspaceID)
	out := make([]RowView, len(rows))
	for i, row := range rows {
		origin, err := describer.describe(ctx, row.Origin)
		if err != nil {
			return nil, err
		}
		out[i] = RowView{Row: row, Origin: origin}
	}
	return out, nil
}
