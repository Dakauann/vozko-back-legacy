package campaignaudience_usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"vozko/domain/campaign"
	"vozko/domain/campaignaudience"
	"vozko/domain/sheetfit"
	"vozko/domain/variabledecl"
	"vozko/domain/workflow"
)

type VariableValues interface {
	Catalog(ctx context.Context, workspaceID string) (variabledecl.Catalog, error)
	Registry() *variabledecl.Registry
}

func (s *Service) prepare(ctx context.Context, ch Channel, a Actor, params Params) (Prepared, func(key, value string) bool, error) {
	prepared, err := ch.Prepare(ctx, a, params)
	if err != nil {
		return Prepared{}, nil, err
	}
	accept, err := s.typed(ctx, a.WorkspaceID, &prepared)
	if err != nil {
		return Prepared{}, nil, err
	}
	return prepared, accept, nil
}

func (s *Service) typed(ctx context.Context, workspaceID string, prepared *Prepared) (func(key, value string) bool, error) {
	if len(prepared.Needs) == 0 && len(prepared.Optional) == 0 {
		return nil, nil
	}
	catalog, err := s.deps.Values.Catalog(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", campaignaudience.ErrUnavailable, err)
	}
	registry := s.deps.Values.Registry()
	prepared.Needs = prepared.Needs.Typed(catalog, registry)
	prepared.Optional = prepared.Optional.Typed(catalog, registry)
	return func(key, value string) bool {
		typ := variabledecl.Type{Kind: variabledecl.KindText}
		if d, ok := catalog.Find(key); ok {
			typ = d.Type
		}
		return registry.Accepts(ctx, variabledecl.Check{WorkspaceID: workspaceID, Name: key, Type: typ, Value: value}) == nil
	}, nil
}

type SheetFiles interface {
	Resolve(ctx context.Context, workspaceID string, t variabledecl.Type, uses []sheetfit.Use, value string) (workflow.SheetFile, error)
}

func (s *Service) fitSheets(ctx context.Context, workspaceID string, bound []campaignaudience.Bound, prepared Prepared) ([]campaignaudience.Bound, error) {
	needs := prepared.Needs.Union(prepared.Optional)
	out := append([]campaignaudience.Bound(nil), bound...)
	for i, b := range out {
		value := strings.TrimSpace(b.Value)
		if !b.Sheet || b.Source != campaign.BindLiteral || value == "" {
			continue
		}
		need := needs.Find(b.Key)
		if need == nil {
			return nil, fmt.Errorf("%w: the Planilha %s has no need", campaignaudience.ErrUnavailable, b.Key)
		}
		_, err := s.deps.SheetFiles.Resolve(ctx, workspaceID, need.Type, need.SheetUses, value)
		var misfit *sheetfit.Misfit
		switch {
		case errors.As(err, &misfit):
			out[i].SheetUnfit = misfit.Code
		case err != nil:
			return nil, fmt.Errorf("%w: %w", campaignaudience.ErrUnavailable, err)
		}
	}
	return out, nil
}
