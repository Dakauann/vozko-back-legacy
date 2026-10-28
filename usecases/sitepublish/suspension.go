package sitepublish_usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"vozko/domain/sitepublish"
)

const maxAdminCases = 200

func (s *Service) Rollback(ctx context.Context, a Actor, projectID, deploymentID string) (sitepublish.DeploymentRecord, error) {
	if err := s.allow(a, ResourceSites, ActionPublish); err != nil {
		return sitepublish.DeploymentRecord{}, err
	}
	if err := s.ensureActive(a.WorkspaceID); err != nil {
		return sitepublish.DeploymentRecord{}, err
	}
	var rec sitepublish.DeploymentRecord
	err := s.exclusive(projectID, func() error {
		var err error
		rec, err = s.rollbackLocked(ctx, a, projectID, deploymentID)
		return err
	})
	return rec, err
}

func (s *Service) rollbackLocked(ctx context.Context, a Actor, projectID, deploymentID string) (sitepublish.DeploymentRecord, error) {
	rec, err := s.Deployments.Get(ctx, a.WorkspaceID, deploymentID)
	if err != nil {
		return sitepublish.DeploymentRecord{}, err
	}
	if rec.ProjectID != projectID {
		return sitepublish.DeploymentRecord{}, sitepublish.ErrDeploymentNotFound
	}
	if rec.Status == sitepublish.DeploymentActive {
		return rec, nil
	}
	if !(sitepublish.Deployment{Status: rec.Status}).Activatable() {
		return sitepublish.DeploymentRecord{}, fmt.Errorf("%w: %s", sitepublish.ErrDeploymentTransition, rec.Status)
	}
	if err := s.reviewable(ctx, projectID); err != nil {
		return sitepublish.DeploymentRecord{}, err
	}
	if _, err := s.goLive(ctx, rec, a.Ref()); err != nil {
		return sitepublish.DeploymentRecord{}, err
	}
	rec.Status = sitepublish.DeploymentActive
	if err := s.Projects.SetPublication(ctx, a.WorkspaceID, projectID, PublicationLive, rec.ID); err != nil {
		return rec, fmt.Errorf("%w: %w", ErrPublishUnrecorded, err)
	}
	return rec, nil
}

func (s *Service) Unpublish(ctx context.Context, a Actor, projectID string) error {
	if err := s.allow(a, ResourceSites, ActionPublish); err != nil {
		return err
	}
	if _, err := s.Projects.Describe(ctx, a.WorkspaceID, projectID); err != nil {
		return err
	}
	return s.exclusive(projectID, func() error { return s.unpublishLocked(ctx, a, projectID) })
}

func (s *Service) unpublishLocked(ctx context.Context, a Actor, projectID string) error {
	active, found, err := s.Deployments.Active(ctx, projectID)
	if err != nil {
		return err
	}
	if found && active.WorkspaceID != a.WorkspaceID {
		return ErrProjectNotFound
	}
	if err := s.settleKeys(ctx, projectID, keysOffline); err != nil {
		return err
	}
	if err := s.deactivateForms(ctx, projectID); err != nil {
		return err
	}
	if found {
		active.Status = sitepublish.DeploymentSuperseded
		if err := s.Deployments.Update(ctx, active); err != nil {
			return err
		}
	}
	return s.Projects.SetPublication(ctx, a.WorkspaceID, projectID, PublicationOffline, "")
}

func (s *Service) Retire(ctx context.Context, workspaceID, projectID string) error {
	if workspaceID == "" {
		return ErrProjectNotFound
	}
	if _, err := s.Projects.Describe(ctx, workspaceID, projectID); err != nil {
		return err
	}
	if err := s.settleKeys(ctx, projectID, keysOffline); err != nil {
		return err
	}
	return s.removeHostnames(ctx, workspaceID, projectID)
}

func (s *Service) removeHostnames(ctx context.Context, workspaceID, projectID string) error {
	list, err := s.Hostnames.ListByProject(ctx, workspaceID, projectID)
	if err != nil {
		return err
	}
	for _, rec := range list {
		if _, err := s.lockedDomain(ctx, rec, s.remove); err != nil && !errors.Is(err, sitepublish.ErrHostnameNotFound) {
			return err
		}
	}
	return nil
}

func (s *Service) OnAccessCut(ctx context.Context, workspaceID string) error {
	return errors.Join(s.Suspend(ctx, workspaceID), s.enforceDomains(ctx, workspaceID))
}

func (s *Service) OnAccessRestored(ctx context.Context, workspaceID string) error {
	return errors.Join(s.Restore(ctx, workspaceID), s.enforceDomains(ctx, workspaceID))
}

func (s *Service) Suspend(ctx context.Context, workspaceID string) error {
	return s.rewriteWorkspace(ctx, workspaceID, true)
}

