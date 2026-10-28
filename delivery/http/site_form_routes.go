package http

import (
	"github.com/gorilla/mux"

	"vozko/delivery/http/siteformhttp"
)

type SiteFormRoutes struct {
	Public      *siteformhttp.PublicHandler
	Submissions *siteformhttp.Handler
}

func (r *router) setupSiteFormRoutes(protected *mux.Router) {
	siteformhttp.RegisterProtectedRoutes(protected, r.siteForms.Submissions, r.ac)
}

func (r *router) setupSiteFormPublicRoutes() {
	siteformhttp.RegisterPublicRoutes(r.mux, r.siteForms.Public)
}
