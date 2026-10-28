package container

import (
	"log"

	"vozko/infra/netguard"
	siterenderinfra "vozko/infra/siterender"
	"vozko/infra/sitehosting"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/sitebuild"
	"vozko/usecases/siterender"
	siteserve_usecase "vozko/usecases/siteserve"
)

type siteSharedWiring struct {
	renderer  *siterender.Service
	build     *sitebuild.Service
	storage   *site_usecase.Service
	directory *sitehosting.Directory
	pages     *siteserve_usecase.Service
}

func (c *Container) siteRenderer() *siterender.Service {
	if c.siteShared.renderer != nil {
		return c.siteShared.renderer
	}
	sites := c.cfg.Sites
	browser, err := siterenderinfra.New(siterenderinfra.Config{
		Endpoint:       sites.RendererURL,
		Token:          sites.RendererToken,
		ReferenceProxy: sites.RendererReferenceProxy,
	})
	if err != nil {
		log.Fatalf("[sites] the renderer: %v", err)
	}
	c.siteShared.renderer = siterender.NewService(browser, netguard.NewHostChecker(nil), sites.PublicDomain, siterender.DefaultConcurrency)
	return c.siteShared.renderer
}
