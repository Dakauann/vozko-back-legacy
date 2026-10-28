package http

import (
	"github.com/gorilla/mux"

	sitehttp "vozko/delivery/http/site"
)

type SiteRoutes struct {
	Storage  *sitehttp.Handler
	Captures *sitehttp.CaptureHandler
}

func (r *router) setupSiteRoutes(protected *mux.Router) {
	sitehttp.RegisterProtectedRoutes(protected, r.sites.Storage, r.ac)
	sitehttp.RegisterCaptureRoutes(protected, r.sites.Captures, r.ac)
}

func (r *router) setupSitePublicRoutes() {
	sitehttp.RegisterPublicRoutes(r.mux, r.sites.Storage)
}
