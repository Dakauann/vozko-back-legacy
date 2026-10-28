package container

import (
	"context"
	"log"
	"time"

	"vozko/domain/opportunity"
	"vozko/domain/shared"
	"vozko/domain/site"
	opportunity_repository "vozko/infra/repositories/opportunity"
	site_repository "vozko/infra/repositories/site"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/siterender"
	"vozko/usecases/sitethumbnail"
)

func (c *Container) siteThumbnails() *sitethumbnail.Service {
	if c.siteThumbnailWiring != nil {
		return c.siteThumbnailWiring
	}
	service, err := sitethumbnail.New(sitethumbnail.Deps{
		Renderer: c.siteRenderer(),
		Uploader: c.useCases.uploadMedia,
		Projects: site_repository.New(c.db),
		Media:    c.repositories.media,
		Files:    c.s3,
		Throttle: c.redisProvider.SharedState(),
	})
	if err != nil {
		log.Fatalf("[sites] thumbnails: %v", err)
	}
	c.siteThumbnailWiring = service
	return service
}

type siteSnippetSource struct {
	store site_usecase.DesignSystemStore
}

func (s siteSnippetSource) Snippets(ctx context.Context, workspaceID string) ([]site.DesignSnippet, error) {
	return site_usecase.SnippetsOf(ctx, s.store, workspaceID)
}

func (p sitePrerenderer) OpenGraph(ctx context.Context, url string) ([]byte, error) {
	return p.renderer.Screenshot(ctx, url, siterender.OpenGraph)
}

type siteAnalyticsDeals struct {
	people opportunity.PersonDealsUseCase
	wins   opportunity.WonOfLeadsReader
}

func (d siteAnalyticsDeals) Scope(by shared.Person, workspaceID string) (opportunity.DealScope, error) {
	return d.people.Scope(by, workspaceID)
}

func (d siteAnalyticsDeals) WonDealsOfLeads(ctx context.Context, workspaceID string, leadIDs []string, from, to time.Time, scope opportunity.DealScope) ([]opportunity.WonTotal, error) {
	return d.wins.WonDealsOfLeads(ctx, workspaceID, leadIDs, from, to, scope)
}

func (c *Container) siteAnalyticsDeals() siteAnalyticsDeals {
	if c.useCases.personDeals == nil {
		log.Fatalf("[sites] analytics: the deal scope is required")
	}
	return siteAnalyticsDeals{people: c.useCases.personDeals, wins: opportunity_repository.NewWonOfLeadsReader(c.db)}
}
