package container

import (
	"vozko/domain/copilot"
	"vozko/usecases/copilot/copilottools/sitetools"
)

func (c *Container) siteCopilotTools() []copilot.Tool {
	return sitetools.SiteTools(sitetools.Deps{
		Storage:     c.siteStorage(),
		Renderer:    c.siteRenderer(),
		Publisher:   c.sitePublishing().publisher,
		Media:       c.mediaGeneration().Service,
		References:  c.siteFormReferences(),
		Pins:        c.redisProvider.SharedState(),
		Inspections: c.redisProvider.SharedState(),
	})
}
