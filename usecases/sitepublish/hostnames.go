package sitepublish_usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"vozko/domain/sitepublish"
	"vozko/domain/workspace"
)

const maxSlugAttempts = 20

type AddHostnameInput struct {
	Kind     sitepublish.HostnameKind
	Slug     string
	Hostname string
}

func (s *Service) platformCandidate(ctx context.Context, snap ProjectSnapshot) (string, error) {
	name, err := s.Workspaces.WorkspaceName(ctx, snap.WorkspaceID)
	if err != nil {
		return "", err
	}
	page := snap.Slug
	if page == "" {
		page = snap.Name
	}
	return sitepublish.PlatformSlug(page, name), nil
}

func (s *Service) platformRecord(ctx context.Context, workspaceID, projectID string) (sitepublish.HostnameRecord, bool, error) {
	list, err := s.Hostnames.ListByProject(ctx, workspaceID, projectID)
	if err != nil {
		return sitepublish.HostnameRecord{}, false, err
	}
	for _, h := range list {
		if h.Kind == sitepublish.HostnamePlatform && h.Status != sitepublish.HostnameRemoving {
			return h, true, nil
		}
	}
	return sitepublish.HostnameRecord{}, false, nil
}

func (s *Service) ensurePlatformHostname(ctx context.Context, workspaceID string, snap ProjectSnapshot) error {
	if _, found, err := s.platformRecord(ctx, workspaceID, snap.ID); err != nil || found {
		return err
	}
	slug, err := s.platformCandidate(ctx, snap)
	if err != nil {
		return err
	}
	_, err = s.createPlatform(ctx, workspaceID, snap.ID, slug, true)
	return err
}

func (s *Service) createPlatform(ctx context.Context, workspaceID, projectID, slug string, retry bool) (sitepublish.HostnameRecord, error) {
	attempts := 1
	if retry {
		attempts = maxSlugAttempts
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		candidate := slug
		if i > 0 {
			suffix := "-" + strconv.Itoa(i+1)
			candidate = strings.TrimRight(slug[:min(len(slug), sitepublish.MaxSlugLength-len(suffix))], "-") + suffix
		}
		host, err := sitepublish.PlatformHostname(candidate, s.cfg.Addresses.Domain, s.reserved...)
		if err != nil {
			if retry && errors.Is(err, sitepublish.ErrSlugReserved) {
				lastErr = err
				continue
			}
			return sitepublish.HostnameRecord{}, err
		}
		now := s.Now()
		rec := sitepublish.HostnameRecord{
			ID: s.NewID(), WorkspaceID: workspaceID, ProjectID: projectID, Hostname: host, Kind: sitepublish.HostnamePlatform,
			Status: sitepublish.InitialHostnameStatus(sitepublish.HostnamePlatform), CreatedAt: now, UpdatedAt: now,
		}
		err = s.Hostnames.Create(ctx, rec)
		if err == nil {
			return rec, nil
		}
		if !errors.Is(err, sitepublish.ErrHostnameTaken) {
			return sitepublish.HostnameRecord{}, err
		}
		lastErr = err
	}
	return sitepublish.HostnameRecord{}, lastErr
}

func (s *Service) ListHostnames(ctx context.Context, a Actor, projectID string) ([]sitepublish.HostnameRecord, error) {
	if err := s.allow(a, ResourceSiteDomains, workspace.ActionRead); err != nil {
		return nil, err
	}
	return s.Hostnames.ListByProject(ctx, a.WorkspaceID, projectID)
}

func (s *Service) ListDeployments(ctx context.Context, a Actor, projectID string, limit int) ([]sitepublish.DeploymentRecord, error) {
	if err := s.allow(a, ResourceSites, workspace.ActionRead); err != nil {
		return nil, err
	}
	return s.Deployments.ListByProject(ctx, a.WorkspaceID, projectID, limit)
}

