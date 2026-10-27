package automationneeds

import (
	"context"
	"fmt"
	"log/slog"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
)

type ConversationValues interface {
	Values(ctx context.Context, entryID string, entryType shared.EntryType) (map[string]string, error)
}

type ServiceDeps struct {
	Source domain.Source
	Values ConversationValues
}

type Service struct {
	source domain.Source
	values ConversationValues
}

func NewService(d ServiceDeps) (*Service, error) {
	if err := deps.Require("automation needs", map[string]any{
		"published automations": d.Source,
		"conversation values":   d.Values,
	}); err != nil {
		return nil, err
	}
	return &Service{source: d.Source, values: d.Values}, nil
}

func (s *Service) Source() domain.Source { return s.source }

func (s *Service) Needs(workspaceID string, a conversation.Automation) (domain.Needs, error) {
	return domain.Of(s.source, workspaceID, a)
}

func (s *Service) AdmitReceptive(workspaceID string, config conversation.ChannelAutomation) error {
	for _, running := range domain.RunningOf(config) {
		needs, err := s.Needs(workspaceID, running)
		if err != nil {
			return err
		}
		if len(needs) > 0 {
			return &domain.Refusal{Cause: domain.ErrReceptiveUnmet, Automation: rootOf(running, needs), Needs: needs}
		}
	}
	return nil
}

func (s *Service) ConversationMissing(ctx context.Context, workspaceID, entryID string, entryType shared.EntryType, target conversation.Automation) error {
	_, err := s.ValuesFor(ctx, workspaceID, entryID, entryType, target)
	return err
}

func (s *Service) ValuesFor(ctx context.Context, workspaceID, entryID string, entryType shared.EntryType, target conversation.Automation) (map[string]string, error) {
	needs, err := s.Needs(workspaceID, target)
	if err != nil {
		return nil, err
	}
	if len(needs) == 0 {
		return s.optionalValues(ctx, workspaceID, entryID, entryType), nil
	}
	values, err := s.values.Values(ctx, entryID, entryType)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	if missing := needs.Missing(values); len(missing) > 0 {
		return nil, &domain.Refusal{Cause: domain.ErrConversationMissing, Automation: rootOf(target, needs), Needs: missing, Values: knownValues(needs, values)}
	}
	return values, nil
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
