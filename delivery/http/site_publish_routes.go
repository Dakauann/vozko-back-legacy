package http

import (
	"github.com/gorilla/mux"

	"vozko/delivery/http/sitepublishhttp"
)

type SitePublishRoutes struct {
	Handler *sitepublishhttp.Handler
	Admin   *sitepublishhttp.AdminHandler
}

func (r *router) setupSitePublishRoutes(protected *mux.Router) {
	sitepublishhttp.RegisterProtectedRoutes(protected, r.sitePublish.Handler, r.ac)
}

func (r *router) setupSitePublishAdminRoutes(admin *mux.Router) {
	sitepublishhttp.RegisterAdminRoutes(admin, r.sitePublish.Admin)
}
