package sitehttp

import (
	"errors"
	"log"
	"net/http"
	"slices"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/sitebuild"
	"vozko/usecases/siteinspect"
)

const (
	CodeProjectTooLarge = site.CodeProjectTooLarge
	CodeEditRefused     = site.CodeEditRefused
	CodeInternal        = "internal_error"
	CodeBuildFailed     = "build_failed"

	CodeSnippetsFull        = "snippets_full"
	CodePageInvalid         = "page_invalid"
	CodePreviewNotBuilt     = "preview_not_built"
	CodeSectionStackInvalid = "section_stack_invalid"
	CodeSectionsUnavailable = "sections_unavailable"
)

var formRefusals = []response.ErrorRefusal{
	{Err: site.ErrFormChannelRequired, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, site.CodeFormChannelRequired, "Um formulário que inicia um fluxo precisa de um canal de acompanhamento")},
	{Err: site_usecase.ErrFormFunnelUnknown, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, "form_funnel_unknown", "O funil do formulário não existe neste workspace")},
	{Err: site_usecase.ErrFormStageUnknown, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, "form_stage_unknown", "A etapa do formulário não existe neste workspace ou neste funil")},
	{Err: site_usecase.ErrFormOwnerUnknown, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, "form_owner_unknown", "O responsável do formulário não existe neste workspace")},
	{Err: site_usecase.ErrFormWorkflowUnknown, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, "form_workflow_unknown", "O fluxo do formulário não existe neste workspace")},
	{Err: site_usecase.ErrFormChannelUnknown, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, "form_channel_unknown", "O canal de acompanhamento do formulário não existe neste workspace")},
}

var refusals = []response.ErrorRefusal{
	{Err: site_usecase.ErrProjectNotFound, Refusal: response.NewRefusal(http.StatusNotFound, "project_not_found", "Página não encontrada")},
	{Err: site_usecase.ErrRevisionNotFound, Refusal: response.NewRefusal(http.StatusNotFound, "revision_not_found", "Versão não encontrada")},
	{Err: site_usecase.ErrDesignSystemNotFound, Refusal: response.NewRefusal(http.StatusNotFound, "design_system_not_found", "Sistema de design não encontrado")},
	{Err: shared.ErrVersionRequired, Refusal: response.NewRefusal(http.StatusPreconditionRequired, httpx.CodeVersionRequired, "Informe a versão carregada no cabeçalho If-Match")},
	{Err: shared.ErrVersionConflict, Refusal: response.NewRefusal(http.StatusConflict, httpx.CodeVersionConflict, "A página foi salva em outra aba ou por outra pessoa")},
	{Err: site_usecase.ErrProjectArchived, Refusal: response.NewRefusal(http.StatusConflict, "project_archived", "A página está arquivada")},
	{Err: site_usecase.ErrProjectPublished, Refusal: response.NewRefusal(http.StatusConflict, "project_published", "A página está no ar; tire do ar antes")},
	{Err: site_usecase.ErrProjectDomainsBusy, Refusal: response.NewRefusal(http.StatusConflict, "project_domains_busy", "Um domínio da página está sendo verificado; tente de novo em instantes")},
	{Err: site_usecase.ErrProjectSuspended, Refusal: response.NewRefusal(http.StatusConflict, "project_suspended", "A página está suspensa")},
	{Err: site_usecase.ErrSlugTaken, Refusal: response.NewRefusal(http.StatusConflict, "slug_taken", "Outra página do workspace já usa este endereço")},
	{Err: site_usecase.ErrOriginTaken, Refusal: response.NewRefusal(http.StatusConflict, "origin_taken", "A página deste pedido da Elo pertence a outra pessoa")},
	{Err: sitepublish.ErrSlugReserved, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, "slug_reserved", "Este endereço é reservado")},
	{Err: sitepublish.ErrSlugInvalid, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, "slug_invalid", "Use letras minúsculas, números e hífens, de 3 a 63 caracteres")},
	{Err: site_usecase.ErrNameInvalid, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, "name_invalid", "O nome precisa ter de 1 a 120 caracteres")},
	{Err: site_usecase.ErrStarterUnknown, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, "starter_unknown", "Este modelo não existe para esta tecnologia")},
	{Err: site_usecase.ErrLabelInvalid, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, "label_invalid", "O nome da versão precisa ter até 120 caracteres em uma linha")},
	{Err: site_usecase.ErrStatusInvalid, Refusal: response.NewRefusal(http.StatusBadRequest, "status_invalid", "Filtro de situação desconhecido")},
	{Err: site_usecase.ErrPathNotFound, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, site.CodePathNotFound, "Um dos arquivos a excluir não existe")},
	{Err: site_usecase.ErrAuthorInvalid, Refusal: response.NewRefusal(http.StatusUnauthorized, "unauthenticated", "Usuário não identificado")},
	{Err: site_usecase.ErrWorkspaceRequired, Refusal: response.NewRefusal(http.StatusForbidden, "workspace_required", "Escolha um workspace")},
	{Err: site.ErrBatchMalformed, Refusal: response.NewRefusal(http.StatusBadRequest, site.CodeBatchMalformed, "As operações precisam ser uma lista")},
	{Err: site.ErrRedirectInvalid, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, site.CodeRedirectInvalid, "Um redirecionamento é inválido")},
	{Err: site.ErrSettingsInvalid, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, site.CodeSettingsInvalid, "As configurações têm valores inválidos")},
	{Err: site.ErrDesignSystemInvalid, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, site.CodeDesignSystemInvalid, "O sistema de design tem valores inválidos")},
	{Err: site_usecase.ErrStepsInvalid, Refusal: response.NewRefusal(http.StatusBadGateway, site.CodeStepsInvalid, "O construtor devolveu uma alteração que não cabe na página")},
	{Err: site_usecase.ErrBuilderUnavailable, Refusal: response.NewRefusal(http.StatusServiceUnavailable, "build_unavailable", "O construtor de páginas está indisponível; tente de novo")},
	{Err: site_usecase.ErrPreviewNotBuilt, Refusal: response.NewRefusal(http.StatusConflict, CodePreviewNotBuilt, "A pré-visualização desta versão ainda não foi preparada por quem edita a página")},
	{Err: site_usecase.ErrPreviewUnavailable, Refusal: response.NewRefusal(http.StatusServiceUnavailable, "preview_unavailable", "Não foi possível preparar a pré-visualização; tente de novo")},
	{Err: site_usecase.ErrRevisionDamaged, Refusal: response.NewRefusal(http.StatusInternalServerError, "revision_damaged", "Um arquivo desta versão não foi encontrado no armazenamento")},
	{Err: site_usecase.ErrSnippetsFull, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, CodeSnippetsFull, "As seções salvas passam do tamanho máximo de uma página")},
	{Err: sitebuild.ErrSectionStackInvalid, Refusal: response.NewRefusal(http.StatusBadRequest, CodeSectionStackInvalid, "Escolha react ou html")},
	{Err: sitebuild.ErrSectionsUnavailable, Refusal: response.NewRefusal(http.StatusServiceUnavailable, CodeSectionsUnavailable, "As seções salvas não puderam ser lidas; tente de novo")},
}

