package sitehttp

import (
	"io"
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
)

// @Summary		Ler configurações da página
// @Description	SEO por rota, integrações, formulários (funil, etapa, responsável, fluxo, canal de acompanhamento, ação de sucesso), blocos de agendamento, redirecionamentos e consentimento, com a versão que vai no If-Match.
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
// @Description	Substitui o documento de configurações inteiro a partir da versão do If-Match. Campos desconhecidos e valores fora das regras respondem 422; funil, etapa, responsável, fluxo e canal de acompanhamento precisam ser deste workspace, e um formulário com fluxo precisa de canal (form_channel_required). O fluxo precisa ser uma automação cujo gatilho é este formulário (form_workflow_not_bound; veja GET /sites/projects/{id}/forms/{formId}/automations); um formulário com funil precisa de responsável ou de fluxo (form_deal_owner_required); `site.webchat_widget` é a chave pública ou o id de um chat do site deste workspace (webchat_widget_unknown) e coloca o chat em todas as páginas. `bookings` guarda os blocos de agendamento (vz-booking) por id: duration_minutes (5 a 480, de 5 em 5), buffer_minutes (0 a 240), notice_minutes (até 30 dias), horizon_days (1 a 90), hours (fuso e janelas por dia da semana, como o horário de atendimento), title com a variável {nome}, meet, fields (email, message) e consent_text; cada regra quebrada tem seu código (booking_duration_invalid, booking_buffer_invalid, booking_notice_invalid, booking_horizon_invalid, booking_hours_invalid, booking_title_invalid, booking_consent_invalid, booking_fields_invalid). Para onde vai o lead de um bloco fica em `forms` com o mesmo id. Salvar reserva a chave pública de cada formulário e de cada bloco de agendamento, então eles já aparecem no gatilho dos fluxos antes de publicar.
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
