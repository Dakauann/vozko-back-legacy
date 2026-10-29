package container

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"vozko/brand"
	deliveryHttp "vozko/delivery/http"
	"vozko/delivery/http/sitepublishhttp"
	"vozko/domain/media"
	"vozko/infra/cloudflare/customhostnames"
	cronPackage "vozko/infra/cron"
	"vozko/infra/dnslookup"
	invoice_repository "vozko/infra/repositories/invoice"
	media_repository "vozko/infra/repositories/media"
	site_repository "vozko/infra/repositories/site"
	siteform_repository "vozko/infra/repositories/siteform"
	sitepublish_repository "vozko/infra/repositories/sitepublish"
	webchat_repository "vozko/infra/repositories/webchat"
	"vozko/infra/s3"
	"vozko/infra/sitehosting"
	site_usecase "vozko/usecases/site"
	siteanalytics_usecase "vozko/usecases/siteanalytics"
	sitepublish_usecase "vozko/usecases/sitepublish"
	"vozko/usecases/siterender"
)

type sitePublishModule struct {
	publisher *sitepublish_usecase.Service
	analytics *siteanalytics_usecase.Service
	hostnames *sitepublish_repository.Hostnames
}

type siteProjectsAdapter struct {
	storage *site_usecase.Service
	marker  *site_usecase.PublishMarker
}

func (a siteProjectsAdapter) translate(err error) error {
	if errors.Is(err, site_usecase.ErrProjectNotFound) {
		return fmt.Errorf("%w: %w", sitepublish_usecase.ErrProjectNotFound, err)
	}
	return err
}

func (a siteProjectsAdapter) Describe(ctx context.Context, workspaceID, projectID string) (sitepublish_usecase.ProjectSnapshot, error) {
	view, err := a.storage.Get(ctx, workspaceID, projectID)
	if err != nil {
		return sitepublish_usecase.ProjectSnapshot{}, a.translate(err)
	}
	return sitepublish_usecase.ProjectSnapshot{ID: view.Project.ID, WorkspaceID: view.Project.WorkspaceID, Name: view.Project.Name, Slug: view.Project.Slug, RevisionID: view.Revision.ID, Version: view.Project.Version}, nil
}

func (a siteProjectsAdapter) Working(ctx context.Context, workspaceID, projectID string) (sitepublish_usecase.ProjectSnapshot, error) {
	view, err := a.storage.PublishSource(ctx, workspaceID, projectID)
	if err != nil {
		return sitepublish_usecase.ProjectSnapshot{}, a.translate(err)
	}
	return sitepublish_usecase.ProjectSnapshot{
		ID: view.Project.ID, WorkspaceID: view.Project.WorkspaceID, Name: view.Project.Name, Slug: view.Project.Slug,
		Project: view.Source(), RevisionID: view.Revision.ID, Version: view.Project.Version,
	}, nil
}

func (a siteProjectsAdapter) setPublication(ctx context.Context, workspaceID, projectID string, state sitepublish_usecase.Publication, deploymentID string) (*site_usecase.Project, error) {
	if !state.Valid() {
		return nil, fmt.Errorf("%w: %s", site_usecase.ErrPublicationInvalid, state)
	}
	p, err := a.storage.SetPublication(ctx, workspaceID, projectID, site_usecase.Status(state), deploymentID)
	if err != nil {
		return nil, a.translate(err)
	}
	return p, nil
}

func (a siteProjectsAdapter) SetPublication(ctx context.Context, workspaceID, projectID string, state sitepublish_usecase.Publication, deploymentID string) error {
	_, err := a.setPublication(ctx, workspaceID, projectID, state, deploymentID)
	return err
}

func (a siteProjectsAdapter) RecordPublish(ctx context.Context, in sitepublish_usecase.PublishRecord) (sitepublish_usecase.PublishRecorded, error) {
	rev, marked, err := a.marker.Mark(ctx, site_usecase.MarkPublishedInput{
		WorkspaceID: in.WorkspaceID, ProjectID: in.ProjectID, SourceRevisionID: in.SourceRevisionID, SourceVersion: in.SourceVersion,
		Files: in.Project.Files, Author: site_usecase.Author(in.Author),
	})
	if err != nil {
		return sitepublish_usecase.PublishRecorded{}, a.translate(err)
	}
	p, err := a.setPublication(ctx, in.WorkspaceID, in.ProjectID, sitepublish_usecase.PublicationLive, in.DeploymentID)
	if err != nil {
		return sitepublish_usecase.PublishRecorded{}, err
	}
	version := p.Version
	if marked.Project != nil {
		version = max(version, marked.Project.Version)
	}
	return sitepublish_usecase.PublishRecorded{RevisionID: rev.ID, Version: version, Current: marked.Current}, nil
}

