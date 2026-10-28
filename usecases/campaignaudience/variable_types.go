package campaignaudience_usecase

import (
	"context"
	"fmt"

	"vozko/domain/campaignaudience"
	"vozko/domain/variabledecl"
)

type VariableValues interface {
	Catalog(ctx context.Context, workspaceID string) (variabledecl.Catalog, error)
	Registry() *variabledecl.Registry
}

func (s *Service) typed(ctx context.Context, workspaceID string, prepared *Prepared) (func(key, value string) bool, error) {
	if len(prepared.Needs) == 0 && len(prepared.Optional) == 0 {
		return nil, nil
	}
	catalog, err := s.deps.Values.Catalog(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", campaignaudience.ErrUnavailable, err)
	}
	prepared.Needs = prepared.Needs.Labeled(catalog)
	prepared.Optional = prepared.Optional.Labeled(catalog)
	registry := s.deps.Values.Registry()
	return func(key, value string) bool {
		typ := variabledecl.Type{Kind: variabledecl.KindText}
		if d, ok := catalog.Find(key); ok {
			typ = d.Type
		}
		return registry.Accepts(ctx, variabledecl.Check{WorkspaceID: workspaceID, Name: key, Type: typ, Value: value}) == nil
	}, nil
}
