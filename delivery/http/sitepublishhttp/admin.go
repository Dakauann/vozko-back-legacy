package sitepublishhttp

import (
	"context"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/sitepublish"
	"vozko/infra/http/middleware"
	sitepublish_usecase "vozko/usecases/sitepublish"
)

type Moderator interface {
	AdminCases(ctx context.Context, statuses []sitepublish.AbuseStatus) ([]sitepublish_usecase.AdminCase, error)
	AdminSuspend(ctx context.Context, adminID, projectID, reason string) (sitepublish.AbuseCase, error)
	AdminRestore(ctx context.Context, adminID, projectID, reason string) ([]sitepublish.AbuseCase, error)
}

type AdminHandler struct {
	moderator Moderator
}

func NewAdminHandler(moderator Moderator) *AdminHandler {
	return &AdminHandler{moderator: moderator}
}

func adminID(r *http.Request) string {
	if claims := middleware.GetClaims(r); claims != nil {
		return claims.UserID
	}
	return ""
}

func (h *AdminHandler) List(w http.ResponseWriter, r *http.Request) {
	var statuses []sitepublish.AbuseStatus
	for _, raw := range strings.Split(r.URL.Query().Get("status"), ",") {
		if s := sitepublish.AbuseStatus(strings.TrimSpace(raw)); s != "" {
			statuses = append(statuses, s)
		}
	}
	cases, err := h.moderator.AdminCases(r.Context(), statuses)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]AbuseCaseDTO, 0, len(cases))
	for _, c := range cases {
		dto := abuseCaseDTO(c.AbuseCase)
		dto.ProjectName, dto.WorkspaceName = c.ProjectName, c.WorkspaceName
		out = append(out, dto)
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *AdminHandler) Suspend(w http.ResponseWriter, r *http.Request) {
	var body AdminReasonRequest
	if !httpx.DecodeStrictJSON(w, r, &body) {
		return
	}
	c, err := h.moderator.AdminSuspend(r.Context(), adminID(r), mux.Vars(r)["projectId"], body.Reason)
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, abuseCaseDTO(c))
}

func (h *AdminHandler) Restore(w http.ResponseWriter, r *http.Request) {
	var body AdminReasonRequest
	if !httpx.DecodeStrictJSON(w, r, &body) {
		return
	}
	cases, err := h.moderator.AdminRestore(r.Context(), adminID(r), mux.Vars(r)["projectId"], body.Reason)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]AbuseCaseDTO, 0, len(cases))
	for _, c := range cases {
		out = append(out, abuseCaseDTO(c))
	}
	response.WriteSuccess(w, http.StatusOK, out)
}
