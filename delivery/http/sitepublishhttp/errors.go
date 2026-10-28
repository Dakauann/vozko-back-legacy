package sitepublishhttp

import (
	"context"
	"errors"
	"log"
	"net/http"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/domain/sitesubmission"
	siteanalytics_usecase "vozko/usecases/siteanalytics"
	"vozko/usecases/siteforms"
	sitepublish_usecase "vozko/usecases/sitepublish"
)

var (
	refusalAccessDenied     = response.NewRefusal(http.StatusForbidden, "site_access_denied", "Você não tem permissão para fazer isso nesta página")
	refusalUnpaid           = response.NewRefusal(http.StatusPaymentRequired, "subscription_inactive", "A assinatura do workspace não está ativa")
	refusalSiteMissing      = response.NewRefusal(http.StatusNotFound, "site_not_found", "Página não encontrada")
	refusalDeploymentGone   = response.NewRefusal(http.StatusNotFound, "deployment_not_found", "Publicação não encontrada")
	refusalHostnameGone     = response.NewRefusal(http.StatusNotFound, "hostname_not_found", "Domínio não encontrado")
	refusalBuildFailed      = response.NewRefusal(http.StatusUnprocessableEntity, "build_failed", "A página não compilou")
	refusalPrerender        = response.NewRefusal(http.StatusBadGateway, "prerender_failed", "Não foi possível pré-renderizar a página")
	refusalBlocked          = response.NewRefusal(http.StatusUnprocessableEntity, "publish_blocked", "A página quebra uma regra de publicação")
	refusalGaps             = response.NewRefusal(http.StatusUnprocessableEntity, "gaps_open", "A página tem pendências que impedem a publicação")
	refusalFormKey          = response.NewRefusal(http.StatusUnprocessableEntity, "form_key_missing", "Um formulário da página não tem chave pública")
	refusalForms            = response.NewRefusal(http.StatusServiceUnavailable, "forms_unavailable", "Não foi possível ativar os formulários agora")
	refusalMedia            = response.NewRefusal(http.StatusServiceUnavailable, "media_unavailable", "Não foi possível copiar uma imagem da biblioteca")
	refusalHosting          = response.NewRefusal(http.StatusServiceUnavailable, "hosting_unavailable", "A hospedagem está indisponível. Tente de novo em instantes")
	refusalUnderReview      = response.NewRefusal(http.StatusConflict, "publish_under_review", "A página está em análise")
	refusalSuspended        = response.NewRefusal(http.StatusConflict, "site_suspended", "A página está suspensa")
	refusalNotActivatable   = response.NewRefusal(http.StatusConflict, "deployment_not_activatable", "Esta publicação não pode voltar ao ar")
	refusalHostnameTaken    = response.NewRefusal(http.StatusConflict, "hostname_taken", "Este endereço já está em uso")
	refusalHostnameInvalid  = response.NewRefusal(http.StatusUnprocessableEntity, "hostname_invalid", "O domínio não é válido")
	refusalHostnameReserved = response.NewRefusal(http.StatusUnprocessableEntity, "hostname_forbidden", "Este domínio não pode ser usado")
	refusalSlugInvalid      = response.NewRefusal(http.StatusUnprocessableEntity, "slug_invalid", "Use de 3 a 63 letras minúsculas, números e hífens")
	refusalSlugReserved     = response.NewRefusal(http.StatusUnprocessableEntity, "slug_reserved", "Este subdomínio é reservado")
	refusalPlatformTaken    = response.NewRefusal(http.StatusConflict, "platform_hostname_exists", "A página já tem um subdomínio")
	refusalDomainsExhausted = response.NewRefusal(http.StatusConflict, "site_domains_exhausted", "Todos os domínios contratados estão em uso. Compre mais domínios ou remova um")
	refusalDomainAdds       = response.NewRefusal(http.StatusTooManyRequests, "domain_adds_limited", "Muitos domínios foram adicionados hoje. Tente de novo amanhã")
	refusalDomainBusy       = response.NewRefusal(http.StatusConflict, "domain_check_in_progress", "Este domínio já está sendo verificado. Aguarde um instante")
	refusalNotVerified      = response.NewRefusal(http.StatusConflict, "hostname_not_verified", "O registro TXT do domínio ainda não foi encontrado")
	refusalDomainProvider   = response.NewRefusal(http.StatusServiceUnavailable, "domain_provider_unavailable", "O provedor de domínios está indisponível. Tente de novo em instantes")
	refusalKindInvalid      = response.NewRefusal(http.StatusBadRequest, "hostname_kind_invalid", "Tipo de domínio desconhecido")
	refusalTransition       = response.NewRefusal(http.StatusConflict, "hostname_transition_invalid", "O domínio não pode mudar para este estado")
	refusalReason           = response.NewRefusal(http.StatusBadRequest, "reason_required", "Informe o motivo")
	refusalPeriod           = response.NewRefusal(http.StatusBadRequest, "analytics_period_invalid", "Período inválido: use from e to no formato AAAA-MM-DD, com no máximo 400 dias")
	refusalAnalytics        = response.NewRefusal(http.StatusServiceUnavailable, "analytics_unavailable", "Não foi possível ler os números agora")
	refusalUnavailable      = response.NewRefusal(http.StatusServiceUnavailable, "publish_unavailable", "Não foi possível concluir agora. Tente de novo em instantes")
	refusalInvalidBody      = response.NewRefusal(http.StatusBadRequest, httpx.CodeInvalidBody, "Corpo da requisição inválido")
	refusalFormChannel      = response.NewRefusal(http.StatusUnprocessableEntity, site.CodeFormChannelRequired, "Um formulário que inicia um fluxo precisa de um canal de acompanhamento")
	refusalSuccessChannel   = response.NewRefusal(http.StatusUnprocessableEntity, sitesubmission.CodeSuccessChannelMissing, "Escolha o número de WhatsApp que abre depois do envio")
	refusalWhatsAppNumber   = response.NewRefusal(http.StatusUnprocessableEntity, "whatsapp_number_unavailable", "O número de WhatsApp escolhido não pode ser usado por esta página")
	refusalWidgetUnknown    = response.NewRefusal(http.StatusUnprocessableEntity, "webchat_widget_unknown", "O chat embutido na página não é deste workspace")
	refusalLiveScan         = response.NewRefusal(http.StatusBadGateway, "live_scan_failed", "Não foi possível conferir a página depois que os scripts rodaram. Tente de novo em instantes")
	refusalUnrecorded       = response.NewRefusal(http.StatusInternalServerError, "publish_unrecorded", "A página foi ao ar, mas o registro da publicação falhou. Publique de novo")
	refusalMediaRefused     = response.NewRefusal(http.StatusUnprocessableEntity, "media_refused", "Uma imagem da página não é um arquivo público da biblioteca deste workspace")
	refusalCanceled         = response.NewRefusal(http.StatusRequestTimeout, "request_canceled", "A requisição foi cancelada antes de terminar")
	refusalInProgress       = response.NewRefusal(http.StatusConflict, "publish_in_progress", "A página já está sendo publicada. Aguarde terminar")
	refusalStale            = response.NewRefusal(http.StatusConflict, "publish_stale", "A página mudou depois da proposta de publicação. Revise e publique de novo")
)