type siteWorkspaceNames struct {
	c *Container
}

func (n siteWorkspaceNames) WorkspaceName(_ context.Context, workspaceID string) (string, error) {
	ws, err := n.c.repositories.workspace.GetWorkspaceByID(workspaceID)
	if err != nil {
		return "", err
	}
	if ws == nil {
		return "", sitepublish_usecase.ErrProjectNotFound
	}
	return ws.Name, nil
}

type sitePrerenderer struct {
	renderer *siterender.Service
}

func (p sitePrerenderer) Prerender(ctx context.Context, url string, routes []string) (map[string]string, error) {
	return p.renderer.Prerender(ctx, siterender.PrerenderRequest{URL: url, Routes: routes})
}

func (p sitePrerenderer) LiveDOM(ctx context.Context, url string, routes []string) (map[string]string, error) {
	return p.renderer.LiveDOM(ctx, siterender.LiveDOMRequest{URL: url, Routes: routes})
}

type siteMediaLibrary struct {
	files *s3.S3Service
	rows  interface {
		GetMediaForWorkspaceByURL(workspaceID, url string) (*media.Media, error)
	}
}

func (m siteMediaLibrary) Lookup(_ context.Context, workspaceID, rawURL string) (*media.Media, error) {
	return m.rows.GetMediaForWorkspaceByURL(workspaceID, rawURL)
}

func (m siteMediaLibrary) Owns(rawURL string) bool {
	_, ok := m.files.KeyFromURL(rawURL)
	return ok
}

func (m siteMediaLibrary) Fetch(ctx context.Context, rawURL string) ([]byte, string, error) {
	key, ok := m.files.KeyFromURL(rawURL)
	if !ok {
		return nil, "", fmt.Errorf("sites: %s is not a library file", rawURL)
	}
	return m.files.DownloadFile(ctx, key)
}

type siteAnalyticsProjects struct {
	storage *site_usecase.Service
}

func (p siteAnalyticsProjects) Owns(ctx context.Context, workspaceID, projectID string) error {
	if _, err := p.storage.Get(ctx, workspaceID, projectID); err != nil {
		if errors.Is(err, site_usecase.ErrProjectNotFound) {
			return siteanalytics_usecase.ErrProjectNotFound
		}
		return err
	}
	return nil
}

func siteBrandWords() []string {
	b := brand.Active()
	return []string{b.Key, b.Name, b.AIName}
}

