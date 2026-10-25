package whatsapptemplate

import (
	"context"
	"net/http"

	whatsapptemplatedomain "vozko/domain/whatsapp/template"
	"vozko/infra/http/middleware"
)

type ButtonTracking interface {
	Apply(ctx context.Context, workspaceID, userID string, components []whatsapptemplatedomain.TemplateComponent, tracked []int) ([]whatsapptemplatedomain.TemplateComponent, error)
}

func (h *WhatsAppTemplateHandler) trackButtons(w http.ResponseWriter, r *http.Request, userID string, components []whatsapptemplatedomain.TemplateComponent, tracked []int) ([]whatsapptemplatedomain.TemplateComponent, bool) {
	if tracked == nil {
		return components, true
	}
	out, err := h.tracking.Apply(r.Context(), middleware.GetWorkspaceID(r), userID, components, tracked)
	if err != nil {
		writeTemplateError(w, err)
		return nil, false
	}
	return out, true
}
