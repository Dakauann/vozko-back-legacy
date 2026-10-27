package googlesheet

import (
	"errors"
	"fmt"
	"strings"
)

const (
	FieldWorkspace  = "workspace"
	FieldAccount    = "account"
	FieldTitle      = "title"
	FieldTabName    = "tabName"
	FieldTabPeriod  = "tabPeriod"
	FieldTabPattern = "tabPattern"
	FieldStyle      = "style"
	FieldLocale     = "locale"
	FieldTimeZone   = "timeZone"
	FieldColumns    = "columns"
	FieldAdd        = "add"
	FieldUpdate     = "update"
	FieldEdit       = "edit"
)

const (
	CodeWorkspaceRequired     = "google_sheet_workspace_required"
	CodeAccountRequired       = "google_sheet_account_required"
	CodeTitleRequired         = "google_sheet_title_required"
	CodeTitleTooLong          = "google_sheet_title_too_long"
	CodeTabNameTooLong        = "google_sheet_tab_name_too_long"
	CodeTabNameForbidden      = "google_sheet_tab_name_forbidden_character"
	CodeLocaleInvalid         = "google_sheet_locale_invalid"
	CodeTimeZoneUnknown       = "google_sheet_time_zone_unknown"
	CodeNoColumns             = "google_sheet_no_columns"
	CodeHeaderRequired        = "google_sheet_header_required"
	CodeHeaderTooLong         = "google_sheet_header_too_long"
	CodeHeaderRepeated        = "google_sheet_header_repeated"
	CodeTypeUnknown           = "google_sheet_type_unknown"
	CodeSourceUnknown         = "google_sheet_source_unknown"
	CodeSourceTypeMismatch    = "google_sheet_source_type_mismatch"
	CodeInstructionTooLong    = "google_sheet_instruction_too_long"
	CodePerMessageRequired    = "google_sheet_per_message_required"
	CodeOptionsNotChoice      = "google_sheet_options_not_choice"
	CodeOptionsRequired       = "google_sheet_options_required"
	CodeOptionBlank           = "google_sheet_option_blank"
	CodeOptionTooLong         = "google_sheet_option_too_long"
	CodeOptionRepeated        = "google_sheet_option_repeated"
	CodeOptionRemoved         = "google_sheet_option_removed"
	CodeEditEmpty             = "google_sheet_edit_empty"
	CodeTabPeriodUnknown      = "google_sheet_tab_period_unknown"
	CodeTabPatternInvalid     = "google_sheet_tab_pattern_invalid"
	CodeTabPatternNotPeriodic = "google_sheet_tab_pattern_not_periodic"
	CodeStyleUnknown          = "google_sheet_style_unknown"
	CodeColorsNotChoice       = "google_sheet_colors_not_choice"
	CodeColorNotOption        = "google_sheet_color_not_option"
	CodeColorUnknown          = "google_sheet_color_unknown"
	CodeLinkTargetRequired    = "google_sheet_link_target_required"
	CodeLinkLabelInvalid      = "google_sheet_link_label_invalid"
	CodeLinkFieldsNotLink     = "google_sheet_link_fields_not_link"
	CodeLinkRequired          = "google_sheet_link_cannot_be_required"
)

var definitionCodes = []string{
	CodeWorkspaceRequired, CodeAccountRequired, CodeTitleRequired, CodeTitleTooLong, CodeTabNameTooLong, CodeTabNameForbidden,
	CodeLocaleInvalid, CodeTimeZoneUnknown, CodeNoColumns, CodeHeaderRequired, CodeHeaderTooLong, CodeHeaderRepeated,
	CodeTypeUnknown, CodeSourceUnknown, CodeSourceTypeMismatch, CodeInstructionTooLong, CodePerMessageRequired,
	CodeOptionsNotChoice, CodeOptionsRequired, CodeOptionBlank, CodeOptionTooLong, CodeOptionRepeated, CodeOptionRemoved,
	CodeEditEmpty, CodeTabPeriodUnknown, CodeTabPatternInvalid, CodeTabPatternNotPeriodic, CodeStyleUnknown,
	CodeColorsNotChoice, CodeColorNotOption, CodeColorUnknown, CodeLinkTargetRequired, CodeLinkLabelInvalid,
	CodeLinkFieldsNotLink, CodeLinkRequired,
}

func DefinitionCodes() []string { return append([]string(nil), definitionCodes...) }

type DefinitionError struct {
	Code   string
	Field  string
	Column int
	Key    string
	Header string
	Option string
	Limit  int
	Detail string
}

func (e *DefinitionError) Error() string {
	parts := []string{e.Code}
	if e.Header != "" {
		parts = append(parts, fmt.Sprintf("coluna %q", e.Header))
	}
	if e.Option != "" {
		parts = append(parts, fmt.Sprintf("opção %q", e.Option))
	}
	if e.Detail != "" {
		parts = append(parts, e.Detail)
	}
	return ErrInvalidSheet.Error() + ": " + strings.Join(parts, ": ")
}

func (e *DefinitionError) Unwrap() error { return ErrInvalidSheet }

func refuse(field, code string) *DefinitionError {
	return &DefinitionError{Code: code, Field: field, Column: -1}
}

func (e *DefinitionError) limit(n int) *DefinitionError {
	e.Limit = n
	return e
}

func (e *DefinitionError) option(option string) *DefinitionError {
	e.Option = option
	return e
}

func (e *DefinitionError) detail(format string, args ...any) *DefinitionError {
	e.Detail = fmt.Sprintf(format, args...)
	return e
}

func columnRefusal(code string, col Column) *DefinitionError {
	return &DefinitionError{Code: code, Field: FieldColumns, Column: -1, Key: col.Key, Header: col.Header}
}

func atColumn(err error, field string, index int) error {
	var def *DefinitionError
	if !errors.As(err, &def) {
		return err
	}
	def.Field, def.Column = field, index
	return def
}
