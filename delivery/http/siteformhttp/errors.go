package siteformhttp

import (
	"errors"
	"log"
	"net/http"

	"vozko/delivery/http/response"
	pfdomain "vozko/domain/publicform"
	"vozko/domain/sitesubmission"
	"vozko/usecases/siteforms"
	workflow_usecase "vozko/usecases/workflow"
)

const (
	CodeInternal    = "internal_error"
	CodePageInvalid = "submissions_page_invalid"
)

var Refusals = []response.ErrorRefusal{
	{Err: sitesubmission.ErrFormUnknown, Refusal: response.NewRefusal(http.StatusNotFound, sitesubmission.CodeFormUnknown, "Este formulário não está publicado")},
	{Err: sitesubmission.ErrOriginRefused, Refusal: response.NewRefusal(http.StatusForbidden, sitesubmission.CodeOriginRefused, "Este endereço não publica este formulário")},
	{Err: sitesubmission.ErrBodyInvalid, Refusal: response.NewRefusal(http.StatusBadRequest, sitesubmission.CodeBodyInvalid, "O envio não tem o formato esperado")},
	{Err: sitesubmission.ErrSlotTaken, Refusal: response.NewRefusal(http.StatusConflict, sitesubmission.CodeSlotTaken, "Este horário não está mais disponível; escolha outro")},
	{Err: sitesubmission.ErrBookingUnavailable, Refusal: response.NewRefusal(http.StatusServiceUnavailable, sitesubmission.CodeBookingUnavailable, "A agenda está indisponível no momento; tente de novo em instantes")},
	{Err: sitesubmission.ErrBotSuspected, Refusal: response.NewRefusal(http.StatusBadRequest, sitesubmission.CodeBotSuspected, "O envio parece automático")},
	{Err: pfdomain.ErrRateLimited, Refusal: response.NewRefusal(http.StatusTooManyRequests, "rate_limited", "Muitas tentativas; aguarde um instante")},
	{Err: pfdomain.ErrChallengeInvalid, Refusal: response.NewRefusal(http.StatusBadRequest, "challenge_invalid", "A verificação do envio é inválida")},
	{Err: pfdomain.ErrChallengeExpired, Refusal: response.NewRefusal(http.StatusBadRequest, "challenge_expired", "A verificação do envio expirou")},
	{Err: pfdomain.ErrChallengeUnsolved, Refusal: response.NewRefusal(http.StatusBadRequest, "challenge_unsolved", "A verificação do envio não foi resolvida")},
	{Err: pfdomain.ErrChallengeReused, Refusal: response.NewRefusal(http.StatusBadRequest, "challenge_reused", "A verificação do envio já foi usada")},
	{Err: pfdomain.ErrSigningKeyMissing, Refusal: response.NewRefusal(http.StatusServiceUnavailable, "form_unavailable", "Formulários indisponíveis no momento")},
	{Err: pfdomain.ErrChallengeUnavailable, Refusal: response.NewRefusal(http.StatusServiceUnavailable, "form_unavailable", "Formulários indisponíveis no momento")},
	{Err: siteforms.ErrProjectNotFound, Refusal: response.NewRefusal(http.StatusNotFound, "project_not_found", "Site não encontrado")},
	{Err: siteforms.ErrFormIDInvalid, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, "form_id_invalid", "Use letras minúsculas, números, hífens e sublinhados no id do formulário")},
	{Err: workflow_usecase.ErrNoDefaultDealFunnel, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, "no_default_deal_funnel", "O workspace não tem um funil de negociações padrão")},
	{Err: siteforms.ErrPageInvalid, Refusal: response.NewRefusal(http.StatusBadRequest, CodePageInvalid, "Página ou período da lista de envios inválido")},
}

func writeError(w http.ResponseWriter, err error) {
	var fields *sitesubmission.FieldsError
	if errors.As(err, &fields) {
		response.WriteErrorWithDetails(w, http.StatusUnprocessableEntity, sitesubmission.CodeFieldsInvalid, "Confira os campos destacados", fields.Details)
		return
	}
	if response.WriteErrorRefusal(w, err, Refusals) {
		return
	}
	log.Printf("[site-forms] request failed: %v", err)
	response.WriteErrorWithCode(w, http.StatusInternalServerError, CodeInternal, "Falha ao processar o formulário", nil)
}
