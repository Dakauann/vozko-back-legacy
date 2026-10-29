package container

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"

	"vozko/delivery/http/sitepreview"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/infra/config"
	site_repository "vozko/infra/repositories/site"
	webchat_repository "vozko/infra/repositories/webchat"
	"vozko/infra/sitebuilder"
	"vozko/infra/siteruntime"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/sitebuild"
	wcuc "vozko/usecases/webchat"
)

func siteRuntimeFiles() map[string]string {
	files, err := siteruntime.Files()
	if err != nil {
		log.Fatalf("[sites] the embedded runtime files are missing: %v", err)
	}
	return files
}

func apiOrigin(sites config.SitesConfig) string {
	u, err := url.Parse(sites.PublicBaseURL)
	if err != nil || u.Host == "" {
		log.Fatalf("[sites] API_BASE_URL is not an absolute url")
	}
	return u.Scheme + "://" + u.Host
}

func (c *Container) siteMediaOrigin() string {
	u, err := url.Parse(c.s3.GetFileURL(""))
	if err != nil || u.Host == "" {
		log.Fatalf("[sites] CLOUDFLARE_R2_ENDPOINT is not an absolute url")
	}
	return u.Scheme + "://" + u.Host
}

func (c *Container) siteSections() *sitebuild.WorkspaceSections {
	library, err := sitebuild.Sections()
	if err != nil {
		log.Fatalf("[sites] the section library: %v", err)
	}
	return sitebuild.NewWorkspaceSections(library, siteSnippetSource{store: site_repository.New(c.db)})
}

func (c *Container) siteBuild() *sitebuild.Service {
	if c.siteShared.build != nil {
		return c.siteShared.build
	}
	sites := c.cfg.Sites
	client, err := sitebuilder.New(sitebuilder.Config{BaseURL: sites.BuilderURL, Token: sites.BuilderToken})
	if err != nil {
		log.Fatalf("[sites] %v", err)
	}
	sections := c.siteSections()
	service, err := sitebuild.NewService(sitebuild.Config{
		Addresses:  siteAddresses(sites),
		SigningKey: sites.PreviewSigningKey,
		APIOrigin:  apiOrigin(sites),
		Runtime:    siteRuntimeFiles(),
	}, sitebuild.Deps{
		Builder:    client,
		Cache:      c.redisProvider.Cache(),
		Media:      sitebuild.MediaFromRepository(c.repositories.media),
		Sections:   sections,
		WhatsApp:   c.siteFormsForPublish(),
		Widgets:    webchat_repository.NewWidgetRepository(c.db),
		Workspaces: siteWorkspaceNames{c: c},
	})
	if err != nil {
		log.Fatalf("[sites] %v", err)
	}
	c.siteShared.build = service
	return service
}

type sitePreviewGrants struct {
	c *Container
}

func (g sitePreviewGrants) PreviewOrigin(origin string) bool {
	return g.c.siteBuild().PreviewOrigin(origin)
}

func (g sitePreviewGrants) PreviewGrant(ctx context.Context, origin string) (sitepublish.PreviewGrant, bool, error) {
	return g.c.siteBuild().PreviewGrant(ctx, origin)
}

var _ wcuc.PreviewGrants = sitePreviewGrants{}

type siteBuildEditor struct {
	service *sitebuild.Service
}

func (e siteBuildEditor) Execute(ctx context.Context, req site_usecase.ExecuteRequest) (site_usecase.ExecuteResult, error) {
	out, err := e.service.Execute(ctx, sitebuild.ExecuteInput{WorkspaceID: req.WorkspaceID, ProjectID: req.ProjectID, Project: req.Project, Batch: req.Batch, Bases: req.Bases})
	if err != nil {
		return site_usecase.ExecuteResult{}, err
	}
	return site_usecase.ExecuteResult{Steps: out.Steps, Failures: out.SiteFailures(), Skipped: out.Skipped}, nil
}

func (e siteBuildEditor) Extract(ctx context.Context, req site_usecase.ExtractRequest) (site_usecase.ExtractResult, error) {
	out, err := e.service.Extract(ctx, sitebuild.ExtractInput{WorkspaceID: req.WorkspaceID, ProjectID: req.ProjectID, Project: req.Project, Node: req.Node, Rev: req.Rev, Bases: req.Bases})
	if err != nil {
		return site_usecase.ExtractResult{}, err
	}
	result := site_usecase.ExtractResult{Source: out.Source}
	if out.Failure != nil {
		result.Failure = &site.Failure{Index: out.Failure.Index, Code: out.Failure.Code, Field: out.Failure.Field, Detail: out.Failure.Detail}
	}
	return result, nil
}

func (e siteBuildEditor) Preview(ctx context.Context, req site_usecase.PreviewRequest) (site_usecase.PreviewResult, error) {
	out, err := e.service.Preview(ctx, sitebuild.PreviewInput{
		WorkspaceID: req.WorkspaceID, ProjectID: req.ProjectID, RevisionID: req.RevisionID, Project: req.Project, Settings: req.Settings,
	})
	if err != nil {
		return site_usecase.PreviewResult{}, siteEditorBuildError(err)
	}
	return site_usecase.PreviewResult{URL: out.URL, RevisionID: out.Revision, Build: out.Build}, nil
}

func siteEditorBuildError(err error) error {
	var failure *sitebuild.BuildFailure
	if !errors.As(err, &failure) {
		return err
	}
	issues := make([]site_usecase.BuildIssue, 0, len(failure.Errors))
	for _, e := range failure.Errors {
		issues = append(issues, site_usecase.BuildIssue{Path: e.Path, Line: e.Line, Column: e.Column, Message: e.Message, Code: e.Code})
	}
	return &site_usecase.BuildFailedError{Errors: issues}
}

var (
	_ site_usecase.Executor  = siteBuildEditor{}
	_ site_usecase.Previewer = siteBuildEditor{}
)

func (c *Container) sitePreviewHost(next http.Handler) http.Handler {
	sites := c.cfg.Sites
	handler, err := sitepreview.NewHandler(c.siteBuild(), sitepreview.Origins{Editors: c.cfg.CORSTrustedOrigins, API: apiOrigin(sites), Media: c.siteMediaOrigin()})
	if err != nil {
		log.Fatalf("[sites] %v", err)
	}
	return sitepreview.Dispatch(siteAddresses(sites), handler, next)
}
