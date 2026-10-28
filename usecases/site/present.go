package site_usecase

import (
	"context"
	"slices"

	"vozko/domain/sitepublish"
)

func (s *Service) present(ctx context.Context, workspaceID string, projects []Project) error {
	if len(projects) == 0 {
		return nil
	}
	var ids, projectIDs []string
	for i, p := range projects {
		unpublished, err := s.unpublished(ctx, p)
		if err != nil {
			return err
		}
		projects[i].HasUnpublishedChanges = unpublished
		projectIDs = append(projectIDs, p.ID)
		if p.ThumbnailMediaID != "" && !slices.Contains(ids, p.ThumbnailMediaID) {
			ids = append(ids, p.ThumbnailMediaID)
		}
	}
	if err := s.locatePrimaryURLs(ctx, workspaceID, projectIDs, projects); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	urls, err := s.media.Locate(ctx, workspaceID, ids)
	if err != nil {
		return err
	}
	for i := range projects {
		projects[i].ThumbnailURL = urls[projects[i].ThumbnailMediaID]
	}
	return nil
}

func (s *Service) locatePrimaryURLs(ctx context.Context, workspaceID string, projectIDs []string, projects []Project) error {
	hosts, err := s.hostnames.ListByProjects(ctx, workspaceID, projectIDs)
	if err != nil {
		return err
	}
	byProject := map[string][]sitepublish.HostnameRecord{}
	for _, h := range hosts {
		byProject[h.ProjectID] = append(byProject[h.ProjectID], h)
	}
	for i := range projects {
		projects[i].PrimaryURL, _ = sitepublish.PrimaryOrigin(byProject[projects[i].ID], s.origins)
	}
	return nil
}

func (s *Service) liveRevision(ctx context.Context, p Project) (string, error) {
	if p.PublishedDeploymentID == "" {
		return "", nil
	}
	return s.published.PublishedRevision(ctx, p.WorkspaceID, p.PublishedDeploymentID)
}

func (s *Service) unpublished(ctx context.Context, p Project) (bool, error) {
	live, err := s.liveRevision(ctx, p)
	if err != nil {
		return false, err
	}
	return live != p.WorkingRevisionID, nil
}
