package sitepublish_usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"vozko/domain/shared/deps"
	"vozko/domain/sitepublish"
	"vozko/domain/workspace"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

var (
	ResourceSites       = workspace.ResourceSites
	ResourceSiteDomains = workspace.ResourceSiteDomains
	ActionPublish       = workspace.ActionPublish
)

type Service struct {
	Deps
	cfg      Config
	reserved []string
}

func New(d Deps, cfg Config) (*Service, error) {
	for name, dep := range map[string]any{
		"projects": d.Projects, "workspaces": d.Workspaces, "builder": d.Builder, "renderer": d.Renderer, "forms": d.Forms,
		"hosting": d.Hosting, "media": d.Media, "custom hostnames": d.CustomHostnames, "access": d.Access,
		"subscriptions": d.Subscriptions, "entitlements": d.Entitlements, "widgets": d.Widgets, "deployments": d.Deployments,
		"hostnames": d.Hostnames, "abuse cases": d.Abuse, "alerts": d.Alerts, "thumbnails": d.Thumbnails, "locks": d.Locks,
		"dns": d.DNS, "reversals": d.Reversals, "counters": d.Counters, "booking calendars": d.Calendars,
	} {
		if deps.IsNil(dep) {
			return nil, fmt.Errorf("sitepublish: %s is required", name)
		}
	}
	if cfg.Addresses.Domain == "" || cfg.APIOrigin == "" || cfg.CNAMETarget == "" {
		return nil, errors.New("sitepublish: the platform domain, api origin and cname target are required")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.NewID == nil {
		return nil, errors.New("sitepublish: an id generator is required")
	}
	return &Service{Deps: d, cfg: cfg, reserved: ReservedWords(cfg.BrandWords, cfg.CNAMETarget)}, nil
}

func ReservedWords(brand []string, cnameTarget string) []string {
	var words []string
	for _, word := range brand {
		if word = strings.TrimSpace(word); word != "" {
			words = append(words, word)
		}
	}
	if label, _, found := strings.Cut(strings.TrimSpace(cnameTarget), "."); found && label != "" {
		words = append(words, label)
	}
	return words
}

func (s *Service) allow(a Actor, resource workspace.Resource, action workspace.Action) error {
	if a.UserID == "" || a.WorkspaceID == "" {
		return ErrAccessDenied
	}
	if err := s.Access.Execute(a.UserID, a.WorkspaceID, resource, action); err != nil {
		return fmt.Errorf("%w: %w", ErrAccessDenied, err)
	}
	return nil
}

func (s *Service) ensureActive(workspaceID string) error {
	if _, err := s.Subscriptions.Execute(workspaceID); err != nil {
		return fmt.Errorf("%w: %w", ErrSubscriptionInactive, err)
	}
	return nil
}

func (s *Service) billingCut(workspaceID string) (bool, error) {
	_, err := s.Subscriptions.Execute(workspaceID)
	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, workspace_plan.ErrSubscriptionNotActive), errors.Is(err, workspace_plan.ErrSubscriptionNotCurrent):
		return true, nil
	}
	return false, err
}

func (s *Service) serving(ctx context.Context, rec sitepublish.DeploymentRecord) (sitepublish.ServingSettings, error) {
	cut, err := s.billingCut(rec.WorkspaceID)
	if err != nil {
		return sitepublish.ServingSettings{}, err
	}
	suspended, err := s.Abuse.Suspended(ctx, rec.ProjectID)
	if err != nil {
		return sitepublish.ServingSettings{}, err
	}
	return sitepublish.ServingSettings{DeploymentID: rec.ID, Suspended: cut || suspended, Redirects: rec.Redirects}, nil
}

func (s *Service) writeKeys(ctx context.Context, rec sitepublish.DeploymentRecord, settings sitepublish.ServingSettings) ([]string, error) {
	value, err := settings.Encode()
	if err != nil {
		return nil, err
	}
	if err := s.Hosting.PutDeployment(ctx, rec.ID, value); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrHostingUnavailable, err)
	}
	hosts, err := s.Hostnames.ActiveHostnames(ctx, rec.ProjectID)
	if err != nil {
		return nil, err
	}
	for _, host := range hosts {
		if err := s.Hosting.PutHost(ctx, host, value); err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrHostingUnavailable, host, err)
		}
	}
	return s.withdrawRemoved(ctx, rec.ProjectID, hosts)
}

