package sitenext_usecase

import (
	"context"
	"errors"
	"fmt"

	"vozko/domain/shared/deps"
	"vozko/domain/site"
	"vozko/domain/sitenext"
	"vozko/domain/sitepublish"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/siteinspect"
)

var ErrUnavailable = errors.New("sitenext: the next steps could not be read")

type Projects interface {
	Get(ctx context.Context, workspaceID, id string) (*site_usecase.View, error)
}

type Outlines interface {
	Outline(ctx context.Context, v *site_usecase.View) siteinspect.Report
}

type Hostnames interface {
	ListByProject(ctx context.Context, workspaceID, projectID string) ([]sitepublish.HostnameRecord, error)
}

type Deps struct {
	Projects  Projects
	Outlines  Outlines
	Hostnames Hostnames
}

type Service struct {
	projects  Projects
	outlines  Outlines
	hostnames Hostnames
}

type Result struct {
	Checked bool
	Steps   []sitenext.Step
}

func New(d Deps) (*Service, error) {
	if err := deps.Require("site next steps", map[string]any{"projects": d.Projects, "outlines": d.Outlines, "hostnames": d.Hostnames}); err != nil {
		return nil, err
	}
	return &Service{projects: d.Projects, outlines: d.Outlines, hostnames: d.Hostnames}, nil
}

func (s *Service) Next(ctx context.Context, workspaceID, projectID string) (Result, error) {
	v, err := s.projects.Get(ctx, workspaceID, projectID)
	if err != nil {
		return Result{}, err
	}
	report := s.outlines.Outline(ctx, v)
	if !report.Known {
		return Result{Steps: []sitenext.Step{}}, nil
	}
	facts := sitenext.Facts{Gaps: report.Gaps, Forms: report.Forms, Settings: settingsOf(v), Source: sitenext.ScanSource(v.Source()), Stage: stageOf(v.Project)}
	if facts.Stage.Live() {
		recs, err := s.hostnames.ListByProject(ctx, workspaceID, projectID)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
		facts.CustomDomain = sitenext.HasCustomDomain(recs)
	}
	return Result{Checked: true, Steps: sitenext.Next(facts)}, nil
}

func settingsOf(v *site_usecase.View) site.Settings {
	if v.Settings == nil {
		return site.Settings{}
	}
	return v.Settings.Settings
}

func stageOf(p site_usecase.Project) sitenext.Stage {
	switch p.PublishState() {
	case site_usecase.PublishDraft:
		return sitenext.StageDraft
	case site_usecase.PublishLive:
		return sitenext.StageLive
	case site_usecase.PublishLiveWithChanges:
		return sitenext.StageLiveWithChanges
	}
	return sitenext.StageClosed
}
