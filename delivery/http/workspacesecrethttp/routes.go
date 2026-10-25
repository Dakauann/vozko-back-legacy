package workspacesecrethttp

import (
	"net/http"

	"github.com/gorilla/mux"

	workspace_domain "vozko/domain/workspace"
)

var SecretEditors = []workspace_domain.PermissionEntry{
	{Resource: workspace_domain.ResourceAgents, Action: workspace_domain.ActionUpdate},
	{Resource: workspace_domain.ResourceWorkflows, Action: workspace_domain.ActionUpdate},
}

func RegisterRoutes(
	protected *mux.Router,
	h *Handler,
	anyOf func([]workspace_domain.PermissionEntry, http.HandlerFunc) http.HandlerFunc,
) {
	for _, base := range []string{"/workspace-secrets", "/workflow-secrets"} {
		protected.HandleFunc(base, anyOf(SecretEditors, h.List)).Methods(http.MethodGet)
		protected.HandleFunc(base+"/{name}", anyOf(SecretEditors, h.Put)).Methods(http.MethodPut)
		protected.HandleFunc(base+"/{name}", anyOf(SecretEditors, h.Delete)).Methods(http.MethodDelete)
	}
}