func (s *Service) withdrawRemoved(ctx context.Context, projectID string, written []string) ([]string, error) {
	still, err := s.Hostnames.ActiveHostnames(ctx, projectID)
	if err != nil {
		return nil, err
	}
	kept := make([]string, 0, len(written))
	for _, host := range written {
		if slices.Contains(still, host) {
			kept = append(kept, host)
			continue
		}
		if err := s.Hosting.DeleteHost(ctx, host); err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrHostingUnavailable, host, err)
		}
	}
	return kept, nil
}

type keyMode int

const (
	keysServe keyMode = iota
	keysSuspend
	keysOffline
)

func (s *Service) refreshProject(ctx context.Context, projectID string) error {
	return s.settleKeys(ctx, projectID, keysServe)
}

func (s *Service) dropAddress(ctx context.Context, deploymentID string) error {
	if err := s.Hosting.DeleteDeployment(ctx, deploymentID); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrHostingUnavailable, deploymentID, err)
	}
	return nil
}

func (s *Service) dropOtherAddresses(ctx context.Context, projectID, keep string) error {
	listed, err := s.Deployments.ListServable(ctx, projectID)
	if err != nil {
		return err
	}
	for _, rec := range listed {
		if rec.ID == keep {
			continue
		}
		if err := s.dropAddress(ctx, rec.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) settleKeys(ctx context.Context, projectID string, mode keyMode) error {
	if mode == keysOffline {
		hosts, err := s.Hostnames.ActiveHostnames(ctx, projectID)
		if err != nil {
			return err
		}
		for _, host := range hosts {
			if err := s.Hosting.DeleteHost(ctx, host); err != nil {
				return fmt.Errorf("%w: %s: %w", ErrHostingUnavailable, host, err)
			}
		}
		return s.dropOtherAddresses(ctx, projectID, "")
	}
	live, found, err := s.Deployments.Active(ctx, projectID)
	if err != nil {
		return err
	}
	if found {
		held, err := s.held(ctx, live)
		if err != nil {
			return err
		}
		found = !held
	}
	if !found {
		return s.dropOtherAddresses(ctx, projectID, "")
	}
	if err := s.dropOtherAddresses(ctx, projectID, live.ID); err != nil {
		return err
	}
	settings, err := s.serving(ctx, live)
	if err != nil {
		return err
	}
	settings.Suspended = settings.Suspended || mode == keysSuspend
	if _, err := s.writeKeys(ctx, live, settings); err != nil {
		return err
	}
	return s.syncLive(ctx, live, settings.Suspended)
}

func (s *Service) activateForms(ctx context.Context, rec sitepublish.DeploymentRecord) error {
	if err := s.Forms.Activate(ctx, ActivateForms{
		WorkspaceID: rec.WorkspaceID, ProjectID: rec.ProjectID, DeploymentID: rec.ID, Manifests: rec.Forms.Manifests, Settings: rec.Forms.Settings, Widgets: rec.Forms.Widgets, Bookings: rec.Forms.Bookings,
	}); err != nil {
		return fmt.Errorf("%w: %w", ErrFormsUnavailable, err)
	}
	return nil
}

func (s *Service) deactivateForms(ctx context.Context, projectID string) error {
	if err := s.Forms.Deactivate(ctx, projectID); err != nil {
		return fmt.Errorf("%w: %w", ErrFormsUnavailable, err)
	}
	return nil
}

func (s *Service) syncLive(ctx context.Context, rec sitepublish.DeploymentRecord, suspended bool) error {
	if suspended {
		if err := s.deactivateForms(ctx, rec.ProjectID); err != nil {
			return err
		}
		return s.Projects.SetPublication(ctx, rec.WorkspaceID, rec.ProjectID, PublicationSuspended, rec.ID)
	}
	if err := s.activateForms(ctx, rec); err != nil {
		return err
	}
	return s.Projects.SetPublication(ctx, rec.WorkspaceID, rec.ProjectID, PublicationLive, rec.ID)
}

func (s *Service) held(ctx context.Context, rec sitepublish.DeploymentRecord) (bool, error) {
	open, err := s.Abuse.OpenForProject(ctx, rec.ProjectID)
	if err != nil {
		return false, err
	}
	for _, c := range open {
		if c.Status == sitepublish.AbuseHeld && c.DeploymentID == rec.ID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) failDeployment(ctx context.Context, rec sitepublish.DeploymentRecord, cause error) error {
	rec.Status = sitepublish.DeploymentFailed
	var issues *IssuesFailure
	if errors.As(cause, &issues) {
		rec.Gaps = issues.Issues
	}
	if err := s.Deployments.Update(ctx, rec); err != nil {
		log.Printf("[sitepublish] deployment %s failed (%v) and could not be marked failed: %v", rec.ID, cause, err)
	}
	return cause
}
