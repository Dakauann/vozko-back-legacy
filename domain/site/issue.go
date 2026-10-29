package site

import "vozko/domain/siteform"

const (
	CodeStackInvalid             = "stack_invalid"
	CodePathInvalid              = "path_invalid"
	CodePathDuplicate            = "path_duplicate"
	CodeFileRequired             = "file_required"
	CodeEntryAmbiguous           = "entry_ambiguous"
	CodeFileTooLarge             = "file_too_large"
	CodeProjectTooLarge          = "project_too_large"
	CodeManifestInvalid          = "site_json_invalid"
	CodeRoutePageMissing         = "route_page_missing"
	CodeKnobsInvalid             = "knobs_invalid"
	CodeKnobVariableUnknown      = "knob_variable_unknown"
	CodeCSSDirectiveForbidden    = "css_directive_forbidden"
	CodeImportForbidden          = "import_forbidden"
	CodeImportDynamicForbidden   = "import_dynamic_forbidden"
	CodeHTMLInvalid              = "html_invalid"
	CodeScriptSourceForbidden    = "script_source_forbidden"
	CodeScriptInlineForbidden    = "script_inline_forbidden"
	CodeEventHandlerForbidden    = "event_handler_forbidden"
	CodeURLForbidden             = "url_forbidden"
	CodeCredentialFieldForbidden = "credential_field_forbidden"
	CodeFormActionForbidden      = "form_action_forbidden"
	CodeFrameSourceForbidden     = "frame_source_forbidden"
	CodeFormInvalid              = siteform.CodeFormInvalid
	CodeFormFieldUnknown         = siteform.CodeFormFieldUnknown
	CodeFormFieldTypeInvalid     = siteform.CodeFormFieldTypeInvalid
	CodeFormFieldDuplicate       = siteform.CodeFormFieldDuplicate
	CodeFormIDDuplicate          = siteform.CodeFormIDDuplicate
	CodeAltMissing               = "alt_missing"
	CodeLinkEmpty                = "link_empty"
	CodeConsentRequired          = siteform.CodeConsentRequired
	CodePrivacyLinkMissing       = "privacy_link_missing"
	CodeFormIdentityMissing      = siteform.CodeIdentityMissing
	CodeConsentBannerMissing     = "consent_banner_missing"
	CodeInvisibleWithoutJS       = "invisible_without_js"
)

type Issue struct {
	Code   string `json:"code"`
	Path   string `json:"path,omitempty"`
	Route  string `json:"route,omitempty"`
	Node   string `json:"node,omitempty"`
	Detail string `json:"detail,omitempty"`
}

var gapCodes = map[string]bool{
	CodeAltMissing:                  true,
	CodeLinkEmpty:                   true,
	CodeConsentRequired:             true,
	CodePrivacyLinkMissing:          true,
	CodeFormIdentityMissing:         true,
	CodeConsentBannerMissing:        true,
	CodeInvisibleWithoutJS:          true,
	CodeFormDealOwnerRequired:       true,
	CodeFormFollowUpFieldMissing:    true,
	CodeBookingUnconfigured:         true,
	CodeBookingIDTaken:              true,
	CodeBookingCalendarDisconnected: true,
}

func GapCodes() []string {
	return []string{CodeAltMissing, CodeLinkEmpty, CodeConsentRequired, CodePrivacyLinkMissing, CodeFormIdentityMissing, CodeConsentBannerMissing, CodeInvisibleWithoutJS, CodeFormDealOwnerRequired, CodeFormFollowUpFieldMissing, CodeBookingUnconfigured, CodeBookingIDTaken, CodeBookingCalendarDisconnected}
}

func IsGap(code string) bool {
	return gapCodes[code]
}

func Split(issues []Issue) (structural, gaps []Issue) {
	for _, issue := range issues {
		if IsGap(issue.Code) {
			gaps = append(gaps, issue)
			continue
		}
		structural = append(structural, issue)
	}
	return structural, gaps
}
