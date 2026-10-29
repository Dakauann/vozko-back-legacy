package webchat

import (
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	"vozko/infra/http/middleware"
)

type WidgetUsageResponse struct {
	SiteOrigins    []string `json:"siteOrigins" example:"https://ofertas.vozko.app"`
	AllowedOrigins []string `json:"allowedOrigins" example:"https://www.loja.com.br"`
}

// @Summary		Onde o WebChat aparece
// @Description	As origens das páginas publicadas que mostram este chat (siteOrigins) e os sites externos onde ele pode abrir (allowedOrigins), para avisar antes de mudar a aparência de um chat usado em mais de um lugar. Uma falha ao ler as páginas responde erro, nunca uma lista vazia.
// @Tags			WebChat
// @Produce		json
// @Param			id	path		string	true	"id do chat"
// @Success		200	{object}	WidgetUsageResponse
// @Failure		404	{object}	response.CodedErrorResponse
// @Security		BearerAuth
// @Router			/webchat/widgets/{id}/usage [get]
func (h *Handler) Usage(w http.ResponseWriter, r *http.Request) {
	usage, err := h.usage.Of(r.Context(), middleware.GetWorkspaceID(r), mux.Vars(r)["id"])
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, WidgetUsageResponse{SiteOrigins: usage.SiteOrigins, AllowedOrigins: usage.AllowedOrigins})
}
