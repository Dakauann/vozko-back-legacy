package automationtransfer_usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"vozko/domain/actor"
	"vozko/domain/automationneeds"
	"vozko/domain/automationtransfer"
	"vozko/domain/conversation"
	"vozko/domain/conversationvars"
	"vozko/domain/customfield"
	"vozko/domain/shared"
	"vozko/domain/stage"
)

var ErrVariablesNotRestored = errors.New("automation transfer: the conversation variables could not be put back")

type Needs interface {
	Needs(workspaceID string, a conversation.Automation) (automationneeds.Needs, error)
}

type Values interface {
	ResolveValues(ctx context.Context, ref conversationvars.Ref, bindings []conversationvars.Binding) (map[string]string, error)
	LeadFields(workspaceID string) ([]*customfield.Definition, error)
}

type VariableStore interface {
	Values(ctx context.Context, entryID string, entryType shared.EntryType) (map[string]string, error)
	Save(ctx context.Context, ref conversationvars.Ref, values map[string]string, by string) error
	Forget(ctx context.Context, ref conversationvars.Ref, names []string) error
}

type storedValues struct {
	written  []string
	previous map[string]string
}

func (s *Service) needsOf(workspaceID string, d automationtransfer.Destination) (automationneeds.Needs, error) {
	needs, err := s.d.Needs.Needs(workspaceID, d.Target)
	if err != nil {
		return nil, fmt.Errorf("read the needs of %s %s: %w", d.Target.Kind, d.Target.ID, err)
	}
	return needs, nil
}

func (s *Service) informs(workspaceID string, d automationtransfer.Destination) ([]automationtransfer.VariableSource, error) {
	asked := d.AIVariables()
	if len(asked) == 0 {
		return nil, nil
	}
	needs, err := s.needsOf(workspaceID, d)
	if err != nil {
		return nil, err
	}
	var out []automationtransfer.VariableSource
	for _, v := range asked {
		need := needs.Find(v.Name)
		if need == nil {
			continue
		}
		if v.Description == "" {
			v.Description = need.Description
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) checkMapping(workspaceID string, d automationtransfer.Destination) error {
	needs, err := s.needsOf(workspaceID, d)
	if err != nil {
		return err
	}
	if len(needs) == 0 && len(d.Variables) == 0 {
		return nil
	}
	defs, err := s.d.Values.LeadFields(workspaceID)
	if err != nil {
		return fmt.Errorf("read the lead fields of %s: %w", workspaceID, err)
	}
	if err := d.CheckMapping(needs, defs); err != nil {
		return Refused(err)
	}
	return nil
}

func (s *Service) pendingValues(ctx context.Context, in Request, chosen automationtransfer.Destination) (map[string]string, []automationtransfer.Filled, error) {
	if len(chosen.Variables) == 0 {
		return nil, nil, nil
	}
	needs, err := s.needsOf(in.WorkspaceID, chosen)
	if err != nil {
		return nil, nil, err
	}
	bindings, filled, err := chosen.Bindings(needs, in.Provided)
	if err != nil || len(bindings) == 0 {
		return nil, filled, err
	}
	values, err := s.d.Values.ResolveValues(ctx, refOf(in), bindings)
	if err != nil {
		return nil, nil, err
	}
	return values, filled, nil
}

func (s *Service) storeValues(ctx context.Context, in Request, pending map[string]string) (storedValues, error) {
	if len(pending) == 0 {
		return storedValues{}, nil
	}
	previous, err := s.d.Variables.Values(ctx, in.EntryID, in.EntryType)
	if err != nil {
		return storedValues{}, fmt.Errorf("read the variables of %s %s: %w", in.EntryType, in.EntryID, err)
	}
	if err := s.d.Variables.Save(ctx, refOf(in), pending, actor.FormatAI(in.AgentID)); err != nil {
		return storedValues{}, fmt.Errorf("store the variables of %s %s: %w", in.EntryType, in.EntryID, err)
	}
	written := make([]string, 0, len(pending))
	for name := range pending {
		written = append(written, name)
	}
	return storedValues{written: written, previous: previous}, nil
}

func (s *Service) restoreValues(ctx context.Context, in Request, stored storedValues) error {
	if len(stored.written) == 0 {
		return nil
	}
	back := map[string]string{}
	var forget []string
	for _, name := range stored.written {
		if value, had := stored.previous[name]; had {
			back[name] = value
		} else {
			forget = append(forget, name)
		}
	}
	if len(back) > 0 {
		if err := s.d.Variables.Save(ctx, refOf(in), back, actor.SystemID); err != nil {
			return err
		}
	}
	return s.d.Variables.Forget(ctx, refOf(in), forget)
}

func (s *Service) undo(ctx context.Context, in Request, move stage.Move, moved bool, stored storedValues, cause error) error {
	undone := context.WithoutCancel(ctx)
	if err := s.restoreValues(undone, in, stored); err != nil {
		slog.Error("automation_transfer.variables_not_restored", slog.String("entry_id", in.EntryID), slog.String("error", err.Error()))
		return fmt.Errorf("%w: %w; %w", ErrVariablesNotRestored, err, cause)
	}
	if moved {
		if err := s.restoreStage(in, move); err != nil {
			slog.Error("automation_transfer.stage_not_restored", slog.String("entry_id", in.EntryID), slog.String("error", err.Error()))
			return fmt.Errorf("%w: %w; %w", ErrStageNotRestored, err, cause)
		}
	}
	return cause
}

func refOf(in Request) conversationvars.Ref {
	return conversationvars.Ref{WorkspaceID: in.WorkspaceID, EntryID: in.EntryID, EntryType: in.EntryType}
}