func (s *Service) AddHostname(ctx context.Context, a Actor, projectID string, in AddHostnameInput) (sitepublish.HostnameRecord, error) {
	if err := s.allow(a, ResourceSiteDomains, workspace.ActionUpdate); err != nil {
		return sitepublish.HostnameRecord{}, err
	}
	if err := s.ensureActive(a.WorkspaceID); err != nil {
		return sitepublish.HostnameRecord{}, err
	}
	snap, err := s.Projects.Describe(ctx, a.WorkspaceID, projectID)
	if err != nil {
		return sitepublish.HostnameRecord{}, err
	}
	switch in.Kind {
	case sitepublish.HostnamePlatform:
		return s.addPlatform(ctx, a.WorkspaceID, snap, in.Slug)
	case sitepublish.HostnameCustom:
		return s.addCustom(ctx, a.WorkspaceID, projectID, in.Hostname)
	}
	return sitepublish.HostnameRecord{}, ErrHostnameKindInvalid
}

func (s *Service) addPlatform(ctx context.Context, workspaceID string, snap ProjectSnapshot, slug string) (sitepublish.HostnameRecord, error) {
	if _, found, err := s.platformRecord(ctx, workspaceID, snap.ID); err != nil {
		return sitepublish.HostnameRecord{}, err
	} else if found {
		return sitepublish.HostnameRecord{}, ErrPlatformHostnameTaken
	}
	retry := false
	if strings.TrimSpace(slug) == "" {
		candidate, err := s.platformCandidate(ctx, snap)
		if err != nil {
			return sitepublish.HostnameRecord{}, err
		}
		slug, retry = candidate, true
	}
	rec, err := s.createPlatform(ctx, workspaceID, snap.ID, strings.TrimSpace(slug), retry)
	if err != nil {
		return sitepublish.HostnameRecord{}, err
	}
	return rec, s.serveHostname(ctx, rec)
}

func (s *Service) serveHostname(ctx context.Context, rec sitepublish.HostnameRecord) error {
	active, found, err := s.Deployments.Active(ctx, rec.ProjectID)
	if err != nil || !found {
		return err
	}
	settings, err := s.serving(ctx, active)
	if err != nil {
		return err
	}
	value, err := settings.Encode()
	if err != nil {
		return err
	}
	if err := s.Hosting.PutHost(ctx, rec.Hostname, value); err != nil {
		return fmt.Errorf("%w: %w", ErrHostingUnavailable, err)
	}
	return nil
}

func (s *Service) RemoveHostname(ctx context.Context, a Actor, projectID, hostnameID string) (sitepublish.HostnameRecord, error) {
	if err := s.allow(a, ResourceSiteDomains, workspace.ActionUpdate); err != nil {
		return sitepublish.HostnameRecord{}, err
	}
	rec, err := s.Hostnames.Get(ctx, a.WorkspaceID, hostnameID)
	if err != nil {
		return sitepublish.HostnameRecord{}, err
	}
	if rec.ProjectID != projectID {
		return sitepublish.HostnameRecord{}, sitepublish.ErrHostnameNotFound
	}
	return s.lockedDomain(ctx, rec, s.remove)
}

func (s *Service) remove(ctx context.Context, rec sitepublish.HostnameRecord) (sitepublish.HostnameRecord, error) {
	if err := s.startRemoval(ctx, &rec); err != nil {
		return rec, err
	}
	if err := s.finishRemoval(ctx, rec); err != nil {
		log.Printf("[sitepublish] hostname %s stays removing until the sweep retries: %v", rec.Hostname, err)
	}
	return rec, nil
}

func (s *Service) startRemoval(ctx context.Context, rec *sitepublish.HostnameRecord) error {
	if rec.Status == sitepublish.HostnameRemoving {
		return nil
	}
	if err := rec.Transition(sitepublish.HostnameRemoving); err != nil {
		return err
	}
	rec.UpdatedAt = s.Now()
	return s.Hostnames.Update(ctx, *rec)
}

func (s *Service) finishRemoval(ctx context.Context, rec sitepublish.HostnameRecord) error {
	if err := s.Hosting.DeleteHost(ctx, rec.Hostname); err != nil {
		return err
	}
	if rec.Kind == sitepublish.HostnameCustom && rec.CloudflareRef != "" {
		if err := s.deleteProviderRecord(ctx, rec.CloudflareRef); err != nil {
			return err
		}
	}
	return s.Hostnames.Delete(ctx, rec.ID)
}
