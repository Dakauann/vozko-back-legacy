package container

import (
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"time"

	"vozko/delivery/http/sitehost"
	"vozko/domain/hostname"
	"vozko/domain/sitepublish"
	"vozko/infra/config"
	sitepublish_repository "vozko/infra/repositories/sitepublish"
	"vozko/infra/s3"
	"vozko/infra/sitehosting"
	siteserve_usecase "vozko/usecases/siteserve"
)

const (
	siteBeaconsPerMinute   = 60
	sitePasswordsPerMinute = 10
)

func siteApplicationHosts(sites config.SitesConfig) []string {
	u, err := url.Parse(sites.PublicBaseURL)
	if err != nil || u.Hostname() == "" {
		log.Fatalf("[sites] API_BASE_URL is not an absolute url")
	}
	hosts := slices.Clone(sites.ApplicationDomains)
	if api := hostname.Normalize(u.Hostname()); !slices.Contains(hosts, api) {
		hosts = append(hosts, api)
	}
	return hosts
}

func (c *Container) siteDirectory() *sitehosting.Directory {
	if c.siteShared.directory == nil {
		c.siteShared.directory = sitehosting.NewDirectory(sitepublish_repository.NewServingKeys(c.db, time.Now), time.Now)
	}
	return c.siteShared.directory
}

func (c *Container) siteBucket() *s3.S3Service {
	return s3.NewS3ServiceForBucket(c.cfg.Sites.R2Bucket)
}

func (c *Container) sitePages() *siteserve_usecase.Service {
	if c.siteShared.pages != nil {
		return c.siteShared.pages
	}
	sites := c.cfg.Sites
	limit := c.services.rateLimiterFactory
	pages, err := siteserve_usecase.New(siteserve_usecase.Deps{
		Directory: c.siteDirectory(),
		Files:     sitehosting.NewFiles(c.siteBucket()),
		Visits:    c.redisProvider.SiteVisits(time.Now),
		Beacons:   limit("site_beacon_ip", siteBeaconsPerMinute, time.Minute),
		Passwords: limit("site_password_ip_host", sitePasswordsPerMinute, time.Minute),
		Now:       time.Now,
		Roll:      rand.IntN,
	}, siteserve_usecase.Config{
		Addresses:       siteAddresses(sites),
		PlatformDomains: sitepublish.PlatformDomains(sites.PublicDomain, siteApplicationHosts(sites)),
		CookieSecret:    []byte(sites.CookieSecret),
	})
	if err != nil {
		log.Fatalf("[sites] page server: %v", err)
	}
	c.siteShared.pages = pages
	return pages
}

func (c *Container) sitePageHost(next http.Handler) http.Handler {
	handler, err := sitehost.NewHandler(c.sitePages())
	if err != nil {
		log.Fatalf("[sites] %v", err)
	}
	return sitehost.Dispatch(c.sitePages(), handler, next)
}

func siteOrigins(sites config.SitesConfig) sitepublish.Origins {
	return sitepublish.Origins{LoopbackPort: sites.APIPort}
}

func siteAddresses(sites config.SitesConfig) sitepublish.Addresses {
	return sitepublish.Addresses{Domain: sites.PublicDomain, Origins: siteOrigins(sites)}
}