func (c *Container) sitePublishing() *sitePublishModule {
	if c.sitePublishWiring != nil {
		return c.sitePublishWiring
	}
	sites := c.cfg.Sites
	if c.s3 == nil {
		log.Fatalf("[sites] the media library storage is required to copy media into deployments")
	}
	storage := c.siteStorage()
	hostnames := sitepublish_repository.NewHostnames(c.db)
	deployments := sitepublish_repository.NewDeployments(c.db)
	marker, err := site_usecase.NewPublishMarker(site_repository.New(c.db), nil, nil)
	if err != nil {
		log.Fatalf("[sites] publish marks: %v", err)
	}
	hosting := sitehosting.New(c.siteBucket(), c.siteDirectory())
	publisher, err := sitepublish_usecase.New(sitepublish_usecase.Deps{
		Projects:        siteProjectsAdapter{storage: storage, marker: marker},
		Workspaces:      siteWorkspaceNames{c: c},
		Builder:         siteCandidateBuilder{service: c.siteBuild()},
		Renderer:        sitePrerenderer{renderer: c.siteRenderer()},
		Forms:           c.siteFormsForPublish(),
		Hosting:         hosting,
		Media:           siteMediaLibrary{files: c.s3, rows: media_repository.NewMediaRepository(c.db)},
		CustomHostnames: customhostnames.New(sites.CloudflareZoneID, sites.CloudflareAPIToken),
		Access:          c.useCases.checkWsAccess,
		Subscriptions:   c.useCases.ensureActiveWorkspaceSubscription,
		Entitlements:    c.useCases.entitlementResolver,
		Widgets:         webchat_repository.NewWidgetRepository(c.db),
		Deployments:     deployments,
		Hostnames:       hostnames,
		Abuse:           sitepublish_repository.NewAbuseCases(c.db),
		Alerts:          c.notifications().Publisher,
		Thumbnails:      c.siteThumbnails(),
		DNS:             dnslookup.NewRecords(nil),
		Reversals:       invoice_repository.NewOpenReversals(c.db),
		Locks:           c.redisProvider.SharedState(),
		Counters:        c.redisProvider.SharedState(),
		Calendars:       c.siteBookingCalendar(),
		Now:             time.Now,
		NewID:           uuid.NewString,
	}, sitepublish_usecase.Config{
		Addresses:          siteAddresses(sites),
		APIOrigin:          sites.PublicBaseURL,
		CNAMETarget:        sites.CustomHostnameTarget,
		ApplicationDomains: siteApplicationHosts(sites),
		BrandWords:         siteBrandWords(),
	})
	if err != nil {
		log.Fatalf("[sites] publishing: %v", err)
	}
	analytics := siteanalytics_usecase.New(siteanalytics_usecase.Deps{
		Source: c.redisProvider.SiteVisits(time.Now), Store: sitepublish_repository.NewAnalytics(c.db), Access: c.useCases.checkWsAccess, Projects: siteAnalyticsProjects{storage: storage},
		Leads: siteform_repository.NewPageLeads(c.db), Deals: c.siteAnalyticsDeals(), Hostnames: hostnames, Now: time.Now,
	})
	c.sitePublishWiring = &sitePublishModule{publisher: publisher, analytics: analytics, hostnames: hostnames}
	return c.sitePublishWiring
}

func (c *Container) siteHostnameReader() *sitepublish_repository.Hostnames {
	return sitepublish_repository.NewHostnames(c.db)
}

type siteAccessHooks struct {
	c *Container
}

func (h siteAccessHooks) OnAccessCut(ctx context.Context, workspaceID string) error {
	return h.c.sitePublishing().publisher.OnAccessCut(ctx, workspaceID)
}

func (h siteAccessHooks) OnAccessRestored(ctx context.Context, workspaceID string) error {
	return h.c.sitePublishing().publisher.OnAccessRestored(ctx, workspaceID)
}

func (h siteAccessHooks) OnPaymentReversed(ctx context.Context, workspaceID string) error {
	return h.c.sitePublishing().publisher.OnPaymentReversed(ctx, workspaceID)
}

func (h siteAccessHooks) OnReversalReleased(ctx context.Context, workspaceID string) error {
	return h.c.sitePublishing().publisher.OnReversalReleased(ctx, workspaceID)
}

func (h siteAccessHooks) UnitsInUse(workspaceID string) (int, error) {
	return h.c.sitePublishing().publisher.UnitsInUse(workspaceID)
}

func (c *Container) siteAccessHooks() siteAccessHooks {
	return siteAccessHooks{c: c}
}

func sitePublishRoutes(c *Container) deliveryHttp.SitePublishRoutes {
	module := c.sitePublishing()
	return deliveryHttp.SitePublishRoutes{
		Handler: sitepublishhttp.NewHandler(module.publisher, module.analytics, siteAddresses(c.cfg.Sites)),
		Admin:   sitepublishhttp.NewAdminHandler(module.publisher),
	}
}

func (c *Container) sitePublishJobs() cronJobs {
	module := c.sitePublishing()
	return cronJobs{
		"site_hostname_sweep":   cronPackage.Executes(module.publisher),
		"site_analytics_rollup": cronPackage.Executes(module.analytics),
	}
}

type siteRetirement struct {
	c *Container
}

func (r siteRetirement) Retire(ctx context.Context, workspaceID, projectID string) error {
	return retirementError(r.c.sitePublishing().publisher.Retire(ctx, workspaceID, projectID))
}

func retirementError(err error) error {
	if errors.Is(err, sitepublish_usecase.ErrDomainBusy) {
		return fmt.Errorf("%w: %w", site_usecase.ErrProjectDomainsBusy, err)
	}
	return err
}
