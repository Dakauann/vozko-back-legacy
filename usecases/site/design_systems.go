package site_usecase

import (
	"context"

	"vozko/domain/shared"
	"vozko/domain/site"
)

func (s *Service) CreateDesignSystem(ctx context.Context, workspaceID string, system site.DesignSystem, isDefault bool) (*DesignSystemRecord, error) {
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	if err := system.Validate(); err != nil {
		return nil, err
	}
	now := s.now()
	d := &DesignSystemRecord{ID: s.newID(), WorkspaceID: workspaceID, System: system, IsDefault: isDefault, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateDesignSystem(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Service) ListDesignSystems(ctx context.Context, workspaceID string) ([]DesignSystemRecord, error) {
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	return s.store.ListDesignSystems(ctx, workspaceID)
}

func (s *Service) GetDesignSystem(ctx context.Context, workspaceID, id string) (*DesignSystemRecord, error) {
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	return s.store.GetDesignSystem(ctx, workspaceID, id)
}

func (s *Service) UpdateDesignSystem(ctx context.Context, workspaceID, id string, expected int64, system site.DesignSystem, isDefault bool) (*DesignSystemRecord, error) {
	if err := shared.RequireVersion(expected); err != nil {
		return nil, err
	}
	if err := system.Validate(); err != nil {
		return nil, err
	}
	current, err := s.GetDesignSystem(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}
	if err := shared.ExpectVersion(current.Version, &expected); err != nil {
		return nil, err
	}
	current.System, current.IsDefault, current.UpdatedAt = system, isDefault, s.now()
	if err := s.store.UpdateDesignSystem(ctx, current, expected); err != nil {
		return nil, err
	}
	return current, nil
}

func (s *Service) DeleteDesignSystem(ctx context.Context, workspaceID, id string) error {
	if workspaceID == "" {
		return ErrWorkspaceRequired
	}
	return s.store.DeleteDesignSystem(ctx, workspaceID, id)
}
