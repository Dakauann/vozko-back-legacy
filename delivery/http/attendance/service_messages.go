package attendance

import (
	"errors"
	"net/http"

	"vozko/delivery/http/response"
	attendancedomain "vozko/domain/attendance"
	"vozko/infra/http/middleware"
)

const (
	CodeServiceMessagesNotOnPlan        = "service_messages_not_on_plan"
	CodeServiceMessagesPriceUnavailable = "service_messages_price_unavailable"
	CodeServiceMessagesPeriodInvalid    = "service_messages_period_invalid"
	CodeDepartmentForbidden             = "attendance_department_forbidden"
)

// @Summary		Seções de atendimento disponíveis
// @Description	Lista as seções da visão geral de atendimento que este workspace pode ler. A seção service_messages só aparece quando o plano do workspace cobra mensagens de serviço do WhatsApp oficial (preço resolvido como na cobrança: item do plano, senão o padrão de Tarifas, acima de zero). Se o preço não puder ser resolvido, ela não aparece. Nenhum dado de mensagens é lido aqui.
// @Tags			Atendimento
// @Produce		json
// @Success		200	{object}	attendance.AvailableSectionsResponse
// @Failure		400	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/attendance/sections [get]
func (h *AttendanceHandler) GetAvailableSections(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.GetWorkspaceID(r)
	if wsID == "" {
		response.WriteError(w, http.StatusBadRequest, "workspace_id required", nil)
		return
	}
	response.WriteSuccess(w, http.StatusOK, attendancedomain.AvailableSectionsResponse{
		Sections: h.serviceMessages.AvailableSections(r.Context(), wsID),
	})
}

// @Summary		Mensagens de serviço do WhatsApp oficial
// @Description	Conta as mensagens de serviço do WhatsApp oficial que a Meta cobrou (marcadas como cobráveis na categoria service, fora da janela gratuita de anúncio) no período, com a série por dia (até 92 dias), por semana (até 371 dias) ou por mês, e o valor estimado em reais ao preço atual do plano. Só responde para workspaces cujo plano cobra mensagens de serviço; os demais recebem 403 service_messages_not_on_plan, e um preço que não pode ser resolvido responde 503 service_messages_price_unavailable. Respeita o escopo de departamentos de quem pede (403 attendance_department_forbidden fora dele) e os filtros de departamento, membro, campanha, canal e origem. Com outro canal no filtro, applicable vem false e o total vem nulo. Os dias são contados em UTC, como nas demais seções. A contagem fica em cache por 60 segundos; quando o limite de consultas analíticas simultâneas está ocupado, responde 503 com Retry-After.
// @Tags			Atendimento
// @Produce		json
// @Param			date_from		query	string	true	"Data inicial (YYYY-MM-DD)"
// @Param			date_to			query	string	true	"Data final (YYYY-MM-DD)"
// @Param			department_id	query	string	false	"ID do departamento"
// @Param			member_id		query	string	false	"ID do membro responsável pela conversa"
// @Param			campaign_id		query	string	false	"ID da campanha"
// @Param			campaign_type	query	string	false	"Tipo da campanha"
// @Param			channel			query	string	false	"Canal de atendimento; mensagens de serviço só existem em whatsapp"
// @Param			origin			query	string	false	"Origem da conversa pela primeira mensagem"	Enums(receptive, campaign, outreach, unknown)
// @Success		200	{object}	attendance.ServiceMessagesSection
// @Failure		400	{object}	response.ErrorResponse
// @Failure		403	{object}	response.ErrorResponse
// @Failure		503	{object}	response.ErrorResponse
// @Failure		504	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/attendance/overview/service_messages [get]
func (h *AttendanceHandler) GetServiceMessagesSection(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.GetWorkspaceID(r)
	if wsID == "" {
		response.WriteError(w, http.StatusBadRequest, "workspace_id required", nil)
		return
	}
	filter, err := parseOverviewFilter(r)
	if err != nil {
		response.WriteErrorWithCode(w, http.StatusBadRequest, CodeOriginInvalid, "Origem inválida: use receptive, campaign, outreach ou unknown", nil)
		return
	}
	departments, blocked := middleware.GetDepartmentFilter(r).ReportScope(filter.DepartmentID)
	if blocked {
		response.WriteErrorWithCode(w, http.StatusForbidden, CodeDepartmentForbidden, "Você não tem acesso a este departamento", nil)
		return
	}
	out, err := h.serviceMessages.ServiceMessages(r.Context(), wsID, attendancedomain.ServiceMessageQuery{Filter: filter, Departments: departments})
	if err != nil {
		writeServiceMessagesError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func writeServiceMessagesError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, attendancedomain.ErrServiceMessagesNotOnPlan):
		response.WriteErrorWithCode(w, http.StatusForbidden, CodeServiceMessagesNotOnPlan, "O plano deste workspace não cobra mensagens de serviço", nil)
	case errors.Is(err, attendancedomain.ErrServiceMessagePriceUnavailable):
		response.WriteErrorWithCode(w, http.StatusServiceUnavailable, CodeServiceMessagesPriceUnavailable, "Não foi possível conferir o preço das mensagens de serviço do plano", nil)
	case errors.Is(err, attendancedomain.ErrInvalidWindow):
		response.WriteErrorWithCode(w, http.StatusBadRequest, CodeServiceMessagesPeriodInvalid, "Informe um período com data inicial e final", nil)
	default:
		writeOverviewError(w, err)
	}
}
