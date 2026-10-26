package handlers

import (
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	"vozko/domain/shared/deps"
	workspace_plan "vozko/domain/workspace/workspace_plan"
	"vozko/infra/http/middleware"
)

const planDuplicateIdempotencyHeader = "Idempotency-Key"

type WorkspacePlanDuplicateDeps struct {
	Duplicate workspace_plan.DuplicatePlanDefinitionUseCase
	Preview   workspace_plan.PreviewPlanDuplicateUseCase
}

type WorkspacePlanDuplicateHandler struct {
	duplicate workspace_plan.DuplicatePlanDefinitionUseCase
	preview   workspace_plan.PreviewPlanDuplicateUseCase
}

func NewWorkspacePlanDuplicateHandler(d WorkspacePlanDuplicateDeps) (*WorkspacePlanDuplicateHandler, error) {
	if err := deps.Require("plan duplicate handler", map[string]any{
		"duplicate plan":         d.Duplicate,
		"preview plan duplicate": d.Preview,
	}); err != nil {
		return nil, err
	}
	return &WorkspacePlanDuplicateHandler{duplicate: d.Duplicate, preview: d.Preview}, nil
}

func RegisterPlanDuplicateAdminRoutes(adminRoutes *mux.Router, h *WorkspacePlanDuplicateHandler) {
	adminRoutes.HandleFunc("/admin/plans/{planId}/duplicate", h.Preview).Methods(http.MethodGet)
	adminRoutes.HandleFunc("/admin/plans/{planId}/duplicate", h.Duplicate).Methods(http.MethodPost)
}

func (h *WorkspacePlanDuplicateHandler) Preview(w http.ResponseWriter, r *http.Request) {
	preview, err := h.preview.Execute(mux.Vars(r)["planId"])
	if err != nil {
		writePlanError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, preview)
}

func (h *WorkspacePlanDuplicateHandler) Duplicate(w http.ResponseWriter, r *http.Request) {
	requestedBy := ""
	if claims := middleware.GetClaims(r); claims != nil {
		requestedBy = claims.UserID
	}
	result, err := h.duplicate.Execute(workspace_plan.DuplicatePlanInput{
		SourcePlanID:   mux.Vars(r)["planId"],
		RequestedBy:    requestedBy,
		IdempotencyKey: r.Header.Get(planDuplicateIdempotencyHeader),
	})
	if err != nil {
		writePlanError(w, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	response.WriteSuccess(w, status, result.Plan)
}
