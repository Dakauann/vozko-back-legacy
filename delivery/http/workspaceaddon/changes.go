package workspaceaddon

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	balance_domain "vozko/domain/balance"
	workspaceaddondomain "vozko/domain/workspace/workspace_addon"
)

var (
	refusalInsufficientBalance   = response.NewRefusal(http.StatusPaymentRequired, "insufficient_balance", "O saldo não cobre o valor a pagar agora. Adicione saldo e tente de novo")
	refusalUnitCap               = response.NewRefusal(http.StatusConflict, "addon_unit_cap_reached", "O workspace já tem o máximo permitido deste adicional")
	refusalUnitsInUse            = response.NewRefusal(http.StatusConflict, "addon_units_in_use", "Há mais itens em uso do que a nova quantidade. Remova alguns antes de reduzir")
	refusalReductionUnsupported  = response.NewRefusal(http.StatusConflict, "addon_reduction_unsupported", "Este adicional não pode ser reduzido por aqui")
	refusalIncreaseNeedsPurchase = response.NewRefusal(http.StatusBadRequest, "addon_increase_needs_purchase", "Para aumentar a quantidade, contrate mais unidades")
	refusalInvalidQuantity       = response.NewRefusal(http.StatusBadRequest, "addon_quantity_invalid", "Informe uma quantidade válida")
	refusalMonthlyOnly           = response.NewRefusal(http.StatusBadRequest, "addon_monthly_only", "Este adicional é cobrado só por mês")
	refusalAddonInactive         = response.NewRefusal(http.StatusConflict, "addon_unavailable", "Este adicional não está disponível para contratação")
	refusalAddonMissing          = response.NewRefusal(http.StatusNotFound, "addon_not_found", "Adicional não encontrado")
	refusalSubscriptionMissing   = response.NewRefusal(http.StatusNotFound, "addon_subscription_not_found", "Contratação do adicional não encontrada")
	refusalChangesUnavailable    = response.NewRefusal(http.StatusServiceUnavailable, "addon_changes_unavailable", "Não foi possível mudar a quantidade agora. Tente de novo em instantes")
)

var addonRefusals = []response.ErrorRefusal{
	{Err: balance_domain.ErrInsufficientBalance, Refusal: refusalInsufficientBalance},
	{Err: workspaceaddondomain.ErrAddonUnitCapReached, Refusal: refusalUnitCap},
	{Err: workspaceaddondomain.ErrMaxUnitsRequired, Refusal: refusalAddonInactive},
	{Err: workspaceaddondomain.ErrMonthlyPriceRequired, Refusal: refusalAddonInactive},
	{Err: workspaceaddondomain.ErrPurchaseChargeInvalid, Refusal: refusalAddonInactive},
	{Err: workspaceaddondomain.ErrMonthlyOnly, Refusal: refusalMonthlyOnly},
	{Err: workspaceaddondomain.ErrUnitsInUse, Refusal: refusalUnitsInUse},
	{Err: workspaceaddondomain.ErrReductionUnsupported, Refusal: refusalReductionUnsupported},
	{Err: workspaceaddondomain.ErrQuantityIncreaseNeedsPurchase, Refusal: refusalIncreaseNeedsPurchase},
	{Err: workspaceaddondomain.ErrInvalidQuantity, Refusal: refusalInvalidQuantity},
	{Err: workspaceaddondomain.ErrInvalidAddonSubscription, Refusal: refusalInvalidQuantity},
	{Err: workspaceaddondomain.ErrAddonInactive, Refusal: refusalAddonInactive},
	{Err: workspaceaddondomain.ErrAddonNotFound, Refusal: refusalAddonMissing},
	{Err: workspaceaddondomain.ErrAddonSubscriptionNotFound, Refusal: refusalSubscriptionMissing},
}

var fallbackAddonRefusals = []response.Refusal{refusalChangesUnavailable}

func (h *WorkspaceAddonHandler) WithQuantityChanges(uc workspaceaddondomain.ScheduleAddonQuantityUseCase) *WorkspaceAddonHandler {
	h.changeQuantity = uc
	return h
}

func (h *WorkspaceAddonHandler) writePurchaseError(w http.ResponseWriter, err error) {
	if refusal, ok := response.MatchRefusal(err, addonRefusals); ok {
		response.WriteRefusal(w, refusal)
		return
	}
	response.WriteRefusal(w, refusalChangesUnavailable)
}

type ChangeQuantityRequest struct {
	Quantity int `json:"quantity" example:"2"`
}

// @Summary		Reduzir quantidade de addon
// @Description	Agenda a nova quantidade de um addon para o próximo ciclo: o ciclo já pago continua com a quantidade atual e a fatura seguinte cobra a nova. Só reduz (para aumentar, contrate mais unidades) e nunca abaixo do que está em uso: para domínios próprios, desconecte domínios antes. Repetir a quantidade atual desfaz uma redução agendada. Nada é devolvido. Responde com scheduledQuantity e scheduledFrom.
// @Tags			Addons
// @Accept			json
// @Produce		json
// @Param			workspaceId			path		string					true	"ID do workspace"
// @Param			addonSubscriptionId	path		string					true	"ID da assinatura do addon"
// @Param			request				body		ChangeQuantityRequest	true	"Nova quantidade"
// @Success		200	{object}	workspace_addon.AddonSubscription
// @Failure		400	{object}	response.ErrorResponse	"addon_quantity_invalid ou addon_increase_needs_purchase"
// @Failure		404	{object}	response.ErrorResponse	"addon_subscription_not_found"
// @Failure		409	{object}	response.ErrorResponse	"addon_units_in_use ou addon_reduction_unsupported"
// @Failure		503	{object}	response.ErrorResponse	"addon_changes_unavailable"
// @Security		BearerAuth
// @Router			/workspaces/{workspaceId}/addons/{addonSubscriptionId}/quantity [put]
func (h *WorkspaceAddonHandler) ChangeQuantity(w http.ResponseWriter, r *http.Request) {
	if h.changeQuantity == nil {
		response.WriteRefusal(w, refusalChangesUnavailable)
		return
	}
	var body ChangeQuantityRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.WriteRefusal(w, refusalInvalidQuantity)
		return
	}
	sub, err := h.changeQuantity.Execute(h.workspaceID(r), mux.Vars(r)["addonSubscriptionId"], body.Quantity)
	if err != nil {
		h.writePurchaseError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, sub)
}
