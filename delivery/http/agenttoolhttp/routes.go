package agenttoolhttp

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
	protected.HandleFunc("/agents/{id}/tools", ac(ag, read, h.List)).Methods(http.MethodGet)
	protected.HandleFunc("/agents/{id}/tools", ac(ag, update, h.Create)).Methods(http.MethodPost)
	protected.HandleFunc("/agents/{id}/tools/order", ac(ag, update, h.Reorder)).Methods(http.MethodPut)
	protected.HandleFunc("/agents/tools/mcp-sources", ac(ag, read, h.MCPSources)).Methods(http.MethodGet)
	protected.HandleFunc("/agents/{id}/tools/validate", ac(ag, update, h.Validate)).Methods(http.MethodPost)
	protected.HandleFunc("/agents/{id}/tools/test", ac(ag, update, h.TestDraft)).Methods(http.MethodPost)
	protected.HandleFunc("/agents/{id}/tools/{toolId}/test", ac(ag, update, h.Test)).Methods(http.MethodPost)
	protected.HandleFunc("/agents/{id}/tools/{toolId}/test-inputs", ac(ag, update, h.SaveTestInputs)).Methods(http.MethodPut)
	protected.HandleFunc("/agents/{id}/tools/{toolId}", ac(ag, update, h.Update)).Methods(http.MethodPut)
	protected.HandleFunc("/agents/{id}/tools/{toolId}", ac(ag, update, h.Delete)).Methods(http.MethodDelete)
	protected.HandleFunc("/agents/{id}/tools/{toolId}/duplicate", ac(ag, update, h.Duplicate)).Methods(http.MethodPost)
	protected.HandleFunc("/agents/{id}/tools/{toolId}/revisions", ac(ag, read, h.Revisions)).Methods(http.MethodGet)
	protected.HandleFunc("/agents/{id}/tools/{toolId}/revisions/{revision}", ac(ag, read, h.Revision)).Methods(http.MethodGet)
}
