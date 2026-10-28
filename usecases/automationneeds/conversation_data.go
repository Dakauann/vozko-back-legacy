package automationneeds

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "vozko/domain/automationneeds"
	ce "vozko/domain/conversation_event"
	"vozko/domain/conversationvars"
	"vozko/domain/shared"
	"vozko/domain/variabledecl"
)

type Origin string

const (
	OriginSystem   Origin = "system"
	OriginCampaign Origin = "campaign"
	OriginStored   Origin = "stored"
	OriginNone     Origin = "none"
)

type StoredDetails interface {
	Detailed(ctx context.Context, entryID string, entryType shared.EntryType) ([]conversationvars.Stored, error)
}

type Timeline interface {
	Create(event *ce.ConversationEvent) error
}

type DataRow struct {
	Name     string
	Label    string
	LabelKey string
	Declared bool
	Type     variabledecl.Type
	Shape    variabledecl.Shape
	Value    string
	Origin   Origin
	SetBy    string
	SetAt    time.Time
	Fits     bool
}

func (q *Queries) ConversationData(ctx context.Context, in ConversationRef) ([]DataRow, error) {
	if !q.allows(in) {
		return nil, ErrForbidden
	}
	return q.conversationData(ctx, in)
}

func (q *Queries) conversationData(ctx context.Context, in ConversationRef) ([]DataRow, error) {
	snapshot, err := q.d.Conversations.Snapshot(ctx, in.EntryID, in.EntryType)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	stored, err := q.d.StoredDetails.Detailed(ctx, in.EntryID, in.EntryType)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	catalog, err := q.d.Service.declarations.Catalog(ctx, in.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	registry := q.d.Service.declarations.Registry()
	storedBy := make(map[string]conversationvars.Stored, len(stored))
	for _, s := range stored {
		storedBy[s.Name] = s
	}
	row := func(name string) DataRow {
		out := DataRow{Name: name, Label: catalog.LabelOf(name), Type: variabledecl.Type{Kind: variabledecl.KindText}, Value: strings.TrimSpace(snapshot.Values[name]), Origin: OriginNone, Fits: true}
		if d, ok := catalog.Find(name); ok {
			out.Declared, out.Type = true, d.Type
		}
		out.Shape = registry.ShapeOf(out.Type)
		switch s, isStored := storedBy[name]; {
		case variabledecl.IsSystemKey(name):
			out.Origin, out.LabelKey = OriginSystem, variabledecl.SystemLabelKey(name)
		case isStored:
			out.Origin, out.SetBy, out.SetAt = OriginStored, s.SetBy, s.SetAt
		case out.Value != "":
			out.Origin = OriginCampaign
		}
		if out.Value != "" && out.Origin != OriginSystem {
			out.Fits = registry.Accepts(ctx, variabledecl.Check{WorkspaceID: in.WorkspaceID, Name: name, Type: out.Type, Value: out.Value}) == nil
		}
		return out
	}
	var rows []DataRow
	seen := map[string]bool{}
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			rows = append(rows, row(name))
		}
	}
	for _, name := range variabledecl.SystemKeys() {
		add(name)
	}
	for _, d := range catalog.All() {
		add(d.Name)
	}
	for _, s := range stored {
		add(s.Name)
	}
	return rows, nil
}

func (q *Queries) SetConversationData(ctx context.Context, in ConversationRef, name, value string) (DataRow, error) {
	if !q.allows(in) {
		return DataRow{}, ErrForbidden
	}
	name, value = strings.TrimSpace(name), strings.TrimSpace(value)
	if err := variabledecl.CheckName(name); err != nil {
		return DataRow{}, err
	}
	rows, err := q.conversationData(ctx, in)
	if err != nil {
		return DataRow{}, err
	}
	current, editable := findRow(rows, name)
	if !editable || (!current.Declared && current.Origin != OriginStored) {
		return DataRow{}, fmt.Errorf("%w: %s", conversationvars.ErrNotEditable, name)
	}
	if value == "" {
		return DataRow{}, &conversationvars.VariablesError{Cause: conversationvars.ErrValueEmpty, Names: []string{name}, Labels: map[string]string{name: current.Label}}
	}
	rejected, err := q.d.Service.Rejected(ctx, in.WorkspaceID, map[string]string{name: value})
	if err != nil {
		return DataRow{}, err
	}
	if len(rejected) > 0 {
		return DataRow{}, &conversationvars.VariablesError{Cause: conversationvars.ErrTypeInvalid, Names: rejected, Labels: map[string]string{name: current.Label}}
	}
	ref := conversationvars.Ref{WorkspaceID: in.WorkspaceID, EntryID: strings.TrimSpace(in.EntryID), EntryType: in.EntryType}
	if err := q.d.Store.Save(ctx, ref, map[string]string{name: value}, in.UserID); err != nil {
		return DataRow{}, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	q.audit(in, current)
	current.Value, current.Origin, current.SetBy, current.SetAt, current.Fits = value, OriginStored, in.UserID, time.Now().UTC(), true
	return current, nil
}

func (q *Queries) audit(in ConversationRef, row DataRow) {
	event := ce.New(in.WorkspaceID, in.EntryID, string(in.EntryType), ce.EventConversationVariableChanged).
		WithActorHuman(in.UserID).
		WithChannel(in.EntryType.EventChannel()).
		WithDetails(map[string]string{"name": row.Name, "label": row.Label}).
		Build()
	event.ID, event.CreatedAt = uuid.NewString(), time.Now().UTC()
	if err := q.d.Timeline.Create(event); err != nil {
		slog.Error("conversation_variable.audit_unwritten", slog.String("workspace_id", in.WorkspaceID), slog.String("entry_id", in.EntryID),
			slog.String("name", row.Name), slog.String("error", err.Error()))
	}
}

func findRow(rows []DataRow, name string) (DataRow, bool) {
	for _, row := range rows {
		if row.Name == name {
			return row, true
		}
	}
	return DataRow{}, false
}
