package sitepublish_usecase

import (
	"context"
	"errors"
	"fmt"

	"vozko/domain/sitepublish"
)

const reconcileBatch = 500

func (s *Service) reconcile(ctx context.Context) []error {
	return append(s.reconcileServing(ctx), s.reconcileDomains(ctx)...)
}

func servingSettled(live sitepublish.LiveProject, cut bool) bool {
	want := PublicationLive
	if cut || live.AbuseSuspended {
		want = PublicationSuspended
	}
	return live.ProjectStatus == string(want)
}

func (s *Service) reconcileServing(ctx context.Context) []error {
	var failures []error
	cuts := map[string]bool{}
	unreadable := map[string]bool{}
	for after := ""; ; {
		page, err := s.Deployments.LiveProjects(ctx, after, reconcileBatch)
		if err != nil {
			return append(failures, fmt.Errorf("live pages: %w", err))
		}
		for _, live := range page {
			if unreadable[live.WorkspaceID] {
				continue
			}
			cut, known := cuts[live.WorkspaceID]
			if !known {
				if cut, err = s.billingCut(live.WorkspaceID); err != nil {
					unreadable[live.WorkspaceID] = true
					failures = append(failures, fmt.Errorf("workspace %s subscription: %w", live.WorkspaceID, err))
					continue
				}
				cuts[live.WorkspaceID] = cut
			}
			if servingSettled(live, cut) {
				continue
			}
			err := s.exclusive(live.ProjectID, func() error { return s.refreshProject(ctx, live.ProjectID) })
			if err != nil && !errors.Is(err, ErrPublishInProgress) {
				failures = append(failures, fmt.Errorf("page %s serving: %w", live.ProjectID, err))
			}
		}
		if len(page) < reconcileBatch {
			return failures
		}
		after = page[len(page)-1].ProjectID
	}
}

func (s *Service) reconcileDomains(ctx context.Context) []error {
	var failures []error
	for after := ""; ; {
		workspaces, err := s.Hostnames.CustomWorkspaces(ctx, after, reconcileBatch)
		if err != nil {
			return append(failures, fmt.Errorf("domain workspaces: %w", err))
		}
		for _, workspaceID := range workspaces {
			if err := s.enforceDomains(ctx, workspaceID); err != nil {
				failures = append(failures, fmt.Errorf("workspace %s domains: %w", workspaceID, err))
			}
		}
		if len(workspaces) < reconcileBatch {
			return failures
		}
		after = workspaces[len(workspaces)-1]
	}
}
