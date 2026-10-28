package sitepublish_usecase

import (
	"context"

	"vozko/domain/sitepublish"
	"vozko/domain/workspace"
)

func (s *Service) SettingsChanges(ctx context.Context, a Actor, projectID string) (sitepublish.SettingsChanges, error) {
	if err := s.allow(a, ResourceSites, workspace.ActionRead); err != nil {
		return sitepublish.SettingsChanges{}, err
	}
	current, err := s.Projects.CurrentSettings(ctx, a.WorkspaceID, projectID)
	if err != nil {
		return sitepublish.SettingsChanges{}, err
	}
	active, found, err := s.Deployments.Active(ctx, projectID)
	if err != nil {
		return sitepublish.SettingsChanges{}, err
	}
	if found && active.WorkspaceID != a.WorkspaceID {
		return sitepublish.SettingsChanges{}, ErrProjectNotFound
	}
	return sitepublish.DiffSettings(active.Settings, found, current), nil
}
