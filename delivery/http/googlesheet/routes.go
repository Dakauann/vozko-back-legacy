package googlesheet

import (
	"net/http"

	"github.com/gorilla/mux"

	workspace_domain "vozko/domain/workspace"
)

func RegisterProtectedRoutes(
	protected *mux.Router,
	h *Handler,
	ac func(workspace_domain.Resource, workspace_domain.Action, http.HandlerFunc) http.HandlerFunc,
) {
	res := workspace_domain.ResourceGoogleSheets
	protected.HandleFunc("/google-sheets/catalog", ac(res, workspace_domain.ActionRead, h.Catalog)).Methods(http.MethodGet)
	protected.HandleFunc("/google-sheets", ac(res, workspace_domain.ActionRead, h.List)).Methods(http.MethodGet)
	protected.HandleFunc("/google-sheets", ac(res, workspace_domain.ActionCreate, h.Create)).Methods(http.MethodPost)
	protected.HandleFunc("/google-sheets/presets", ac(res, workspace_domain.ActionRead, h.Presets)).Methods(http.MethodGet)
	protected.HandleFunc("/google-sheets/spreadsheets", ac(res, workspace_domain.ActionRead, h.Spreadsheets)).Methods(http.MethodGet)
	protected.HandleFunc("/google-sheets/files", ac(res, workspace_domain.ActionRead, h.Files)).Methods(http.MethodGet)
	protected.HandleFunc("/google-sheets/files/{spreadsheetId}", ac(res, workspace_domain.ActionUpdate, h.RenameFile)).Methods(http.MethodPatch)
	protected.HandleFunc("/google-sheets/workbook", ac(res, workspace_domain.ActionCreate, h.CreateWorkbook)).Methods(http.MethodPost)
	protected.HandleFunc("/google-sheets/tab-name-preview", ac(res, workspace_domain.ActionRead, h.PreviewTabName)).Methods(http.MethodPost)
	protected.HandleFunc("/google-sheets/{id}/layout", ac(res, workspace_domain.ActionUpdate, h.UpdateLayout)).Methods(http.MethodPatch)
	protected.HandleFunc("/google-sheets/{id}", ac(res, workspace_domain.ActionRead, h.Get)).Methods(http.MethodGet)
	protected.HandleFunc("/google-sheets/{id}", ac(res, workspace_domain.ActionUpdate, h.UpdateDetails)).Methods(http.MethodPatch)
	protected.HandleFunc("/google-sheets/{id}", ac(res, workspace_domain.ActionDelete, h.Delete)).Methods(http.MethodDelete)
	protected.HandleFunc("/google-sheets/{id}/columns", ac(res, workspace_domain.ActionUpdate, h.EditColumns)).Methods(http.MethodPatch)
	protected.HandleFunc("/google-sheets/{id}/health", ac(res, workspace_domain.ActionRead, h.Health)).Methods(http.MethodGet)
	protected.HandleFunc("/google-sheets/{id}/retry", ac(res, workspace_domain.ActionUpdate, h.Retry)).Methods(http.MethodPost)
	protected.HandleFunc("/google-sheets/{id}/reactivate", ac(res, workspace_domain.ActionUpdate, h.Reactivate)).Methods(http.MethodPost)
	protected.HandleFunc("/google-sheets/{id}/rows", ac(res, workspace_domain.ActionUpdate, h.AddRow)).Methods(http.MethodPost)
	protected.HandleFunc("/google-sheets/{id}/rows", ac(res, workspace_domain.ActionRead, h.ListRows)).Methods(http.MethodGet)
}

func RegisterEditSessionRoutes(
	protected *mux.Router,
	h *EditSessionHandler,
	ac func(workspace_domain.Resource, workspace_domain.Action, http.HandlerFunc) http.HandlerFunc,
) {
	res := workspace_domain.ResourceGoogleSheets
	protected.HandleFunc("/google-sheets/files/{spreadsheetId}/edit-session", ac(res, workspace_domain.ActionRead, h.Editor)).Methods(http.MethodGet)
	protected.HandleFunc("/google-sheets/files/{spreadsheetId}/edit-session", ac(res, workspace_domain.ActionUpdate, h.Start)).Methods(http.MethodPost)
	protected.HandleFunc("/google-sheets/files/{spreadsheetId}/edit-sessions", ac(res, workspace_domain.ActionRead, h.History)).Methods(http.MethodGet)
	protected.HandleFunc("/google-sheets/edit-sessions/{sessionId}/heartbeat", ac(res, workspace_domain.ActionUpdate, h.Heartbeat)).Methods(http.MethodPost)
	protected.HandleFunc("/google-sheets/edit-sessions/{sessionId}/end", ac(res, workspace_domain.ActionUpdate, h.End)).Methods(http.MethodPost)
	protected.HandleFunc("/google-sheets/edit-sessions/{sessionId}/force-end", ac(res, workspace_domain.ActionDelete, h.ForceEnd)).Methods(http.MethodPost)
}

func RegisterRowRemovalRoutes(
	protected *mux.Router,
	h *RowRemovalHandler,
	ac func(workspace_domain.Resource, workspace_domain.Action, http.HandlerFunc) http.HandlerFunc,
) {
	res := workspace_domain.ResourceGoogleSheets
	protected.HandleFunc("/google-sheets/{id}/rows/{rowId}/cancel", ac(res, workspace_domain.ActionUpdate, h.Cancel)).Methods(http.MethodPost)
	protected.HandleFunc("/google-sheets/{id}/rows/{rowId}/remove", ac(res, workspace_domain.ActionUpdate, h.Remove)).Methods(http.MethodPost)
	protected.HandleFunc("/google-sheets/{id}/removals", ac(res, workspace_domain.ActionRead, h.List)).Methods(http.MethodGet)
}

func RegisterModelFitRoutes(
	protected *mux.Router,
	h *ModelFitHandler,
	ac func(workspace_domain.Resource, workspace_domain.Action, http.HandlerFunc) http.HandlerFunc,
) {
	res := workspace_domain.ResourceGoogleSheets
	protected.HandleFunc("/google-sheets/files/{spreadsheetId}/fit", ac(res, workspace_domain.ActionRead, h.Fit)).Methods(http.MethodPost)
	protected.HandleFunc("/google-sheets/files/{spreadsheetId}/copies", ac(res, workspace_domain.ActionCreate, h.Copy)).Methods(http.MethodPost)
}
