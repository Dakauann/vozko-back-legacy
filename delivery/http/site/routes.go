package sitehttp

import (
	"bytes"
	"errors"
	"net/http"

	"github.com/gorilla/mux"

	"vozko/domain/shared"
	workspace_domain "vozko/domain/workspace"
)

type AccessControl func(workspace_domain.Resource, workspace_domain.Action, http.HandlerFunc) http.HandlerFunc

func RegisterProtectedRoutes(protected *mux.Router, h *Handler, ac AccessControl) {
	if h == nil {
		return
	}
	sites := workspace_domain.ResourceSites
	read, create, update, remove := workspace_domain.ActionRead, workspace_domain.ActionCreate, workspace_domain.ActionUpdate, workspace_domain.ActionDelete
	protected.HandleFunc("/sites/starters", ac(sites, read, h.Starters)).Methods(http.MethodGet)
	protected.HandleFunc("/sites/sections", ac(sites, read, h.Sections)).Methods(http.MethodGet)
	protected.HandleFunc("/sites/projects", ac(sites, read, h.List)).Methods(http.MethodGet)
	protected.HandleFunc("/sites/projects", ac(sites, create, h.Create)).Methods(http.MethodPost)
	protected.HandleFunc("/sites/projects/{id}", ac(sites, read, h.Get)).Methods(http.MethodGet)
	protected.HandleFunc("/sites/projects/{id}/duplicate", ac(sites, create, h.Duplicate)).Methods(http.MethodPost)
	protected.HandleFunc("/sites/projects/{id}", ac(sites, update, h.Update)).Methods(http.MethodPatch)
	protected.HandleFunc("/sites/projects/{id}", ac(sites, remove, h.Delete)).Methods(http.MethodDelete)
	protected.HandleFunc("/sites/projects/{id}/files", ac(sites, read, h.Files)).Methods(http.MethodGet)
	protected.HandleFunc("/sites/projects/{id}/files", ac(sites, update, h.SaveFiles)).Methods(http.MethodPut)
	protected.HandleFunc("/sites/projects/{id}/edits", ac(sites, update, h.Edit)).Methods(http.MethodPost)
	protected.HandleFunc("/sites/projects/{id}/revisions", ac(sites, read, h.Revisions)).Methods(http.MethodGet)
	protected.HandleFunc("/sites/projects/{id}/revisions", ac(sites, update, h.SaveVersion)).Methods(http.MethodPost)
	protected.HandleFunc("/sites/projects/{id}/revisions/{rev}/restore", ac(sites, update, h.Restore)).Methods(http.MethodPost)
	protected.HandleFunc("/sites/projects/{id}/settings", ac(sites, read, h.Settings)).Methods(http.MethodGet)
	protected.HandleFunc("/sites/projects/{id}/settings", ac(sites, update, h.PutSettings)).Methods(http.MethodPut)
	protected.HandleFunc("/sites/projects/{id}/preview", ac(sites, read, h.Preview)).Methods(http.MethodPost)
	protected.HandleFunc("/sites/projects/{id}/outline", ac(sites, read, h.Outline)).Methods(http.MethodGet)
	protected.HandleFunc("/sites/projects/{id}/export", ac(sites, read, h.Export)).Methods(http.MethodGet)
	protected.HandleFunc("/sites/projects/{id}/snippets", ac(sites, update, h.SaveSnippet)).Methods(http.MethodPost)
	protected.HandleFunc("/sites/design-systems", ac(sites, read, h.DesignSystems)).Methods(http.MethodGet)
	protected.HandleFunc("/sites/design-systems", ac(sites, create, h.CreateDesignSystem)).Methods(http.MethodPost)
	protected.HandleFunc("/sites/design-systems/{id}", ac(sites, read, h.DesignSystem)).Methods(http.MethodGet)
	protected.HandleFunc("/sites/design-systems/{id}", ac(sites, update, h.UpdateDesignSystem)).Methods(http.MethodPut)
	protected.HandleFunc("/sites/design-systems/{id}", ac(sites, remove, h.DeleteDesignSystem)).Methods(http.MethodDelete)
}

func isConflict(err error) bool {
	return errors.Is(err, shared.ErrVersionConflict)
}

func bytesReader(raw []byte) *bytes.Reader {
	return bytes.NewReader(raw)
}
