package automationneeds

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/conversationvars"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	"vozko/domain/sheetfit"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

type ConversationValues interface {
	Values(ctx context.Context, entryID string, entryType shared.EntryType) (map[string]string, error)
}

type DeclarationCatalog interface {
	Catalog(ctx context.Context, workspaceID string) (variabledecl.Catalog, error)
	Registry() *variabledecl.Registry
}

type SheetFiles interface {
	Resolve(ctx context.Context, workspaceID string, t variabledecl.Type, uses []sheetfit.Use, value string) (workflow.SheetFile, error)
	Candidates(ctx context.Context, workspaceID string, t variabledecl.Type, uses []sheetfit.Use) ([]sheetfit.Candidate, error)
	Attention(ctx context.Context, workspaceID string, file workflow.SheetFile) (bool, error)
}

type ServiceDeps struct {
	Source       domain.Source
	Values       ConversationValues
	Declarations DeclarationCatalog
	Sheets       SheetFiles
}

type Service struct {
	source       domain.Source
	values       ConversationValues
	declarations DeclarationCatalog
	sheets       SheetFiles
}

func NewService(d ServiceDeps) (*Service, error) {
	if err := deps.Require("automation needs", map[string]any{
		"published automations": d.Source,
		"conversation values":   d.Values,
		"declarations":          d.Declarations,
		"sheet files":           d.Sheets,
	}); err != nil {
		return nil, err
	}
	return &Service{source: d.Source, values: d.Values, declarations: d.Declarations, sheets: d.Sheets}, nil
}

func (s *Service) Source() domain.Source { return s.source }

func (s *Service) Label(ctx context.Context, workspaceID string, needs domain.Needs) domain.Needs {
	return s.Catalog(ctx, workspaceID).Apply(needs)
}

func (s *Service) Catalog(ctx context.Context, workspaceID string) Labels {
	registry := s.declarations.Registry()
	catalog, err := s.declarations.Catalog(ctx, workspaceID)
	if err != nil {
		slog.Warn("automation.variables.labels_unreadable", slog.String("workspace_id", workspaceID), slog.String("outcome", "names_humanized"), slog.String("error", err.Error()))
		return Labels{registry: registry}
	}
	return Labels{catalog: catalog, registry: registry}
}

type Labels struct {
	catalog  variabledecl.Catalog
	registry *variabledecl.Registry
}

func (l Labels) Apply(needs domain.Needs) domain.Needs {
	out := needs.Labeled(l.catalog)
	for i := range out {
		if l.registry != nil {
			out[i].Shape = l.registry.ShapeOf(out[i].Type)
			out[i].Example = l.registry.ExampleOf(variabledecl.Declaration{Type: out[i].Type, Example: l.exampleOf(out[i].Name)})
		}
	}
	return out
}

func (l Labels) exampleOf(name string) string {
	if d, ok := l.catalog.Find(name); ok {
		return d.Example
	}
	return ""
}

func (l Labels) Refusal(refusal *domain.Refusal) *domain.Refusal {
	labeled := *refusal
	labeled.Needs = l.Apply(refusal.Needs)
	return &labeled
}