func (s *Service) Restore(ctx context.Context, workspaceID string) error {
	return s.rewriteWorkspace(ctx, workspaceID, false)
}

func (s *Service) rewriteWorkspace(ctx context.Context, workspaceID string, cut bool) error {
	active, err := s.Deployments.ListActiveByWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	mode := keysServe
	if cut {
		mode = keysSuspend
	}
	var failures []error
	for _, rec := range active {
		if err := s.settleKeys(ctx, rec.ProjectID, mode); err != nil {
			failures = append(failures, fmt.Errorf("page %s: %w", rec.ProjectID, err))
		}
	}
	return errors.Join(failures...)
}

type AdminCase struct {
	sitepublish.AbuseCase
	ProjectName   string
	WorkspaceName string
}

func (s *Service) AdminCases(ctx context.Context, statuses []sitepublish.AbuseStatus) ([]AdminCase, error) {
	cases, err := s.Abuse.List(ctx, statuses, maxAdminCases)
	if err != nil {
		return nil, err
	}
	workspaces, projects := map[string]string{}, map[string]string{}
	out := make([]AdminCase, 0, len(cases))
	for _, c := range cases {
		ws, seen := workspaces[c.WorkspaceID]
		if !seen {
			if ws, err = s.Workspaces.WorkspaceName(ctx, c.WorkspaceID); err != nil && !errors.Is(err, ErrProjectNotFound) {
				return nil, err
			}
			workspaces[c.WorkspaceID] = ws
		}
		name, seen := projects[c.ProjectID]
		if !seen {
			snap, err := s.Projects.Describe(ctx, c.WorkspaceID, c.ProjectID)
			if err != nil && !errors.Is(err, ErrProjectNotFound) {
				return nil, err
			}
			name = snap.Name
			projects[c.ProjectID] = name
		}
		out = append(out, AdminCase{AbuseCase: c, ProjectName: name, WorkspaceName: ws})
	}
	return out, nil
}

func (s *Service) projectWorkspace(ctx context.Context, projectID string) (sitepublish.DeploymentRecord, error) {
	rec, err := s.Deployments.Latest(ctx, projectID)
	if errors.Is(err, sitepublish.ErrDeploymentNotFound) {
		return rec, ErrProjectNotFound
	}
	return rec, err
}

func (s *Service) AdminSuspend(ctx context.Context, adminID, projectID, reason string) (sitepublish.AbuseCase, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return sitepublish.AbuseCase{}, ErrReasonRequired
	}
	latest, err := s.projectWorkspace(ctx, projectID)
	if err != nil {
		return sitepublish.AbuseCase{}, err
	}
	now := s.Now()
	c := sitepublish.AbuseCase{
		ID: s.NewID(), WorkspaceID: latest.WorkspaceID, ProjectID: projectID, Source: sitepublish.AbuseAdmin,
		Status: sitepublish.AbuseSuspended, Reason: reason, DecidedBy: adminID, DecidedAt: &now, CreatedAt: now,
	}
	if err := s.Abuse.Create(ctx, c); err != nil {
		return sitepublish.AbuseCase{}, err
	}
	return c, s.refreshProject(ctx, projectID)
}

func (s *Service) AdminRestore(ctx context.Context, adminID, projectID, reason string) ([]sitepublish.AbuseCase, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, ErrReasonRequired
	}
	var closed []sitepublish.AbuseCase
	err := s.exclusive(projectID, func() error {
		var err error
		closed, err = s.closeCasesAndRelease(ctx, adminID, projectID, reason)
		return err
	})
	if err != nil {
		return nil, err
	}
	return closed, s.refreshProject(ctx, projectID)
}

func (s *Service) closeCasesAndRelease(ctx context.Context, adminID, projectID, reason string) ([]sitepublish.AbuseCase, error) {
	open, err := s.Abuse.OpenForProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	now := s.Now()
	var release string
	closed := make([]sitepublish.AbuseCase, 0, len(open))
	for _, c := range open {
		if c.Status == sitepublish.AbuseHeld {
			release = c.DeploymentID
			c.Status = sitepublish.AbuseReleased
		} else {
			c.Status = sitepublish.AbuseRestored
		}
		c.Reason = strings.TrimSpace(c.Reason + "\n" + reason)
		c.DecidedBy, c.DecidedAt = adminID, &now
		if err := s.Abuse.Update(ctx, c); err != nil {
			return nil, err
		}
		closed = append(closed, c)
	}
	if release != "" {
		latest, err := s.projectWorkspace(ctx, projectID)
		if err != nil {
			return nil, err
		}
		rec, err := s.Deployments.Get(ctx, latest.WorkspaceID, release)
		if err != nil {
			return nil, err
		}
		if rec.Status == sitepublish.DeploymentReady {
			if _, err := s.goLive(ctx, rec, "admin:"+adminID); err != nil {
				return nil, err
			}
		}
	}
	return closed, nil
}
