package container

import (
	"strings"
	"testing"

	"vozko/infra/config"
)

func TestTheContainerBuildsOneSiteRendererForEveryConsumer(t *testing.T) {
	c := &Container{cfg: config.Config{Sites: config.SitesConfig{
		RendererURL:            "http://siterender-gateway:9222",
		RendererToken:          strings.Repeat("r", 32),
		RendererReferenceProxy: "http://siterender-gateway:3129",
		PublicDomain:           e2ePublic,
	}}}

	first := c.siteRenderer()
	second := c.siteRenderer()

	if first == nil || first != second {
		t.Fatalf("siteRenderer returned %p then %p, want one shared instance", first, second)
	}
}

func TestTheContainerReturnsTheBuildAndStorageItAlreadyWired(t *testing.T) {
	w := newSiteWorld(t)
	c := &Container{siteShared: siteSharedWiring{build: w.build, storage: w.storage}}

	if c.siteBuild() != w.build || c.siteStorage() != w.storage {
		t.Fatalf("the container rebuilt a site service it already had")
	}
}
