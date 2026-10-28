package automationneeds

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/sheetfit"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

type Resolved struct {
	Values map[string]string
	Sheets map[string]workflow.SheetFile
}

func (s *Service) variables(ctx context.Context, workspaceID string, target conversation.Automation) (domain.Variables, variabledecl.Catalog, error) {
	vars, err := domain.VariablesOf(s.source, workspaceID, target)
	if err != nil {
		return domain.Variables{}, variabledecl.Catalog{}, err
	}
	if len(sheetNeedsOf(vars)) == 0 {
		return vars, variabledecl.Catalog{}, nil
	}
	catalog, err := s.declarations.Catalog(ctx, workspaceID)
	if err != nil {
		return domain.Variables{}, variabledecl.Catalog{}, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	return vars.WithDeclaredDefaults(catalog), catalog, nil
}

func (s *Service) resolveSheets(ctx context.Context, workspaceID string, target conversation.Automation, vars domain.Variables, catalog variabledecl.Catalog, values map[string]string) (map[string]workflow.SheetFile, error) {
	needs := sheetNeedsOf(vars)
	if len(needs) == 0 {
		return nil, nil
	}
	files := make(map[string]workflow.SheetFile, len(needs))
	var unfit domain.Needs
	for _, need := range needs {
		d, declared := catalog.Find(need.Name)
		value := strings.TrimSpace(values[need.Name])
		if !declared || d.Type.Kind != variabledecl.KindSheet || value == "" {
			unfit = append(unfit, need)
			continue
		}
		file, err := s.sheets.Resolve(ctx, workspaceID, d.Type, need.SheetUses, value)
		var misfit *sheetfit.Misfit
		switch {
		case errors.As(err, &misfit):
			unfit = append(unfit, need)
		case err != nil:
			return nil, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
		default:
			files[need.Name] = file
		}
	}
	if len(unfit) > 0 {
		return nil, s.labeledRefusal(workspaceID, &domain.Refusal{Cause: domain.ErrSheetsUnmet, Automation: rootOf(target, unfit), Needs: unfit})
	}
	return files, nil
}

func sheetNeedsOf(vars domain.Variables) domain.Needs {
	return append(vars.Required.SheetNeeds(), vars.Optional.SheetNeeds()...)
}

func (s *Service) AgentSheetFiles(ctx context.Context, workspaceID, entryID string, entryType shared.EntryType, agentID string) (map[string]workflow.SheetFile, error) {
	resolved, err := s.valuesWith(ctx, workspaceID, entryID, entryType, conversation.Automation{Kind: conversation.AutomationAgent, ID: agentID}, nil)
	return resolved.Sheets, err
}

var ErrNotASheet = errors.New("automation needs: the variable is not declared as a Planilha")

type SheetCandidates struct {
	Variable   variabledecl.Declaration
	Exact      bool
	Candidates []sheetfit.Candidate
}

func (s *Service) SheetCandidates(ctx context.Context, workspaceID string, target *conversation.Automation, name string) (SheetCandidates, error) {
	catalog, err := s.declarations.Catalog(ctx, workspaceID)
	if err != nil {
		return SheetCandidates{}, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	d, declared := catalog.Find(strings.TrimSpace(name))
	if !declared || d.Type.Kind != variabledecl.KindSheet {
		return SheetCandidates{}, fmt.Errorf("%w: %s", ErrNotASheet, name)
	}
	out := SheetCandidates{Variable: d}
	var uses []sheetfit.Use
	if target != nil {
		vars, _, err := s.variables(ctx, workspaceID, *target)
		if err != nil {
			return SheetCandidates{}, err
		}
		for _, need := range sheetNeedsOf(vars) {
			if need.Name == d.Name {
				uses, out.Exact = need.SheetUses, true
			}
		}
	}
	out.Candidates, err = s.sheets.Candidates(ctx, workspaceID, d.Type, uses)
	if err != nil {
		var misfit *sheetfit.Misfit
		if errors.As(err, &misfit) {
			return SheetCandidates{}, err
		}
		return SheetCandidates{}, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	return out, nil
}

func (q *Queries) SheetCandidates(ctx context.Context, workspaceID string, target *conversation.Automation, name string) (SheetCandidates, error) {
	return q.d.Service.SheetCandidates(ctx, workspaceID, target, name)
}

type SheetLaunch struct {
	Checked   bool
	Unfit     bool
	Attention bool
}

func (s *Service) CampaignSheets(ctx context.Context, workspaceID string, targets []conversation.Automation, values map[string]string) (SheetLaunch, error) {
	var out SheetLaunch
	for _, target := range targets {
		vars, catalog, err := s.variables(ctx, workspaceID, target)
		if err != nil {
			return SheetLaunch{}, err
		}
		for _, need := range sheetNeedsOf(vars) {
			d, declared := catalog.Find(need.Name)
			value := strings.TrimSpace(values[need.Name])
			if value == "" {
				value = strings.TrimSpace(need.Default)
			}
			if value == "" {
				continue
			}
			out.Checked = true
			if !declared || d.Type.Kind != variabledecl.KindSheet {
				out.Unfit = true
				continue
			}
			file, err := s.sheets.Resolve(ctx, workspaceID, d.Type, need.SheetUses, value)
			var misfit *sheetfit.Misfit
			switch {
			case errors.As(err, &misfit):
				out.Unfit = true
				continue
			case err != nil:
				return SheetLaunch{}, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
			}
			attention, err := s.sheets.Attention(ctx, workspaceID, file)
			if err != nil {
				return SheetLaunch{}, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
			}
			out.Attention = out.Attention || attention
		}
	}
	return out, nil
}