var outlineReasonCodes = []string{
	site.CodeManifestInvalid, siteinspect.CodeBuildFailed, siteinspect.CodeRulesBroken, siteinspect.CodePreviewUnavailable,
	siteinspect.CodeCaptureUnavailable, siteinspect.CodeSnapshotStale, siteinspect.CodeSnapshotInvalid, siteinspect.CodeRouteUnreadable, siteinspect.CodeNotInspected, siteinspect.CodeInspectionThrottled,
}

var requestRefusals = []response.Refusal{
	response.NewRefusal(http.StatusBadRequest, httpx.CodeInvalidBody, "Corpo da requisição inválido"),
	response.NewRefusal(http.StatusBadRequest, CodePageInvalid, "limit vai de 1 a 100 e offset começa em 0"),
	response.NewRefusal(http.StatusConflict, httpx.CodeVersionConflict, "A página foi salva em outra aba ou por outra pessoa"),
	response.NewRefusal(http.StatusRequestEntityTooLarge, CodeProjectTooLarge, "O envio passa do tamanho máximo de uma página"),
	response.NewRefusal(http.StatusUnprocessableEntity, CodeEditRefused, "A alteração foi recusada"),
	response.NewRefusal(http.StatusUnprocessableEntity, CodeBuildFailed, "A página não compilou"),
	response.NewRefusal(http.StatusInternalServerError, CodeInternal, "Erro interno"),
}

func detailCodes() []string {
	return append(slices.Clone(site.Codes()), outlineReasonCodes...)
}

func writeError(w http.ResponseWriter, err error, fallback string) {
	var issues *site_usecase.IssuesError
	var edit *site_usecase.EditRefusal
	var form *site_usecase.FormReferenceError
	var failed *site_usecase.BuildFailedError
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		response.WriteErrorWithCode(w, http.StatusRequestEntityTooLarge, CodeProjectTooLarge, "O envio passa do tamanho máximo de uma página", nil)
	case errors.As(err, &issues):
		response.WriteErrorWithDetails(w, http.StatusUnprocessableEntity, issues.Code(), "Os arquivos quebram uma regra da página", IssuesDetails{Issues: issues.Issues})
	case errors.As(err, &failed):
		response.WriteErrorWithDetails(w, http.StatusUnprocessableEntity, CodeBuildFailed, "A página não compilou", buildDetails(failed))
	case errors.As(err, &edit):
		response.WriteErrorWithDetails(w, http.StatusUnprocessableEntity, CodeEditRefused, "A alteração foi recusada",
			EditRefusalDetails{Failures: edit.Failures, Skipped: edit.Skipped, Ignored: edit.Ignored, Issues: edit.Issues})
	case errors.As(err, &form):
		matched, ok := response.MatchRefusal(err, formRefusals)
		if !ok {
			writeInternal(w, err, fallback)
			return
		}
		response.WriteErrorWithDetails(w, matched.Status, matched.Code, matched.Message, FormDetails{Form: form.Form})
	case response.WriteErrorRefusal(w, err, formRefusals), response.WriteErrorRefusal(w, err, refusals):
	default:
		writeInternal(w, err, fallback)
	}
}

func writeInternal(w http.ResponseWriter, err error, fallback string) {
	log.Printf("[sites] %s: %v", fallback, err)
	response.WriteErrorWithCode(w, http.StatusInternalServerError, CodeInternal, fallback, nil)
}

func buildDetails(failed *site_usecase.BuildFailedError) BuildFailedDetails {
	out := BuildFailedDetails{Errors: make([]BuildErrorResponse, 0, len(failed.Errors))}
	for _, e := range failed.Errors {
		out.Errors = append(out.Errors, BuildErrorResponse{Path: e.Path, Line: e.Line, Column: e.Column, Message: e.Message, Code: e.Code})
	}
	return out
}
