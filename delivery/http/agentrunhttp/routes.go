package agentrunhttp

import (
	"net/http"

	"github.com/gorilla/mux"

	workspace_domain "vozko/domain/workspace"
)

func RegisterRoutes(
	protected *mux.Router,
	h *Handler,
	ac func(workspace_domain.Resource, workspace_domain.Action, http.HandlerFunc) http.HandlerFunc,
) {
	ag, read := workspace_domain.ResourceAgents, workspace_domain.ActionReadDetails
	protected.HandleFunc("/agents/{id}/runs", ac(ag, read, h.List)).Methods(http.MethodGet)
	protected.HandleFunc("/agents/{id}/runs/{runId}", ac(ag, read, h.Get)).Methods(http.MethodGet)
}
