package campaignaudience

import (
	"errors"
	"fmt"

	"vozko/domain/campaign"
)

var (
	ErrChannelInvalid           = errors.New("campaign audience: unknown channel")
	ErrSourceInvalid            = errors.New("campaign audience: the source must be leads or list")
	ErrSourceAmbiguous          = errors.New("campaign audience: send either a lead selection or a list, never both")
	ErrSelectionRequired        = errors.New("campaign audience: the leads source needs a selection")
	ErrListRequired             = errors.New("campaign audience: the list source needs the id of an uploaded list")
	ErrVariableUnknown          = errors.New("campaign audience: the message has no variable with this key")
	ErrVariableDuplicate        = errors.New("campaign audience: the same variable was sent twice")
	ErrMissingPolicyInvalid     = errors.New("campaign audience: what to do without a value must be text or exclude")
	ErrTextRequired             = errors.New("campaign audience: the default text of a variable cannot be empty")
	ErrChoiceRequired           = errors.New("campaign audience: some leads have no value for a variable; choose a default text or leave them out")
	ErrSourceRequired           = errors.New("campaign audience: every variable needs a lead field or a fixed text")
	ErrListVariablesMissing     = errors.New("campaign audience: some rows of the list have no value for a template variable")
	ErrListMetadataMissing      = errors.New("campaign audience: some rows of the list have no value for a metadata key the flow or agent needs")
	ErrLeadsMetadataUnavailable = errors.New("campaign audience: the flow or agent needs metadata keys that leads do not carry; send a list with those columns")
	ErrListNotFound             = errors.New("campaign audience: the list was not found")
	ErrListExpired              = errors.New("campaign audience: the list expired, upload it again")
	ErrListChannelMismatch      = errors.New("campaign audience: the list was uploaded for another channel")
	ErrSourceUnsupported        = errors.New("campaign audience: this campaign cannot take this source")
	ErrRowNotFound              = errors.New("campaign audience: the row was not found in the list")
	ErrRowDuplicate             = errors.New("campaign audience: the list already has this contact")
	ErrRowContactInvalid        = errors.New("campaign audience: the contact of the row is invalid")
	ErrFileRequired             = errors.New("campaign audience: send the file or the pasted text")
	ErrFileUnreadable           = errors.New("campaign audience: the file could not be read as CSV or XLSX")
	ErrFileTooLarge             = errors.New("campaign audience: the file is larger than 20 MB")
	ErrTemplateRequired         = errors.New("campaign audience: the template is required")
	ErrMessageRequired          = errors.New("campaign audience: the message is required")
	ErrKindRequired             = errors.New("campaign audience: the campaign kind is required")
	ErrUnavailable              = errors.New("campaign audience: the audience step is not configured")
	ErrSuggestionSlot           = errors.New("campaign audience: the template has no variable with this number")
	ErrSuggestionLiteral        = errors.New("campaign audience: a suggestion names a lead field; a free variable has no suggestion")
	ErrSuggestionDuplicate      = errors.New("campaign audience: the same template variable was suggested twice")
	ErrListColumnUnknown        = errors.New("campaign audience: the list has no column with the header a variable reads")
	ErrColumnNeedsList          = errors.New("campaign audience: only a list audience has columns to feed a variable")
	ErrColumnWithSource         = errors.New("campaign audience: a variable reads a list column, a lead field or a fixed text, never two of them")
	ErrDefaultUnavailable       = errors.New("campaign audience: only an agent variable with a default can use it")
	ErrSheetLiteralOnly         = errors.New("campaign audience: a Planilha is one spreadsheet for the whole campaign, never a list column, a lead field or a missing policy")
	ErrSheetRequired            = errors.New("campaign audience: choose the spreadsheet the automation writes to")
	ErrSheetUnfit               = errors.New("campaign audience: the chosen spreadsheet does not fit what the automation writes")
)

type VariableError struct {
	Cause error
	Key   string
}

func (e *VariableError) Error() string {
	return fmt.Sprintf("%v: %q", e.Cause, e.Key)
}

func (e *VariableError) Unwrap() error {
	return e.Cause
}

func variableError(cause error, key string) error {
	return &VariableError{Cause: cause, Key: key}
}

func VariableOf(err error) (string, bool) {
	var named *VariableError
	if !errors.As(err, &named) || named.Key == "" {
		return "", false
	}
	return named.Key, true
}

var errorCodes = []struct {
	err  error
	code string
}{
	{ErrChannelInvalid, "audience_channel_invalid"},
	{ErrSourceInvalid, "audience_source_invalid"},
	{ErrSourceAmbiguous, "audience_source_ambiguous"},
	{ErrSelectionRequired, "audience_selection_required"},
	{ErrListRequired, "audience_list_required"},
	{ErrVariableUnknown, "audience_variable_unknown"},
	{ErrVariableDuplicate, "audience_variable_duplicate"},
	{ErrMissingPolicyInvalid, "audience_missing_policy_invalid"},
	{ErrTextRequired, "audience_text_required"},
	{ErrChoiceRequired, "audience_variable_choice_required"},
	{ErrSourceRequired, "audience_variable_source_required"},
	{ErrListVariablesMissing, "audience_list_variables_missing"},
	{ErrListMetadataMissing, "audience_list_metadata_missing"},
	{ErrLeadsMetadataUnavailable, "audience_leads_metadata_unavailable"},
	{ErrLeadStatusInvalid, "audience_lead_status_invalid"},
	{ErrListNotFound, "audience_list_not_found"},
	{ErrListExpired, "audience_list_expired"},
	{ErrListChannelMismatch, "audience_list_channel_mismatch"},
	{ErrSourceUnsupported, "audience_source_unsupported"},
	{ErrRowNotFound, "audience_row_not_found"},
	{ErrRowDuplicate, "audience_row_duplicate"},
	{ErrRowContactInvalid, "audience_row_contact_invalid"},
	{ErrFileRequired, "audience_file_required"},
	{ErrFileUnreadable, "audience_file_unreadable"},
	{ErrFileTooLarge, "audience_file_too_large"},
	{ErrTemplateRequired, "audience_template_required"},
	{ErrMessageRequired, "audience_message_required"},
	{ErrKindRequired, "audience_kind_required"},
	{ErrUnavailable, "audience_unavailable"},
	{ErrSuggestionSlot, "template_suggestion_slot_invalid"},
	{ErrSuggestionLiteral, "template_suggestion_literal"},
	{ErrSuggestionDuplicate, "template_suggestion_duplicate"},
	{ErrListColumnUnknown, "audience_list_column_unknown"},
	{ErrColumnNeedsList, "audience_variable_column_needs_list"},
	{ErrColumnWithSource, "audience_variable_column_with_source"},
	{ErrDefaultUnavailable, "audience_variable_default_unavailable"},
	{ErrSheetLiteralOnly, "audience_variable_sheet_literal_only"},
	{ErrSheetRequired, "audience_sheet_required"},
	{ErrSheetUnfit, "audience_sheet_unfit"},
	{campaign.ErrListEmpty, "audience_file_empty"},
	{campaign.ErrListContactColumn, "audience_contact_column_missing"},
	{campaign.ErrListTooWide, "audience_file_too_wide"},
	{campaign.ErrListTooLarge, "audience_list_too_large"},
	{campaign.ErrListContactKind, "audience_channel_invalid"},
}

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

func ErrorCodes() []string {
	seen := map[string]bool{}
	var codes []string
	for _, known := range errorCodes {
		if !seen[known.code] {
			seen[known.code] = true
			codes = append(codes, known.code)
		}
	}
	return codes
}
