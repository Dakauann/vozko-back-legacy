package personalinboxhttp

import (
	"net/http"

	"github.com/gorilla/mux"
)

func RegisterProtectedRoutes(protected *mux.Router, h *Handler) {
	protected.HandleFunc("/inbox/preferences", h.Preferences).Methods(http.MethodGet)
	protected.HandleFunc("/inbox/preferences", h.SavePreferences).Methods(http.MethodPut)
}
