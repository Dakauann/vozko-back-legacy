package site_usecase

import (
	"context"
	"maps"
	"slices"

	"vozko/domain/shared"
	"vozko/domain/site"
)

func (s *Service) GetSettings(ctx context.Context, workspaceID, projectID string) (*StoredSettings, error) {
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	if _, err := s.store.GetProject(ctx, workspaceID, projectID); err != nil {
		return nil, err
	}
	return s.store.GetSettings(ctx, workspaceID, projectID)
}

func (s *Service) PutSettings(ctx context.Context, workspaceID, projectID string, expected int64, raw []byte) (*StoredSettings, error) {
	if err := shared.RequireVersion(expected); err != nil {
		return nil, err
	}
	if workspaceID == "" {
		return nil, ErrWorkspaceRequired
	}
	p, err := s.store.GetProject(ctx, workspaceID, projectID)
	if err != nil {
		return nil, err
	}
	if err := editable(*p); err != nil {
		return nil, err
	}
	current, err := s.store.GetSettings(ctx, workspaceID, projectID)
	if err != nil {
		return nil, err
	}
	if err := shared.ExpectVersion(current.Version, &expected); err != nil {
		return nil, err
	}
	settings, err := site.DecodeSettings(raw)
	if err != nil {
		return nil, err
	}
	for _, id := range slices.Sorted(maps.Keys(settings.Forms)) {
		if err := s.references.Check(ctx, workspaceID, id, settings.Forms[id]); err != nil {
			return nil, err
		}
	}
	next := &StoredSettings{ProjectID: projectID, WorkspaceID: workspaceID, Settings: settings, UpdatedAt: s.now()}
	if err := s.store.SaveSettings(ctx, next, expected); err != nil {
		return nil, err
	}
	return next, nil
}
