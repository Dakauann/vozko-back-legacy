package handlers

import (
	"net/http"

	"vozko/delivery/http/response"
	workspace_plan "vozko/domain/workspace/workspace_plan"
	"vozko/infra/http/middleware"
)

var PlanRefusals = []response.ErrorRefusal{
	{Err: workspace_plan.ErrPlanNotFound, Refusal: response.Refusal{Status: http.StatusNotFound, Code: "plan_not_found", Message: "Plano não encontrado"}},
	{Err: workspace_plan.ErrPlanArchived, Refusal: response.Refusal{Status: http.StatusConflict, Code: "plan_archived", Message: "O plano está arquivado"}},
	{Err: workspace_plan.ErrPlanExclusiveLocked, Refusal: response.Refusal{Status: http.StatusConflict, Code: "plan_exclusive_locked", Message: "O plano é exclusivo de um afiliado"}},
	{Err: workspace_plan.ErrInvalidPlanName, Refusal: response.Refusal{Status: http.StatusBadRequest, Code: "plan_name_required", Message: "Informe o nome do plano"}},
	{Err: workspace_plan.ErrInvalidBasePrice, Refusal: response.Refusal{Status: http.StatusBadRequest, Code: "plan_base_price_invalid", Message: "O preço base precisa ser zero ou maior"}},
	{Err: workspace_plan.ErrInvalidMaxCallChannels, Refusal: response.Refusal{Status: http.StatusBadRequest, Code: "plan_max_call_channels_invalid", Message: "O plano precisa de ao menos um canal de chamada"}},
	{Err: workspace_plan.ErrExclusiveAffiliateRequired, Refusal: response.Refusal{Status: http.StatusBadRequest, Code: "plan_exclusive_affiliate_required", Message: "Escolha um afiliado ativo"}},
	{Err: workspace_plan.ErrDuplicateNameUnavailable, Refusal: response.Refusal{Status: http.StatusConflict, Code: "plan_duplicate_name_unavailable", Message: "Todos os nomes de cópia deste plano já estão em uso"}},
	{Err: workspace_plan.ErrDuplicateKeyRequired, Refusal: response.Refusal{Status: http.StatusBadRequest, Code: "plan_duplicate_key_required", Message: "Envie o cabeçalho Idempotency-Key com até 128 caracteres"}},
	{Err: workspace_plan.ErrDuplicateKeyReused, Refusal: response.Refusal{Status: http.StatusConflict, Code: "plan_duplicate_key_reused", Message: "Esta Idempotency-Key já duplicou outro plano"}},
	{Err: workspace_plan.ErrDuplicateRequesterRequired, Refusal: response.Refusal{Status: http.StatusUnauthorized, Code: "plan_duplicate_requester_required", Message: "Entre novamente para duplicar o plano"}},
}

var refusalPlanInternal = response.Refusal{Status: http.StatusInternalServerError, Code: "plan_internal_error", Message: "Erro interno"}

var PlanRequestRefusals = []response.Refusal{refusalPlanInternal, middleware.RefusalRoleRequired}

func writePlanError(w http.ResponseWriter, err error) {
	if response.WriteErrorRefusal(w, err, PlanRefusals) {
		return
	}
	response.WriteRefusal(w, refusalPlanInternal)
}
