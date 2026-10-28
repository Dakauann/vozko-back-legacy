package container

import (
	"context"
	"log"

	deliveryHttp "vozko/delivery/http"
	sitehttp "vozko/delivery/http/site"
	"vozko/domain/sitepublish"
	emailinbox_repository "vozko/infra/repositories/emailinbox"
	site_repository "vozko/infra/repositories/site"
	sitepublish_repository "vozko/infra/repositories/sitepublish"
	uwrepo "vozko/infra/repositories/unofficial_whatsapp"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/sitebuild"
	"vozko/usecases/siteforms"
	"vozko/usecases/siteinspect"
)

func (c *Container) siteFormReferences() *site_usecase.WorkspaceReferences {
	refs, err := site_usecase.NewWorkspaceReferences(site_usecase.ReferenceDeps{
		Pipelines:   c.repositories.pipeline,
		Stages:      c.repositories.stage,
		Members:     c.repositories.workspace,
		Departments: c.repositories.workspaceDepartment,
		Workflows:   c.repositories.workflow,
		Phones:      c.repositories.businessPhone,
		PhoneList:   c.useCases.workspacePhones,
		Instances:   uwrepo.NewInstanceRepository(c.db),
		Inboxes:     emailinbox_repository.NewInboxRepository(c.db),
	})
	if err != nil {
		log.Fatalf("[sites] form settings need the pipeline, stage, member, department, workflow, phone, instance and inbox repositories: %v", err)
	}
	return refs
}

func (c *Container) siteStorage() *site_usecase.Service {
	if c.siteShared.storage != nil {
		return c.siteShared.storage
	}
	builder := siteBuildEditor{service: c.siteBuild()}
	service, err := site_usecase.NewService(site_usecase.Deps{
		Store:      site_repository.New(c.db),
		Executor:   builder,
		Previewer:  builder,
		References: c.siteFormReferences(),
		Forms:      c.siteFormsForPublish(),
		Retirement: siteRetirement{c: c},
		Published:  sitePublishedRevisions{deployments: sitepublish_repository.NewDeployments(c.db)},
		Media:      sitebuild.MediaFromRepository(c.repositories.media),
		Numbers:    c.siteWhatsAppNumbers(),
		Hostnames:  c.siteHostnameReader(),
		Previews:   c.redisProvider.SharedState(),
		Origins:    siteOrigins(c.cfg.Sites),
	})
	if err != nil {
		log.Fatalf("[sites] %v", err)
	}
	c.siteShared.storage = service
	return service
}

func (c *Container) siteWhatsAppNumbers() siteforms.PageNumbers {
	return siteforms.PageNumbers{Phones: c.repositories.businessPhone, Grants: c.repositories.workspacePhoneAccess, Instances: c.unofficialWhatsApp.Instances}
}

type sitePublishedRevisions struct {
	deployments interface {
		Get(ctx context.Context, workspaceID, id string) (sitepublish.DeploymentRecord, error)
	}
}

func (p sitePublishedRevisions) PublishedRevision(ctx context.Context, workspaceID, deploymentID string) (string, error) {
	rec, err := p.deployments.Get(ctx, workspaceID, deploymentID)
	if err != nil {
		return "", err
	}
	return rec.RevisionID, nil
}

func siteRoutes(c *Container) deliveryHttp.SiteRoutes {
	storage := c.siteStorage()
	inspector := siteinspect.Service{Previews: storage, Cache: c.redisProvider.SharedState()}
	outlines := siteinspect.Outlines{Inspector: inspector, State: c.redisProvider.SharedState()}
	return deliveryHttp.SiteRoutes{
		Storage:  sitehttp.NewHandler(storage, outlines, c.siteSections(), c.useCases.checkWsAccess, apiOrigin(c.cfg.Sites)),
		Captures: sitehttp.NewCaptureHandler(storage, outlines, c.siteThumbnails()),
	}
}
