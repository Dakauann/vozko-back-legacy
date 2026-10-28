package sitehttp

import (
	"io"
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
)

// @Summary		Ler configurações da página
// @Description	SEO por rota, integrações, formulários (funil, etapa, responsável, fluxo, canal de acompanhamento, ação de sucesso), redirecionamentos e consentimento, com a versão que vai no If-Match.
// @Tags			Páginas
// @Produce		json
// @Param			id	path		string	true	"id da página"
// @Success		200	{object}	SettingsResponse
// @Failure		404	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id}/settings [get]
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	s, err := h.svc.GetSettings(r.Context(), workspaceID, mux.Vars(r)["id"])
	if err != nil {
		writeError(w, err, "Failed to load the settings")
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentSettings(s))
}

// @Summary		Salvar configurações da página
// @Description	Substitui o documento de configurações inteiro a partir da versão do If-Match. Campos desconhecidos e valores fora das regras respondem 422; funil, etapa, responsável, fluxo e canal de acompanhamento precisam ser deste workspace, e um formulário com fluxo precisa de canal (form_channel_required).
// @Tags			Páginas
// @Accept			json
// @Produce		json
// @Param			id			path		string			true	"id da página"
// @Param			If-Match	header		string			true	"versão das configurações"
// @Param			body		body		site.Settings	true	"configurações"
// @Success		200			{object}	SettingsResponse
// @Failure		409			{object}	httpx.VersionConflict{current=SettingsResponse}
// @Failure		413			{object}	response.ErrorResponse
// @Failure		422			{object}	response.ErrorResponse{details=FormDetails}
// @Failure		428			{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/sites/projects/{id}/settings [put]
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	version, ok := httpx.IfMatchVersion(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, settingsBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, err, "Failed to read the settings")
		return
	}
	id := mux.Vars(r)["id"]
	s, err := h.svc.PutSettings(r.Context(), workspaceID, id, version, raw)
	if isConflict(err) {
		current, getErr := h.svc.GetSettings(r.Context(), workspaceID, id)
		if getErr != nil {
			writeError(w, getErr, "Failed to load the settings")
			return
		}
		httpx.WriteVersionConflict(w, "As configurações foram salvas em outra aba ou por outra pessoa", presentSettings(current))
		return
	}
	if err != nil {
		writeError(w, err, "Failed to save the settings")
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentSettings(s))
}