var refusals = []response.ErrorRefusal{
	{Err: site.ErrFormChannelRequired, Refusal: refusalFormChannel},
	{Err: sitesubmission.ErrSuccessChannelRequired, Refusal: refusalSuccessChannel},
	{Err: siteforms.ErrWhatsAppNumber, Refusal: refusalWhatsAppNumber},
	{Err: siteforms.ErrProjectNotFound, Refusal: refusalSiteMissing},
	{Err: sitepublish_usecase.ErrWidgetUnknown, Refusal: refusalWidgetUnknown},
	{Err: sitepublish_usecase.ErrAccessDenied, Refusal: refusalAccessDenied},
	{Err: sitepublish_usecase.ErrSubscriptionInactive, Refusal: refusalUnpaid},
	{Err: sitepublish_usecase.ErrProjectNotFound, Refusal: refusalSiteMissing},
	{Err: sitepublish.ErrDeploymentNotFound, Refusal: refusalDeploymentGone},
	{Err: sitepublish.ErrHostnameNotFound, Refusal: refusalHostnameGone},
	{Err: sitepublish_usecase.ErrBuildFailed, Refusal: refusalBuildFailed},
	{Err: sitepublish_usecase.ErrPrerenderFailed, Refusal: refusalPrerender},
	{Err: sitepublish_usecase.ErrPublishBlocked, Refusal: refusalBlocked},
	{Err: sitepublish_usecase.ErrGapsOpen, Refusal: refusalGaps},
	{Err: sitepublish_usecase.ErrFormKeyMissing, Refusal: refusalFormKey},
	{Err: sitepublish_usecase.ErrFormsUnavailable, Refusal: refusalForms},
	{Err: sitepublish_usecase.ErrMediaRefused, Refusal: refusalMediaRefused},
	{Err: sitepublish_usecase.ErrMediaUnavailable, Refusal: refusalMedia},
	{Err: sitepublish_usecase.ErrLiveScanFailed, Refusal: refusalLiveScan},
	{Err: sitepublish_usecase.ErrPublishUnrecorded, Refusal: refusalUnrecorded},
	{Err: sitepublish_usecase.ErrPublishInProgress, Refusal: refusalInProgress},
	{Err: sitepublish_usecase.ErrPublishStale, Refusal: refusalStale},
	{Err: sitepublish_usecase.ErrHostingUnavailable, Refusal: refusalHosting},
	{Err: sitepublish_usecase.ErrUnderReview, Refusal: refusalUnderReview},
	{Err: sitepublish_usecase.ErrSiteSuspended, Refusal: refusalSuspended},
	{Err: sitepublish.ErrDeploymentTransition, Refusal: refusalNotActivatable},
	{Err: sitepublish.ErrHostnameTaken, Refusal: refusalHostnameTaken},
	{Err: sitepublish.ErrHostnameInvalid, Refusal: refusalHostnameInvalid},
	{Err: sitepublish.ErrHostnameForbidden, Refusal: refusalHostnameReserved},
	{Err: sitepublish.ErrSlugInvalid, Refusal: refusalSlugInvalid},
	{Err: sitepublish.ErrSlugReserved, Refusal: refusalSlugReserved},
	{Err: sitepublish_usecase.ErrPlatformHostnameTaken, Refusal: refusalPlatformTaken},
	{Err: sitepublish_usecase.ErrSiteDomainsExhausted, Refusal: refusalDomainsExhausted},
	{Err: sitepublish_usecase.ErrDomainProvider, Refusal: refusalDomainProvider},
	{Err: sitepublish_usecase.ErrDomainAddsLimited, Refusal: refusalDomainAdds},
	{Err: sitepublish_usecase.ErrDomainBusy, Refusal: refusalDomainBusy},
	{Err: sitepublish.ErrHostnameNotVerified, Refusal: refusalNotVerified},
	{Err: sitepublish_usecase.ErrHostnameKindInvalid, Refusal: refusalKindInvalid},
	{Err: sitepublish.ErrHostnameTransition, Refusal: refusalTransition},
	{Err: sitepublish_usecase.ErrReasonRequired, Refusal: refusalReason},
	{Err: siteanalytics_usecase.ErrInvalidPeriod, Refusal: refusalPeriod},
	{Err: siteanalytics_usecase.ErrAccessDenied, Refusal: refusalAccessDenied},
	{Err: siteanalytics_usecase.ErrProjectNotFound, Refusal: refusalSiteMissing},
	{Err: siteanalytics_usecase.ErrUnavailable, Refusal: refusalAnalytics},
}

var fallbackRefusals = []response.Refusal{refusalUnavailable, refusalInvalidBody, refusalCanceled}

func writeError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		response.WriteRefusal(w, refusalCanceled)
		return
	}
	match, ok := response.MatchRefusal(err, refusals)
	if !ok {
		log.Printf("[sitepublish] unexpected error: %v", err)
		response.WriteRefusal(w, refusalUnavailable)
		return
	}
	var build *sitepublish_usecase.BuildFailure
	var issues *sitepublish_usecase.IssuesFailure
	switch {
	case errors.As(err, &build):
		response.WriteErrorWithDetails(w, match.Status, match.Code, match.Message, build.Errors)
	case errors.As(err, &issues):
		response.WriteErrorWithDetails(w, match.Status, match.Code, match.Message, issues.Issues)
	default:
		response.WriteRefusal(w, match)
	}
}
