package handlers

import (
	"net/http"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
	"vozko/infra/http/middleware"
)

// @Summary		Resumo de envios do WhatsApp oficial
// @Description	Conta todo modelo que o workspace enviou a um lead: os envios de campanhas e os avulsos (o botão WhatsApp oficial do CRM, um modelo dentro de uma conversa, modelos agendados, a Elo e os fluxos). Conversas que o cliente começou nunca contam. Cada envio entra no período pelo momento em que aconteceu: o avulso quando foi enviado, o de campanha quando saiu ou falhou, e uma decisão que não envia (proteção anti-spam, número fora do WhatsApp, recusa antes do envio) quando o lead entrou na campanha. Envios de campanha anteriores aos carimbos de envio não têm data: com período eles ficam de fora e `undated` diz quantos são; sem período tudo conta pelo status atual. `bySource` separa os envios de campanhas dos avulsos. A categoria vem do modelo enviado. Membros de departamento veem as campanhas dos seus departamentos e os avulsos enviados a partir delas.
// @Tags			WhatsApp Campaigns
// @Produce		json
// @Param			from	query		string	false	"Início do período (AAAA-MM-DD)"
// @Param			to		query		string	false	"Fim do período (AAAA-MM-DD, inclusive)"
// @Success		200		{object}	campaign.SendsSummary
// @Failure		401		{object}	response.ErrorResponse
// @Failure		403		{object}	response.ErrorResponse
// @Failure		500		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/whatsapp/campaigns/summary [get]
func (h *WhatsAppCampaignHandler) Summary(w http.ResponseWriter, r *http.Request) {
	if middleware.GetClaims(r) == nil {
		response.WriteError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	if shouldReturnEmptyDepartmentList(r) {
		response.WriteSuccess(w, http.StatusOK, campaign.NewSendsSummary(campaign.SendsTally{}))
		return
	}

	values := r.URL.Query()
	summary, err := h.summaryUseCase.Execute(wce.WorkspaceSummaryFilter{
		WorkspaceID:   middleware.GetWorkspaceID(r),
		DepartmentIDs: departmentFilterIDs(r),
		Window: campaign.SendWindow{
			From: httpx.ParseDateBound(values.Get("from"), false),
			To:   httpx.ParseDateBound(values.Get("to"), true),
		},
	})
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "Failed to load whatsapp campaigns summary", nil)
		return
	}
	response.WriteSuccess(w, http.StatusOK, summary)
}
