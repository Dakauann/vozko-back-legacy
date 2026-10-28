package sitepublishhttp

import (
	"net/http"

	"github.com/gorilla/mux"

	workspace_domain "vozko/domain/workspace"
	"vozko/usecases/sitepublish"
)

type accessFunc func(workspace_domain.Resource, workspace_domain.Action, http.HandlerFunc) http.HandlerFunc

func RegisterProtectedRoutes(protected *mux.Router, h *Handler, ac accessFunc) {
	if h == nil {
		return
	}
	sites, domains := sitepublish_usecase.ResourceSites, sitepublish_usecase.ResourceSiteDomains
	read, update, publish := workspace_domain.ActionRead, workspace_domain.ActionUpdate, sitepublish_usecase.ActionPublish
	base := "/sites/projects/{id}"
	protected.HandleFunc(base+"/deployments", ac(sites, read, h.ListDeployments)).Methods(http.MethodGet)
	protected.HandleFunc(base+"/deployments", ac(sites, publish, h.Publish)).Methods(http.MethodPost)
	protected.HandleFunc(base+"/deployments/{deploymentId}/activate", ac(sites, publish, h.Activate)).Methods(http.MethodPost)
	protected.HandleFunc(base+"/settings-changes", ac(sites, read, h.SettingsChanges)).Methods(http.MethodGet)
	protected.HandleFunc(base+"/unpublish", ac(sites, publish, h.Unpublish)).Methods(http.MethodPost)
	protected.HandleFunc(base+"/hostnames", ac(domains, read, h.ListHostnames)).Methods(http.MethodGet)
	protected.HandleFunc(base+"/hostnames", ac(domains, update, h.AddHostname)).Methods(http.MethodPost)
	protected.HandleFunc(base+"/hostnames/{hostnameId}", ac(domains, update, h.RemoveHostname)).Methods(http.MethodDelete)
	protected.HandleFunc(base+"/hostnames/{hostnameId}/verify", ac(domains, update, h.VerifyHostname)).Methods(http.MethodPost)
	protected.HandleFunc("/sites/domain-slots", ac(domains, read, h.DomainSlots)).Methods(http.MethodGet)
	protected.HandleFunc(base+"/analytics", ac(sites, read, h.Analytics)).Methods(http.MethodGet)
}

func RegisterAdminRoutes(admin *mux.Router, h *AdminHandler) {
	if h == nil {
		return
	}
	admin.HandleFunc("/admin/sites", h.List).Methods(http.MethodGet)
	admin.HandleFunc("/admin/sites/{projectId}/suspend", h.Suspend).Methods(http.MethodPost)
	admin.HandleFunc("/admin/sites/{projectId}/restore", h.Restore).Methods(http.MethodPost)
}
