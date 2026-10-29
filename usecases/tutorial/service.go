package tutorial_usecase

import (
	"context"
	"strings"
	"time"

	"vozko/domain/shared/deps"
	"vozko/domain/tutorial"
)

type Deps struct {
	Progress  tutorial.ProgressRepository
	SiteFacts tutorial.SiteFactsReader
	Now       func() time.Time
}

type Service struct {
	progress tutorial.ProgressRepository
	sites    tutorial.SiteFactsReader
	now      func() time.Time
}

type Change struct {
	TutorialID string
	Status     tutorial.Status
	Step       int
}

func New(d Deps) (*Service, error) {
	if err := deps.Require("tutorial service", map[string]any{"progress": d.Progress, "site facts": d.SiteFacts}); err != nil {
		return nil, err
	}
	now := d.Now
	if now == nil {
		now = time.Now
	}
	return &Service{progress: d.Progress, sites: d.SiteFacts, now: now}, nil
}

func (s *Service) Progress(ctx context.Context, userID string) ([]tutorial.Progress, error) {
	if err := tutorial.RequireUser(userID); err != nil {
		return nil, err
	}
	return s.progress.List(ctx, userID)
}

func (s *Service) SaveProgress(ctx context.Context, userID string, change Change) (tutorial.Progress, error) {
	if err := tutorial.RequireUser(userID); err != nil {
		return tutorial.Progress{}, err
	}
	next, err := tutorial.NewProgress(change.TutorialID, change.Status, change.Step, s.now().UTC())
	if err != nil {
		return tutorial.Progress{}, err
	}
	previous, found, err := s.progress.Find(ctx, userID, next.TutorialID)
	if err != nil {
		return tutorial.Progress{}, err
	}
	saved := next.After(previous, found)
	if err := s.progress.Save(ctx, userID, saved); err != nil {
		return tutorial.Progress{}, err
	}
	return saved, nil
}

func (s *Service) ResetProgress(ctx context.Context, userID, tutorialID string) error {
	if err := tutorial.RequireUser(userID); err != nil {
		return err
	}
	if !tutorial.ValidID(tutorialID) {
		return tutorial.ErrInvalidID
	}
	return s.progress.Delete(ctx, userID, tutorialID)
}

func (s *Service) SitesChecklist(ctx context.Context, workspaceID string) (tutorial.Checklist, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return tutorial.Checklist{}, tutorial.ErrWorkspaceRequired
	}
	facts, err := s.sites.SiteFacts(ctx, workspaceID)
	if err != nil {
		return tutorial.Checklist{}, err
	}
	return tutorial.SitesChecklist(facts), nil
}
