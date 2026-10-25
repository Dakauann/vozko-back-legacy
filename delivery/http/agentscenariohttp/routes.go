package agentscenariohttp

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
	ag := workspace_domain.ResourceAgents
	read, update := workspace_domain.ActionReadDetails, workspace_domain.ActionUpdate
	protected.HandleFunc("/agents/{id}/scenarios", ac(ag, read, h.List)).Methods(http.MethodGet)
	protected.HandleFunc("/agents/{id}/scenarios", ac(ag, update, h.Create)).Methods(http.MethodPost)
	protected.HandleFunc("/agents/{id}/scenarios/run", ac(ag, update, h.RunAll)).Methods(http.MethodPost)
	protected.HandleFunc("/agents/{id}/scenarios/{scenarioId}", ac(ag, update, h.Update)).Methods(http.MethodPut)
	protected.HandleFunc("/agents/{id}/scenarios/{scenarioId}", ac(ag, update, h.Delete)).Methods(http.MethodDelete)
	protected.HandleFunc("/agents/{id}/scenarios/{scenarioId}/runs", ac(ag, read, h.Runs)).Methods(http.MethodGet)
	protected.HandleFunc("/agents/{id}/scenarios/{scenarioId}/run", ac(ag, update, h.Run)).Methods(http.MethodPost)
	protected.HandleFunc("/agents/{id}/runs/{runId}/scenario", ac(ag, update, h.FromRun)).Methods(http.MethodPost)
}
