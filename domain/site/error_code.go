package site

import (
	"errors"
	"slices"

	"vozko/domain/sitehtml"
)

var errorCodes = []struct {
	err  error
	code string
}{
	{ErrStackInvalid, CodeStackInvalid},
	{ErrPathInvalid, CodePathInvalid},
	{ErrManifestInvalid, CodeManifestInvalid},
	{ErrKnobsInvalid, CodeKnobsInvalid},
	{ErrKnobValueInvalid, CodeKnobValueInvalid},
	{ErrFormChannelRequired, CodeFormChannelRequired},
	{ErrDesignSystemInvalid, CodeDesignSystemInvalid},
	{ErrSettingsInvalid, CodeSettingsInvalid},
	{ErrRedirectInvalid, CodeRedirectInvalid},
	{ErrStepsInvalid, CodeStepsInvalid},
	{ErrBatchMalformed, CodeBatchMalformed},
	{ErrEditRefused, CodeEditRefused},
	{sitehtml.ErrMalformed, CodeHTMLInvalid},
}

const (
	CodeSettingsInvalid = "settings_invalid"
	CodeRedirectInvalid = "redirect_invalid"
	CodeStepsInvalid    = "steps_invalid"
	CodeBatchMalformed  = "batch_malformed"
	CodeEditRefused     = "edit_refused"
)

func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	for _, known := range errorCodes {
		if errors.Is(err, known.err) {
			return known.code
		}
	}
	return ""
}

func Codes() []string {
	codes := []string{
		CodeStackInvalid, CodePathInvalid, CodePathDuplicate, CodeFileRequired, CodeEntryAmbiguous, CodeFileTooLarge,
		CodeProjectTooLarge, CodeManifestInvalid, CodeRoutePageMissing, CodeKnobsInvalid, CodeKnobVariableUnknown,
		CodeCSSDirectiveForbidden, CodeImportForbidden, CodeImportDynamicForbidden, CodeHTMLInvalid, CodeScriptSourceForbidden, CodeScriptInlineForbidden,
		CodeEventHandlerForbidden, CodeURLForbidden, CodeCredentialFieldForbidden, CodeFormActionForbidden, CodeFrameSourceForbidden, CodeFormInvalid,
		CodeFormFieldUnknown, CodeFormFieldTypeInvalid, CodeFormFieldDuplicate, CodeFormIDDuplicate,
		CodeOperationInvalid, CodeOperationUnknown, CodeFieldRequired, CodeFieldInvalid, CodePathNotFound, CodePathExists,
		CodeNodeInvalid, CodeNodeStale, CodeRevInvalid, CodeRefInvalid, CodeRefDuplicate, CodeRefUnknown, CodePositionInvalid,
		CodeTextMarkupInvalid, CodeAttributeForbidden, CodeClassInvalid, CodeBreakpointInvalid, CodeMediaInvalid,
		CodeLibraryKeyInvalid, CodeTokenInvalid, CodeKnobUnknown, CodeKnobValueInvalid, CodeEditNoop, CodeEditPlaceholder,
		CodeEditNotFound, CodeEditAmbiguous, CodeSkipped, CodeOperationFailed, CodeFileStale, CodeKnobExists, CodeWhatsAppTextTooLong, CodeWhatsAppNumberUnknown,
		CodeSettingsInvalid, CodeRedirectInvalid, CodeStepsInvalid, CodeBatchMalformed, CodeEditRefused, CodeFormChannelRequired, CodeDesignSystemInvalid,
	}
	return slices.Concat(codes, GapCodes())
}
