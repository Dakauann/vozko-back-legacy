package tutorialhttp

import (
	"net/http"

	"github.com/gorilla/mux"

	workspace_domain "vozko/domain/workspace"
)

type AccessControl func(workspace_domain.Resource, workspace_domain.Action, http.HandlerFunc) http.HandlerFunc

func RegisterProtectedRoutes(protected *mux.Router, h *Handler, ac AccessControl) {
	if h == nil {
		return
	}
	protected.HandleFunc("/tutorials/progress", h.ListProgress).Methods(http.MethodGet)
	protected.HandleFunc("/tutorials/progress/{tutorialId}", h.SaveProgress).Methods(http.MethodPut)
	protected.HandleFunc("/tutorials/progress/{tutorialId}", h.ResetProgress).Methods(http.MethodDelete)
	protected.HandleFunc("/tutorials/checklists/sites", ac(workspace_domain.ResourceSites, workspace_domain.ActionRead, h.SitesChecklist)).Methods(http.MethodGet)
}
