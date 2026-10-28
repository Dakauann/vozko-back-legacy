package variabledecl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "vozko/domain/variabledecl"
)

var errUnwired = errors.New("variable declarations: the service needs the repository and the type registry")

type Service struct {
	repo     domain.Repository
	registry *domain.Registry
	now      func() time.Time
}

func NewService(repo domain.Repository, registry *domain.Registry) (*Service, error) {
	if repo == nil || registry == nil {
		return nil, errUnwired
	}
	return &Service{repo: repo, registry: registry, now: time.Now}, nil
}

func (s *Service) Registry() *domain.Registry { return s.registry }

func (s *Service) List(ctx context.Context, workspaceID string) ([]domain.Declaration, error) {
	rows, err := s.repo.List(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	return domain.NewCatalog(rows).All(), nil
}

func (s *Service) Described(ctx context.Context, workspaceID string) ([]domain.Declaration, error) {
	rows, err := s.List(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.registry.Described(rows), nil
}

func (s *Service) Catalog(ctx context.Context, workspaceID string) (domain.Catalog, error) {
	rows, err := s.List(ctx, workspaceID)
	if err != nil {
		return domain.Catalog{}, err
	}
	return domain.NewCatalog(rows), nil
}

func (s *Service) Save(ctx context.Context, workspaceID, by string, d domain.Declaration) (domain.Declaration, error) {
	prepared, err := s.registry.Prepare(ctx, workspaceID, d)
	if err != nil {
		return domain.Declaration{}, err
	}
	checked, err := s.registry.CheckDeclaration(prepared)
	if err != nil {
		return domain.Declaration{}, err
	}
	checked.UpdatedBy = by
	checked.UpdatedAt = s.now().UTC()
	if err := s.repo.Save(ctx, workspaceID, checked); err != nil {
		return domain.Declaration{}, fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	return checked, nil
}

func (s *Service) Delete(ctx context.Context, workspaceID, name string) error {
	removed, err := s.repo.Delete(ctx, workspaceID, strings.TrimSpace(name))
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrUnreadable, err)
	}
	if !removed {
		return fmt.Errorf("%w: %s", domain.ErrNotFound, name)
	}
	return nil
}

func (s *Service) Rejected(ctx context.Context, workspaceID string, values map[string]string) ([]string, error) {
	catalog, err := s.Catalog(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.registry.Rejected(ctx, workspaceID, catalog, values)
}
