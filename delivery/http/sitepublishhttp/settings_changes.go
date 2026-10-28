package sitepublishhttp

import (
	"net/http"

	"vozko/delivery/http/response"
	"vozko/domain/sitepublish"
)

// @Summary		Configurações que mudam na próxima publicação
// @Description	Compara as configurações atuais da página com as que foram ao ar na publicação ativa e lista o que muda ao publicar: redirecionamentos (para onde o visitante vai), integrações de rastreamento (para onde vão os dados), destino e ação de sucesso de cada formulário, aviso de cookies e SEO por rota e do site. Cada item traz action (added, removed ou changed) e os campos alterados; first true quando a página nunca foi publicada e tudo conta como novo. Exige `sites:read`.
// @Tags			sites
// @Produce		json
// @Param			id	path		string	true	"ID da página"
// @Success		200	{object}	sitepublish.SettingsChanges
// @Failure		403	{object}	response.ErrorResponse	"site_access_denied"
// @Failure		404	{object}	response.ErrorResponse	"site_not_found"
// @Failure		503	{object}	response.ErrorResponse	"publish_unavailable"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/settings-changes [get]
func (h *Handler) SettingsChanges(w http.ResponseWriter, r *http.Request) {
	a, ok := h.actor(w, r)
	if !ok {
		return
	}
	var changes sitepublish.SettingsChanges
	changes, err := h.publisher.SettingsChanges(r.Context(), a, projectID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, changes)
}
