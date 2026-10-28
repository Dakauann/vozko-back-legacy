package siteformhttp

import (
	"errors"
	"log"
	"net/http"

	"vozko/delivery/http/response"
	pfdomain "vozko/domain/publicform"
	"vozko/domain/sitesubmission"
	"vozko/usecases/siteforms"
)

const (
	CodeInternal    = "internal_error"
	CodePageInvalid = "submissions_page_invalid"
)

var Refusals = []response.ErrorRefusal{
	{Err: sitesubmission.ErrFormUnknown, Refusal: response.NewRefusal(http.StatusNotFound, sitesubmission.CodeFormUnknown, "Este formulário não está publicado")},
	{Err: sitesubmission.ErrOriginRefused, Refusal: response.NewRefusal(http.StatusForbidden, sitesubmission.CodeOriginRefused, "Este endereço não publica este formulário")},
	{Err: sitesubmission.ErrBodyInvalid, Refusal: response.NewRefusal(http.StatusBadRequest, sitesubmission.CodeBodyInvalid, "O envio não tem o formato esperado")},
	{Err: sitesubmission.ErrBotSuspected, Refusal: response.NewRefusal(http.StatusBadRequest, sitesubmission.CodeBotSuspected, "O envio parece automático")},
	{Err: pfdomain.ErrRateLimited, Refusal: response.NewRefusal(http.StatusTooManyRequests, "rate_limited", "Muitas tentativas; aguarde um instante")},
	{Err: pfdomain.ErrChallengeInvalid, Refusal: response.NewRefusal(http.StatusBadRequest, "challenge_invalid", "A verificação do envio é inválida")},
	{Err: pfdomain.ErrChallengeExpired, Refusal: response.NewRefusal(http.StatusBadRequest, "challenge_expired", "A verificação do envio expirou")},
	{Err: pfdomain.ErrChallengeUnsolved, Refusal: response.NewRefusal(http.StatusBadRequest, "challenge_unsolved", "A verificação do envio não foi resolvida")},
	{Err: pfdomain.ErrChallengeReused, Refusal: response.NewRefusal(http.StatusBadRequest, "challenge_reused", "A verificação do envio já foi usada")},
	{Err: pfdomain.ErrSigningKeyMissing, Refusal: response.NewRefusal(http.StatusServiceUnavailable, "form_unavailable", "Formulários indisponíveis no momento")},
	{Err: pfdomain.ErrChallengeUnavailable, Refusal: response.NewRefusal(http.StatusServiceUnavailable, "form_unavailable", "Formulários indisponíveis no momento")},
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
