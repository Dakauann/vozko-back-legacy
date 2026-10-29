package http

import (
	"github.com/gorilla/mux"

	sitehttp "vozko/delivery/http/site"
)

type SiteRoutes struct {
	Storage   *sitehttp.Handler
	Captures  *sitehttp.CaptureHandler
	NextSteps *sitehttp.NextStepsHandler
}

func (r *router) setupSiteRoutes(protected *mux.Router) {
	sitehttp.RegisterProtectedRoutes(protected, r.sites.Storage, r.ac)
	sitehttp.RegisterCaptureRoutes(protected, r.sites.Captures, r.ac)
	sitehttp.RegisterNextStepsRoutes(protected, r.sites.NextSteps, r.ac)
}

func (r *router) setupSitePublicRoutes() {
	sitehttp.RegisterPublicRoutes(r.mux, r.sites.Storage)
}
