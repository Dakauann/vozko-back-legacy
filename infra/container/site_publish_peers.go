package container

import (
	"context"
	"errors"

	sitepublish_repository "vozko/infra/repositories/sitepublish"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/sitebuild"
	"vozko/usecases/siteforms"
	sitepublish_usecase "vozko/usecases/sitepublish"
)

type siteCandidateBuilder struct {
	service *sitebuild.Service
}

func (b siteCandidateBuilder) Candidate(ctx context.Context, req sitepublish_usecase.CandidateRequest) (sitepublish_usecase.Candidate, error) {
	out, err := b.service.Candidate(ctx, sitebuild.CandidateInput{
		WorkspaceID: req.WorkspaceID, ProjectID: req.ProjectID, Project: req.Project, Settings: req.Settings, FormKeys: req.FormKeys, PrimaryOrigin: req.PrimaryOrigin, OGImages: req.OGImages,
	})
	var failure *sitebuild.BuildFailure
	if errors.As(err, &failure) {
		return sitepublish_usecase.Candidate{Errors: candidateBuildErrors(failure.Errors)}, nil
	}
	if err != nil {
		return sitepublish_usecase.Candidate{}, err
	}
	c := sitepublish_usecase.Candidate{URL: out.URL, Imports: out.Result.Imports, PrerenderPath: out.Result.PrerenderPath, Errors: candidateBuildErrors(out.Result.Errors)}
	for _, a := range out.Result.Assets {
		c.Assets = append(c.Assets, sitepublish_usecase.BuildAsset{Path: a.Path, ContentType: a.ContentType, Body: a.Body})
	}
	for _, r := range out.Result.Routes {
		c.Routes = append(c.Routes, sitepublish_usecase.BuildRoute{Route: r.Route, HTMLPath: r.HTMLPath, Interactive: r.Interactive})
	}
	return c, nil
}

func candidateBuildErrors(errs []sitebuild.BuildError) []sitepublish_usecase.BuildError {
	var out []sitepublish_usecase.BuildError
	for _, e := range errs {
		out = append(out, sitepublish_usecase.BuildError{Path: e.Path, Line: e.Line, Column: e.Column, Message: e.Message, Code: e.Code})
	}
	return out
}

var (
	_ sitepublish_usecase.CandidateBuilder = siteCandidateBuilder{}
	_ sitepublish_usecase.Forms            = siteFormsForPublish{}
	_ sitepublish_usecase.Projects         = siteProjectsAdapter{}
	_ sitebuild.WhatsAppLinks              = siteFormsForPublish{}
	_ sitebuild.WorkspaceNames             = siteWorkspaceNames{}
	_ site_usecase.FormsRetirement         = siteFormsForPublish{}
	_ site_usecase.PublicationRetirement   = siteRetirement{}
	_ sitepublish_usecase.Prerenderer      = sitePrerenderer{}
	_ sitepublish_usecase.MediaLibrary     = siteMediaLibrary{}
)

var _ siteforms.HostnameReader = (*sitepublish_repository.Hostnames)(nil)
