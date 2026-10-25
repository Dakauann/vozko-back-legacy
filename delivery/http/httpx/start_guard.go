package httpx

import (
	"net/http"

	"vozko/delivery/http/response"
	"vozko/domain/workspace"
)

type Guard func(http.HandlerFunc) http.HandlerFunc

func WithStart(guard Guard, w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if guard == nil {
		response.WriteErrorWithCode(w, http.StatusForbidden, workspace.PermissionRequiredCode(workspace.ActionStart), "You cannot start or schedule campaigns", nil)
		return
	}
	guard(next)(w, r)
}