func (s *Service) Rejected(ctx context.Context, workspaceID string, values map[string]string) ([]string, error) {
	catalog, err := s.declarations.Catalog(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	rejected, err := s.declarations.Registry().Rejected(ctx, workspaceID, catalog, values)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	return rejected, nil
}

func (l Labels) Variables(err error) error {
	var named *conversationvars.VariablesError
	if !errors.As(err, &named) {
		return err
	}
	labeled := *named
	labeled.Labels = make(map[string]string, len(named.Names))
	for _, name := range named.Names {
		labeled.Labels[name] = l.catalog.LabelOf(name)
	}
	return &labeled
}

func (s *Service) labeledRefusal(workspaceID string, refusal *domain.Refusal) error {
	return s.Catalog(context.Background(), workspaceID).Refusal(refusal)
}

func (s *Service) Needs(workspaceID string, a conversation.Automation) (domain.Needs, error) {
	return domain.Of(s.source, workspaceID, a)
}

func (s *Service) AdmitReceptive(workspaceID string, kind domain.SlotKind, config conversation.ChannelAutomation) error {
	if !kind.Valid() {
		return fmt.Errorf("%w: channel %q", domain.ErrNotFound, kind)
	}
	for _, running := range domain.RunningOf(config) {
		if err := s.ChannelRefusal(workspaceID, running, kind.EntryType()); err != nil {
			return err
		}
		vars, _, err := s.variables(context.Background(), workspaceID, running)
		if err != nil {
			return err
		}
		needs := vars.Required
		if len(needs) > 0 {
			return s.labeledRefusal(workspaceID, &domain.Refusal{Cause: domain.ErrReceptiveUnmet, Automation: rootOf(running, needs), Needs: needs})
		}
	}
	return nil
}

func (s *Service) ChannelRefusal(workspaceID string, target conversation.Automation, channel shared.EntryType) error {
	if !workflow.IsChannel(string(channel)) {
		return nil
	}
	walk, err := domain.ChannelWalkOf(s.source, workspaceID, target, []shared.EntryType{channel})
	if err != nil {
		return err
	}
	return domain.ChannelRefusal(target, walk)
}

func (s *Service) ConversationMissing(ctx context.Context, workspaceID, entryID string, entryType shared.EntryType, target conversation.Automation) error {
	if err := s.ChannelRefusal(workspaceID, target, entryType); err != nil {
		return err
	}
	_, err := s.valuesWith(ctx, workspaceID, entryID, entryType, target, nil)
	return err
}

func (s *Service) ConversationMissingWith(ctx context.Context, workspaceID, entryID string, entryType shared.EntryType, target conversation.Automation, pending map[string]string) error {
	if err := s.ChannelRefusal(workspaceID, target, entryType); err != nil {
		return err
	}
	_, err := s.valuesWith(ctx, workspaceID, entryID, entryType, target, pending)
	return err
}

func (s *Service) ValuesFor(ctx context.Context, workspaceID, entryID string, entryType shared.EntryType, target conversation.Automation) (map[string]string, error) {
	resolved, err := s.valuesWith(ctx, workspaceID, entryID, entryType, target, nil)
	return resolved.Values, err
}

func (s *Service) Resolve(ctx context.Context, workspaceID, entryID string, entryType shared.EntryType, target conversation.Automation) (Resolved, error) {
	return s.valuesWith(ctx, workspaceID, entryID, entryType, target, nil)
}

func (s *Service) valuesWith(ctx context.Context, workspaceID, entryID string, entryType shared.EntryType, target conversation.Automation, pending map[string]string) (Resolved, error) {
	vars, catalog, err := s.variables(ctx, workspaceID, target)
	if err != nil {
		return Resolved{}, err
	}
	needs := vars.Required
	var values map[string]string
	if len(needs) == 0 {
		values = withPending(s.optionalValues(ctx, workspaceID, entryID, entryType), pending)
	} else {
		read, err := s.values.Values(ctx, entryID, entryType)
		if err != nil {
			return Resolved{}, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
		}
		values = withPending(read, pending)
		if missing := needs.Missing(values); len(missing) > 0 {
			return Resolved{}, s.labeledRefusal(workspaceID, &domain.Refusal{Cause: domain.ErrConversationMissing, Automation: rootOf(target, needs), Needs: missing, Values: knownValues(needs, values)})
		}
	}
	filled := vars.Fill(values)
	sheets, err := s.resolveSheets(ctx, workspaceID, target, vars, catalog, filled)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{Values: filled, Sheets: sheets}, nil
}

func (s *Service) optionalValues(ctx context.Context, workspaceID, entryID string, entryType shared.EntryType) map[string]string {
	values, err := s.values.Values(ctx, entryID, entryType)
	if err != nil {
		slog.Warn("automation.variables.unreadable", slog.String("workspace_id", workspaceID), slog.String("entry_id", entryID), slog.String("entry_type", string(entryType)), slog.String("outcome", "no_values"), slog.String("error", err.Error()))
		return nil
	}
	return values
}

func rootOf(target conversation.Automation, needs domain.Needs) domain.Link {
	for _, need := range needs {
		if len(need.Via) > 0 && need.Via[0].Automation == target {
			return need.Via[0]
		}
	}
	return domain.Link{Automation: target}
}

func knownValues(needs domain.Needs, values map[string]string) map[string]string {
	out := map[string]string{}
	for _, need := range needs {
		if v, ok := values[need.Name]; ok && v != "" {
			out[need.Name] = v
		}
	}
	return out
}

func withPending(values, pending map[string]string) map[string]string {
	if len(pending) == 0 {
		return values
	}
	out := make(map[string]string, len(values)+len(pending))
	for name, value := range values {
		out[name] = value
	}
	for name, value := range pending {
		out[name] = value
	}
	return out
}
